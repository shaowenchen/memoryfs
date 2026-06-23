# MemoryFS 使用场景与优化指南

MemoryFS 是**内存优先**的分布式文件系统：元数据通过 Raft 强一致，Chunk 按 RF 跨节点复制，并发读写**直接命中存储**——无中间缓存层（Direct）。两种 chunk 后端：`memory`（默认，纯 RAM）与 `disk`（直接磁盘 I/O）。

## 典型使用场景

### 1. ML 训练 Scratch 空间

**场景**：多 GPU 节点共享 epoch 数据集、checkpoint 中间态，训练结束即删。

**推荐配置**：
```bash
memoryfs node -chunk-backend memory -disk-quota-gb 64 -replica-factor 2 \
     -default-ttl 24h -max-file-age 48h
```

**要点**：
- `memory`：chunk 直接驻 RAM，无 cache 层抖动
- `-disk-quota-gb`：每个节点的 RAM 上限（预分配 buffer slot）
- `-default-ttl`：新建文件自动过期，避免训练任务异常退出后内存堆积
- RF=2：单节点故障不丢数据，滚动更新时可 drain

**注意**：大文件按 4 MiB 分 chunk，元数据更新频繁；极高并发写同一目录需关注 Raft leader 瓶颈。

---

### 2. CI/CD 共享构建缓存

**场景**：多个 Runner/Pod 共享 Go module、Docker layer、编译产物缓存。

**推荐配置**：
```bash
memoryfs node -chunk-backend disk -disk-quota-gb 100 \
     -gc-interval 10m -replica-factor 2
```

**要点**：
- `disk` 后端直接落盘，重启后仍可命中缓存
- 磁盘配额防止某 Runner 写爆节点
- 定期 GC 清理孤儿 chunk
- 挂载点可挂到 `$GOCACHE` 或 `$CI_CACHE_DIR`

**注意**：缓存命中依赖文件名稳定；不同 job 写同名文件会互相覆盖（POSIX 语义）。

---

### 3. 敏感临时数据（纯内存）

**场景**：密钥材料、临时解密文件，希望进程结束或断电后不可恢复。

**推荐配置**：
```bash
memoryfs node -chunk-backend memory -replica-factor 1 -standalone
```

**要点**：
- `memory` 后端不落盘，断电即失（RF>1 时副本仍在其他节点内存）
- 纯内存 + RF=1 适合单节点隔离环境

**注意**：rolling update 必须先 drain 到其他节点，否则数据丢失。

---

### 4. 实时 ETL / 流处理 Staging

**场景**：Flink/Spark 中间结果、窗口聚合临时表，高 IOPS、生命周期短。

**推荐配置**：
```bash
memoryfs node -chunk-backend memory -disk-quota-gb 96 -default-ttl 6h \
     -max-file-age 12h -gc-interval 5m
```

**要点**：
- `memory` 后端读写直达 RAM，吞吐由网络 + RF 决定，不受磁盘 IOPS 限制
- TTL 按 mtime 或 ExpireAt 清理，定期释放 RAM

**注意**：无文件锁/租约；多 writer 并发写同一文件行为未定义，应用层应一写多读或分文件。

---

### 5. Kubernetes 分布式 emptyDir

**场景**：多 Pod 共享快速 workspace（比 NFS 低延迟，比 emptyDir 可跨节点）。

**推荐配置**（Helm values 示意）：
```yaml
replicaFactor: 2
node:
  chunkBackend: "memory"   # 留空时 diskSync.enabled=false 也默认 memory
  storageGB: 32
  diskSync:
    enabled: false         # 设 true 切到 disk 后端，chunk 走 hostPath/PVC
```

**要点**：
- 默认 memory 后端，读写直达 RAM；切换到 disk 后端时 chunk 写 hostPath/PVC
- FUSE mount 需 `privileged` + `/dev/fuse`

