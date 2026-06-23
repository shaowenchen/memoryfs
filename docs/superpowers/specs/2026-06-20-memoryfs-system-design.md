# MemoryFS System Design

**Date:** 2026-06-20  
**Status:** approved  
**Type:** baseline（系统级基准规格）

## Goal

分布式文件系统：Raft 强一致元数据 + 多副本 Chunk 存储，支持 FUSE 挂载与 HTTP 管理面板。

## Architecture Summary

```
FUSE/mount ──HTTP──► Node 集群
                       ├── KV + Raft (元数据)
                       ├── Chunk × RF (本地磁盘)
                       └── Repair / GC / Dashboard
```

完整细节见 [docs/reference/ARCHITECTURE.md](../../reference/ARCHITECTURE.md)。

## Components

| 组件 | 路径 | 职责 |
|------|------|------|
| Node | `memoryfs node` | Raft、HTTP/gRPC API、chunk 存储 |
| Mount | `memoryfs mount` | FUSE 客户端，HTTP meta + chunk |
| Meta | `pkg/meta`, `pkg/raftnode` | inode、目录项、集群状态 |
| Service | `pkg/service` | 业务逻辑、repair、registry |
| FUSE | `pkg/fusefs`, `pkg/client`, `pkg/storage` | 客户端文件系统 |
| Deploy | `deploy/helm` | K8s StatefulSet、hostNetwork |

## Key Constraints

- HTTP URI prefix 默认 `/memoryfs`（Helm）
- FUSE mount 从种子节点 **发现全集群节点**；chunk 按 registry 副本位置 **直连数据节点** 读写
- 可挂载多个种子节点（逗号分隔 `-nodes`）实现元数据/chunk 访问高可用
- Mount 容器必须持续运行
- 副本因子 RF 默认 2，节点数 ≥ RF

## Related Docs

- [ARCHITECTURE.md](../../reference/ARCHITECTURE.md)
- [MOUNT.md](../../reference/MOUNT.md)
- [USECASES.md](../../reference/USECASES.md)

## Change Process

新功能或行为变更：**新建** `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`，批准后写 plan，TDD 实现。勿直接改本基准 spec，除非架构级变更。
