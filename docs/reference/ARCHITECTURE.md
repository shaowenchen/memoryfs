# 架构与设计

## 概览

MemoryFS 是分布式文件系统：元数据通过 **Raft** 强一致复制，文件内容切成 **4 MiB Chunk** 多副本存储在各节点本地磁盘。

```
FUSE/mount ──HTTP/gRPC──► Node 集群
                            ├── KV + Raft (元数据)
                            ├── Chunk × RF (多副本，disk/tiered/buffered)
                            └── Lifecycle / GC / TTL / Repair
```

每个 Node 同时暴露 HTTP、gRPC、RDMA（实验性）三种接口，Chunk 传输按 **RDMA → gRPC → HTTP** 自动降级。

## 接口

| 接口 | 默认端口 | 用途 |
|------|----------|------|
| HTTP | 19800 | REST API、FUSE 客户端、Dashboard |
| gRPC | 19801 | 元数据 RPC、Chunk 流式传输 |
| Raft | 19802 | 节点共识 |
| RDMA | 19803 | 有 IB/RoCE 设备时自动启用，否则降级 gRPC |

Helm 部署时 HTTP API 与 Dashboard 默认路径前缀为 `/memoryfs`（如 `/memoryfs/dashboard`）。集群探针使用无前缀的 `/health`、`/metrics`。网络与 RDMA 见 [Kubernetes 部署](#kubernetes-部署)。

## 元数据

- 每个节点内置 KV，写操作经 Raft 复制到多数派
- Log/Stable 持久化到 `{data}/{id}/raft.db`
- inode、目录项、chunk 索引、节点注册、集群 epoch 均存于 KV

## Chunk 存储

| 后端 | 说明 |
|------|------|
| `disk` | 写入即落盘 |
| `tiered` | 落盘 + 内存读缓存（默认 512MB） |
| `buffered` | 先写内存，定时 flush 落盘 |
| `memory` | 纯内存，重启丢失 |

- 默认 **RF=2**，副本位置记录在 `memoryfs:chunkloc:{id}`
- `-flush-interval` 定时 fsync；`drain`/shutdown 前再次落盘
- 节点重启从本地磁盘恢复，缺失时从 peer **rebuild**

```
写 chunk "2_0" (RF=3)
  ├─► Node1: /data/chunks/2/2_0
  ├─► Node2: /data/chunks/2/2_0
  └─► Node3: /data/chunks/2/2_0
```

## 节点生命周期

```
active ──► draining ──► drained ──► (停止) ──► ready ──► active
```

| 阶段 | 行为 |
|------|------|
| active | 正常读写 |
| draining | 拒绝新 chunk；落盘并确保 RF 副本 |
| drained | 可安全停止 |
| ready | 新节点就绪，从 peer rebuild |

K8s 滚动更新：StatefulSet + preStop drain + postStart ready + PDB。详见 [deploy/README.md](../deploy/README.md)。

## Kubernetes 部署

### 网络（hostNetwork）

默认 **hostNetwork: true**（`dnsPolicy: ClusterFirstWithHostNet`）。HTTP、Raft、gRPC、RDMA 直接监听主机端口；**每 K8s 节点最多 1 个 Pod**（required podAntiAffinity）。节点间用 **主机 IP**（`status.hostIP`）宣告地址。默认端口：HTTP 19800、gRPC 19801、Raft 19802、RDMA 19803（见 Helm `service.httpPort` 等）。

### RDMA

无需单独 Helm 开关。节点有 RDMA 设备（InfiniBand / RoCE，`/dev/infiniband/uverbs*`）时自动走 RDMA 传输，否则降级 gRPC/HTTP。Chart 默认挂载 `/dev/infiniband` 并注入 `IPC_LOCK`；镜像默认带 `rdma` 构建标签。

### 资源与 storageGB

Helm 参数 `node.storageGB` 表示每节点 chunk 存储上限（GB）。Chart 自动设置 Pod 内存 **request/limit = storageGB + 1Gi**（额外 1Gi 预留给进程、Raft 与运行时开销）；磁盘配额 `diskQuotaGB` 与 `storageGB` 一致。

## 集群 Epoch

节点 join/leave 时 epoch +1。客户端可通过 `/health` 感知拓扑变化。

## 协议示例

### HTTP

```bash
curl http://127.0.0.1:19800/health
curl http://127.0.0.1:19800/memoryfs/v1/cluster/overview
curl -X PUT http://127.0.0.1:19800/memoryfs/chunks/2_0 --data-binary @file.bin
```

### gRPC

```bash
grpcurl -plaintext 127.0.0.1:19801 memoryfs.v1.MemoryFS/Health
```

## 传输优先级

写/读 Chunk 时依次尝试 RDMA → gRPC → HTTP，失败自动 fallback。
