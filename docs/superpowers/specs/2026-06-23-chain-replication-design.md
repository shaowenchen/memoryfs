# Chain Replication Design

**Date:** 2026-06-23
**Status:** approved
**Author:** memoryfs

## Goal

把 chunk 副本同步从“同步 N-way fan-out”改为 **Chain Replication**：写入只需要 HEAD 落盘即可返回，HEAD 异步沿链向下传播到 TAIL。降低写延迟、消除慢副本拖累、保持均匀分布。

## Context

旧实现 (`SelectNodes` + `putChunk`)：
- mount 把 chunk 发到 hash 选中的某个 replica
- 该节点本地写 + **同步**并行 fan-out 到所有其他 replica
- 任一 peer 慢或不可达 → 整个 PUT 阻塞 / 超时
- 实测：3 节点 RF=2，单个 peer dial timeout 75 s → 写速 ~45 KB/s

约束：
- 不引入额外的元数据存储（chain 由节点列表确定性派生）
- 兼容现有 `/chunks/{id}` HTTP API
- in-memory backend，HEAD 落盘 ≈ 内存写

## Approaches Considered

### Option A: Quorum / 多数派写（如 Dynamo）
- 优点：可调一致性
- 缺点：复杂、需要 vector clock / read repair；in-memory 场景 overkill

### Option B: Chain Replication（3FS / Apache Ozone / BookKeeper）
- 优点：写延迟 = 1× 本地写，强一致读（TAIL），异步同步链路
- 缺点：HEAD 故障会丢失尚未传播的数据；MIDDLE/TAIL 滞后

### Recommendation
**Chain Replication**。memoryfs 是内存级缓存而非主存储，HEAD 失败丢失最近窗口是可接受的折衷；写延迟优先级 > 数据耐久。

## Design

### 概念模型

```
ChainTable
  └── Chain (× N)
        └── Target (× RF)        Role: HEAD / MIDDLE / TAIL
              └── NodeURL
```

- **Target** — 一个数据副本，绑定一个节点，有 Role
- **Chain** — 有序 Target 列表，HEAD→MIDDLE→…→TAIL；**同一 Chain 的 Target 必须在不同节点**
- **ChainTable** — 集群当前可用的全部 Chain；Chain 不在 Table 内不参与存储
- **Role 语义**
  - **HEAD**：最新数据入口，所有写从这里进入
  - **MIDDLE**：从 HEAD 同步数据，再向下游传播
  - **TAIL**：链尾，承载最旧（=已提交）数据，强一致读首选

`pkg/chunk/chain.go` 中的关键类型：

```go
type TargetRole int  // RoleHead / RoleMiddle / RoleTail

type Target struct {
    NodeURL string
    Role    TargetRole
}

type Chain struct {
    ID      uint32
    Targets []Target          // HEAD first, TAIL last
}

type ChainTable struct {
    Chains []Chain
}
```

### Chain 派生

`BuildChainTable(nodes, rf)`：
1. 按 NodeURL 字典序排序
2. 生成 `len(nodes)` 条 Chain
3. Chain i 的 RF 个 Target 从 `sorted[i]` 起按环形取 RF 个相邻节点
4. 第 0 个 = HEAD，第 RF-1 个 = TAIL，中间 = MIDDLE

3 节点 + RF=2：

| Chain | HEAD | TAIL |
|-------|------|------|
| 0     | n0   | n1   |
| 1     | n1   | n2   |
| 2     | n2   | n0   |

3 节点 + RF=3：

| Chain | HEAD | MIDDLE | TAIL |
|-------|------|--------|------|
| 0     | n0   | n1     | n2   |
| 1     | n1   | n2     | n0   |
| 2     | n2   | n0     | n1   |

每个节点恰好担任 N/N = 1 条 chain 的 HEAD，写入负载完全均匀。

### chunk → chain 映射

```go
chainID = fnv1a(chunkID) % len(ChainTable.Chains)
```

确定性、无状态；客户端与服务端独立计算结果一致。

### 写路径

```
mount ──PUT /chunks/{id}──► HEAD
                              │
                              ├─ 1) Chunks.Put(id, data)   本地落盘
                              ├─ 2) 立即返回 201           ← 写请求结束
                              └─ 3) goroutine: PUT /chunks/{id}?replica=1 ──► MIDDLE
                                                                                  │
                                                                                  ├─ Chunks.Put(local)
                                                                                  └─ goroutine: PUT replica=1 ──► TAIL
```

- HEAD 本地写完即返回，**不等下游确认**
- 沿链转发：HEAD→MIDDLE→TAIL，每跳带 `replica=1` 标记，表示“链路内部转发，不再触发分发逻辑”
- 转发失败 → 把 `chunkID` 入 `RepairQueue`，由后台 worker 重试

### 故障与降级

- **HEAD 不可达**：mount 顺序尝试 Chain 中下一个 Target；该 Target 成为新的"链入口"，继续异步向其下游传播
- **MIDDLE 不可达**：HEAD 把任务交给 RepairQueue，由 repair 直接 PUT 给 TAIL
- **节点重启**：从 peer 拉取本链应有的 chunk（现有 `Rebuild` 逻辑）
- **所有 Target 都不可达**：mount 拿到错误，dd 报 EIO

### 读路径

```
mount ──GET /chunks/{id}──► 优先尝试 TAIL（已提交、最一致）
                                fallback → MIDDLE → HEAD
```

TAIL 收到 chunk 时，等价于"全链已提交"。优先读 TAIL 提供强一致；HEAD/MIDDLE 仅在 TAIL 暂不可达时兜底。

### 端到端示例

3 节点 `n0/n1/n2`、RF=2，写 chunk `9_0_7`：

```
fnv1a("9_0_7") % 3 = 1   → Chain 1: n1(HEAD) → n2(TAIL)

mount → PUT n1/chunks/9_0_7
            n1 store local
            n1 ──202 OK──► mount        (写完成)
            n1 goroutine ──PUT n2?replica=1──► n2 store local
```

### 测试策略

- `pkg/chunk/chain_test.go`
  - `TestBuildChainTable3Nodes2RF` — HEAD/TAIL 数量在每节点上完全相等
  - `TestSelectChainSpreadsHEADs` — 1024 个 chunk 的 HEAD 分布偏差 < 20%
  - `TestChainNextOfReturnsTail` — RF=3 时 HEAD→MIDDLE→TAIL 转发链正确

## Success Criteria

- [x] `BuildChainTable` 输出 HEAD/TAIL 在每节点均衡（单测验证）
- [x] HEAD 本地写完即返回，不等待下游
- [x] HEAD/MIDDLE 用 goroutine 异步向 chain 下一跳转发
- [x] HEAD 不可达时 mount 顺序 fallback 到 chain 内其他 Target
- [x] 已有 `RecordChunkRegistry` / `Rebuild` 流程兼容（registry 现在记录的是 chain targets）

## Open Questions

- **HEAD 故障窗口数据丢失** — 当前接受。需要更强耐久时可改为"HEAD + MIDDLE 都落盘再 ACK"，写延迟 ~2×
- **动态扩缩容时 ChainTable 重建** — 当前按 cluster nodes 实时派生；节点变化期间可能出现短暂错位，由 `Rebuild` 兜底
- **ChainTable 持久化** — 目前完全确定性派生，无需持久化；未来若引入手动迁移可补充