**注意**：FUSE 客户端单点；生产建议 mount sidecar 与业务 Pod 同生命周期，或每节点 DaemonSet 挂载后 hostPath 共享。

---

### 6. 媒体转码 Scratch

**场景**：视频转码中间帧、分片文件，体积大、读写顺序为主。

**推荐配置**：
```bash
memoryfs node -chunk-backend disk -disk-quota-gb 500 -replica-factor 2
memoryfs mount -nodes http://n1:19800,http://n2:19800 -mount /mnt/memoryfs -f
```

**要点**：
- 大顺序写走磁盘，避免占满内存
- gRPC/RDMA 传输适合大块 chunk

**注意**：单文件 >4 GiB 时 chunk 数量多，元数据体积增长；超大文件建议应用层自行分片路径。

---

## 运维 API

| 接口 | 方法 | 用途 |
|------|------|------|
| `/v1/stats` | GET | 查看 chunk 数、磁盘/内存用量、节点状态 |
| `/v1/gc` | POST | 手动触发孤儿 chunk 清理 |
| `/v1/lifecycle/drain` | POST | 滚动更新前迁移副本 |
| `/v1/lifecycle/ready` | POST | 新节点就绪并 rebuild |
| `/health` | GET | 健康检查 + epoch |

示例：
```bash
curl -s http://127.0.0.1:19800/v1/stats | jq
curl -X POST http://127.0.0.1:19800/v1/gc
```

---

## 已覆盖 vs 仍需规划

| 能力 | 状态 | 说明 |
|------|------|------|
| 多副本 + 磁盘持久化 | ✅ | RF 可配，本地 chunk 目录 |
| 滚动更新 drain/ready | ✅ | SIGTERM 自动 drain |
| Direct memory/disk 后端 | ✅ | 读写直达存储，无中间缓存层 |
| 磁盘/内存配额 | ✅ | `-disk-quota-gb`（memory 后端时即 RAM 上限） |
| TTL / 文件过期 | ✅ | `-default-ttl`、`-max-file-age` |
| 孤儿 chunk GC | ✅ | 后台 + `/v1/gc` |
| 节点统计 | ✅ | `/v1/stats` |
| 节点间 TLS/认证 | ❌ | 生产需 mTLS 或网络隔离 |
| Prometheus `/metrics` | ✅ | 节点级指标，Helm 可选 ServiceMonitor |
| Web 运维面板 | ✅ | `/dashboard` |
| 副本修复队列 | ✅ | 后台 + `/v1/repair/run` |
| 节点间 TLS/认证 | 部分 | `-api-token` Bearer 认证 |
| 分布式文件锁 | ❌ | 应用层协调 |
| 跨节点 chunk 迁移均衡 | ❌ | 仅 drain 时复制 |
| 目录级配额 | ❌ | 仅节点级磁盘配额 |
| 加密 at-rest | ❌ | 依赖磁盘/云加密 |
| ACL / 多租户 | ❌ | 依赖挂载点 UID/GID |

---

## 场景选型简表

| 场景 | backend | RF | 关键参数 |
|------|---------|-----|----------|
| ML scratch | memory | 2 | disk-quota-gb, default-ttl |
| CI 缓存 | disk | 2 | disk-quota-gb, gc-interval |
| 敏感临时 | memory | 1 | standalone |
| ETL staging | memory | 2 | default-ttl, max-file-age |
| K8s shared vol | memory/disk | 2 | storageGB + lifecycle hooks |
| 媒体转码 | disk | 2 | disk-quota-gb, gRPC |

---

## 后续可优化方向

1. **Prometheus `/metrics`**：暴露 stats、GC、Raft lag
2. **目录/项目配额**：按 path prefix 限制用量
3. **Chunk 均衡**：后台检测热点节点并迁移副本
4. **Write-once 语义**：CI 缓存场景避免意外覆盖
5. **客户端直连 chunk**：FUSE 读路径 bypass 元数据 leader
6. **mTLS**：节点 HTTP/gRPC 双向认证
