# 分布式内存存储系统设计

**Date:** 2026-06-21  
**Status:** draft（待评审）  
**Type:** 架构设计 — 以「多节点内存池」为核心存储模型

## 1. 目标

用 **多台机器上的物理内存** 组成一个统一的、可挂载的文件存储：

| 目标 | 说明 |
|------|------|
| **内存即存储** | 数据默认驻留各 Node 预分配的 chunk 内存池，而非以磁盘为主、内存为辅 |
| **分布式** | 容量 = Σ(各节点 `storageGB`)，单文件可跨节点 block 分布 |
| **文件语义** | POSIX/FUSE 挂载，应用无感 |
| **可控副本** | RF 份 block 分布在不同节点，节点故障可重建 |
| **强一致元数据** | inode、目录、block 索引经 Raft 复制 |

**非目标（当前阶段）**

- 跨集群持久化（重启后数据仍在）— 可选 `diskSync` 异步落盘，但不是主路径
- 无限扩容 shrink — 容量在 Pod 启动时按 `storageGB` 固定
- 单副本极致性能 — 默认 RF≥2

## 2. 与 JuiceFS / 本地盘的对比

```
JuiceFS:  元数据引擎(Raft/Redis) + 对象存储(磁盘/S3) + 客户端缓存
MemoryFS: 元数据(Raft KV)         + 分布式内存池(RF)   + FUSE 薄客户端
```

| 维度 | JuiceFS | MemoryFS（目标） |
|------|---------|------------------|
| 数据介质 | 对象存储（持久） | **节点内存池（主）** |
| 逻辑块 | Chunk 64MiB | Chunk 64MiB |
| 物理块 | Block 4MiB 上传对象存储 | Block 4MiB 存节点内存 |
| 写路径 | 客户端缓冲 → flush → 对象存储 | **FUSE → Leader → 内存池 + 复制** |
| 容量 | 对象存储几乎无限 | **集群内存总和（如 3×32GB=96GB）** |

## 3. 总体架构

```
                    ┌─────────────────────────────────────┐
                    │  FUSE mount（薄客户端）               │
                    │  - 元数据读：任意 seed               │
                    │  - 写：每次 write → Leader 一次 RPC   │
                    │  - 读：registry 定位 → 副本节点 GET   │
                    └──────────────┬──────────────────────┘
                                   │ HTTP/gRPC/RDMA
         ┌─────────────────────────┼─────────────────────────┐
         ▼                         ▼                         ▼
   ┌───────────┐            ┌───────────┐            ┌───────────┐
   │  Node 0   │            │  Node 1   │            │  Node 2   │
   │ Raft      │◄──────────►│ Raft      │◄──────────►│ Raft      │
   │ Leader*   │   19802    │ Follower  │            │ Follower  │
   ├───────────┤            ├───────────┤            ├───────────┤
   │ Meta KV   │            │ Meta KV   │            │ Meta KV   │
   │ (复制)    │            │ (复制)    │            │ (复制)    │
   ├───────────┤            ├───────────┤            ├───────────┤
   │ MemPool   │            │ MemPool   │            │ MemPool   │
   │ 32GB 预分配│            │ 32GB 预分配│            │ 32GB 预分配│
   │ chunk 池  │            │ chunk 池  │            │ chunk 池  │
   └───────────┘            └───────────┘            └───────────┘
         * Leader 负责写编排、registry、inode 提交
```

**核心原则**

1. **客户端不缓存 chunk** — 数据面写入只到 Node，不在 mount 进程堆内存里攒块。
2. **Node 是存储主体** — 选副本、写内存、同步 peer、提交元数据均在 Node 服务内完成。
3. **内存池启动即分配** — Pod Ready 时按 `storageGB` reserve RSS，chunk 在池内按需占用。
4. **元数据与数据分离** — Raft 只复制 **索引**（小）；block 字节走 **复制通道**（大）。

## 4. 数据模型

### 4.1 三层地址

```
文件 (inode)
  └─ Chunk  (64 MiB)  — 元数据 attr.Chunks[] 逻辑索引
       └─ Block (4 MiB) — 物理存储与复制单元，id = {ino}_{chunkIdx}_{blockIdx}
```

| 对象 | ID 示例 | 存放位置 |
|------|---------|----------|
| inode | `ino=9` | Raft KV |
| 逻辑 chunk | `9_0` | attr.Chunks（仅名字） |
| 物理 block | `9_0_3` | 某 RF 个节点的 MemPool |

### 4.2 集群内存视图

```
集群总容量 = Σ node.storageGB
集群已用   = Σ node.mem_bytes   (实际 chunk 字节)
df 展示    = 总容量 / 总已用          (mount 汇总各节点 stats)
```

每个 Node：

```
MemPool 总大小     = storageGB × 1GiB     (启动时 PreallocMemory reserve)
MemPool 已用         = 本节点持有的 chunk 字节和
MemPool 可接受新块   = 总大小 - 已用       (Put 时检查 quota)
```

## 5. 写路径（目标行为）

```
应用 write(buf, offset)
  │
  ▼
FUSE File.Write
  │  POST /v1/fs/write { ino, offset, data }  →  pinned Leader
  ▼
Service.WriteAt (Leader only)
  │
  ├─ 1. GetAttr(ino)                    — 读当前 inode（本地 Raft KV）
  ├─ 2. 定位 Block(offset)              — chunkIdx, blockIdx, blockOff
  ├─ 3. RMW block                       — 读本地/peer 已有 4MiB，合并写入
  ├─ 4. SelectNodes(blockID, RF)        — 一致性哈希选 RF 个节点
  ├─ 5. PutChunk                        — 本地 MemPool.Put（若在副本集内）
  ├─ 6. PutChunkReplica × (RF-1)        — 同步到 peer 内存池
  ├─ 7. RecordChunkRegistry             — Raft 写 block→replicas 映射
  └─ 8. SetAttr(size, chunks)           — Raft 提交 inode 更新
  │
  ▼
返回成功 → FUSE 完成 write
```

**要点**

- **一次 RPC** 完成：数据落内存 + 副本 + 元数据（非客户端两次写）。
- **Leader 编排** — Follower 不直接接受 FUSE 的 mutating write（redirect 到 Leader）。
- **Block 为复制粒度** — 4MiB 对齐 RDMA/gRPC 传输效率；小写通过 RMW 合并进 block。

### 5.1 后续优化（Node 侧写合并，非 FUSE 缓存）

| 阶段 | 行为 |
|------|------|
| **当前** | 每次 FUSE write（如 1MiB）→ 一次 `/v1/fs/write` |
| **优化** | Leader 对同一 `(ino, block)` 做 **短时写合并窗口**（如 1–10ms 或满 4MiB），再 PutChunk + 复制 |

合并窗口在 **Node 内存**，不在 mount — 符合「不写 FUSE 缓存」原则。

## 6. 读路径

```
应用 read(buf, offset)
  │
  ▼
FUSE → ChunkStore.Read
  │
  ├─ registry GET block replicas     (任意 seed / 缓存的 leader)
  └─ GET /chunks/{blockID}           依次尝试 replica 节点
         RDMA → gRPC → HTTP fallback
```

读 **不经过 Leader**，直连持有副本的 Node，降低 Leader 热点。

## 7. 副本与放置

```
blockID ──hash──► 起始节点 start
副本集 = [ start, start+1, … ] mod N   (RF 个，排序后稳定)
```

| 参数 | 默认 | 约束 |
|------|------|------|
| RF | 2 | RF ≤ 节点数 |
| 放置 | 按 blockID 哈希 | 同 block 的 RF 份在不同节点 |

**Registry**（Raft KV）

```json
{
  "chunk_id": "9_0_3",
  "replicas": ["http://10.0.0.1:19800", "http://10.0.0.2:19800"],
  "epoch": 12
}
```

## 8. 节点内存池（MemPool）

### 8.1 启动分配

```
Pod 启动
  → OpenStoreWithOptions(backend=memory, diskQuotaGB=storageGB)
  → PreallocMemory: make([]byte, storageGB << 30)   // 保留 RSS
  → QuotaMemory:    chunk map 在配额内 Put/Get
```

日志：

```
chunk storage: preallocated 34359738368 bytes at pod start (storageGB quota)
```

K8s：`memory request/limit = storageGB + 1Gi`（+1Gi 给 Raft/运行时）。

### 8.2 Chunk 在池内的形态

```
MemPool
  └── map[blockID] → []byte   // 实际长度 ≤ 4MiB，按需分配 slice（引用预分配池配额）
```

**设计选择**

| 方案 | 优点 | 缺点 |
|------|------|------|
| A. 预分配大数组 + 配额计数（当前） | RSS 可预测，与 K8s limit 对齐 | 启动即占满 limit |
| B. 稀疏 map，仅按写入增长 | 启动占用小 | RSS 不可预测，易 OOM |
| **推荐 A** | 符合「Pod 创建即按大小分配好」 | — |

### 8.3 可选持久化（diskSync）

```
memory (默认)  ── 主路径，重启丢数据
diskSync       ── 异步 flush 到 hostPath，用于慢速备份/恢复
```

持久化是 **增强**，不改变「分布式内存为主存储」的语义。

## 9. 元数据（Raft）

| 键空间 | 内容 |
|--------|------|
| inode / dentry | 目录树、文件属性 |
| `memoryfs:chunkloc:{blockID}` | block → replicas |
| `memoryfs:nodes` | 集群成员 HTTP/Raft 地址 |
| `memoryfs:epoch` | 拓扑变更版本 |

**写顺序**（单 block 写入）

1. 数据已写入 RF 个节点内存（至少 1 成功才继续，不足 RF 入 repair 队列）
2. Raft 提交 registry
3. Raft 提交 inode（size、chunks 列表）

失败回滚策略（待实现强化）：registry 与 inode 原子批处理，或 version + repair。

## 10. 故障与恢复

| 事件 | 行为 |
|------|------|
| 单节点宕机 | 读走其余副本；写仍可用（RF 节点足够） |
| Leader 宕机 | Raft 选新 Leader；mount 跟随 redirect |
| block 副本不足 | repair 后台从幸存副本拉取到缺失节点 |
| 节点重启（纯 memory） | 本地 chunk 丢失；从 peer **rebuild** 属于它的副本 |
| 滚动升级 | preStop drain → 确保 RF → 停止 Pod |

## 11. 容量规划示例

```
3 节点 × storageGB=32  →  集群 df ≈ 96G
RF=2                   →  有效「冗余后」净容量约 48G _unique_ 数据
                       （同一份逻辑数据占 2 份内存）
```

规划公式：

```
可存唯一数据 ≈ (Σ storageGB) / RF
```

## 12. 接口一览

| 调用方 | 接口 | 用途 |
|--------|------|------|
| FUSE | `POST /v1/fs/write` | 写数据 + 元数据（Leader） |
| FUSE | `POST /v1/fs/getattr` 等 | 元数据读 |
| FUSE | `GET /chunks/{blockID}` | 读 block（副本节点） |
| 运维 | `GET /v1/stats` | 单节点内存池用量 |
| 运维 | `GET /v1/cluster/overview` | 集群容量汇总 |

## 13. 实现状态对照

| 设计项 | 状态 |
|--------|------|
| Raft 元数据 | ✅ |
| 64MiB/4MiB 分层 | ✅ |
| RF 复制 + registry | ✅ |
| FUSE 写直通 Leader (`WriteAt`) | ✅ |
| Node PreallocMemory | ✅ |
| Leader 写合并窗口 | ⬜ 待做 |
| RDMA 主路径写 | 🔶 实验性 |
| registry + inode 原子提交 | 🔶 部分 |
| 集群级内存碎片整理 | ⬜ 待做 |

## 14. 演进路线

### Phase 1 — 稳定内存主路径（当前）

- [x] 写直通 Leader
- [x] Pod 预分配内存池
- [ ] 修复大顺序写 EIO（Leader 超时、quota、复制失败可观测）
- [ ] Node 侧 block 写合并（降低 1MiB dd 的 RPC 次数）

### Phase 2 — 性能

- [ ] gRPC 流式 `/v1/fs/write`，避免 base64/JSON 大块
- [ ] RDMA Write 作为复制主路径
- [ ] 读路径多副本并行竞速

### Phase 3 — 可选持久化

- [ ] diskSync 后台 flush + 启动时 warm 内存池
- [ ] 快照 / checkpoint（元数据 + chunk 清单）

## 15. 总结一句话

**MemoryFS = Raft 管理的分布式 inode 树 + 由各 Node 预分配内存池承载的 4MiB block 多副本存储；FUSE 只发写请求，真正的「存内存、同步副本、提交元数据」全在 Node 集群内完成。**
