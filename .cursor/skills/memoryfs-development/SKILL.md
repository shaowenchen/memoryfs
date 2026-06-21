---
name: memoryfs-development
description: MemoryFS project conventions — use when writing or modifying Go code in this repo
---

# MemoryFS Development

## 包布局

| 包 | 职责 |
|----|------|
| `cmd/node` | 节点进程：Raft + HTTP/gRPC + chunk 存储 |
| `cmd/mount` | FUSE 客户端（Linux only） |
| `pkg/meta` | inode/目录项抽象 |
| `pkg/service` | 节点业务逻辑 |
| `pkg/client` | 远程元数据 HTTP 客户端 |
| `pkg/fusefs` | FUSE 节点实现 |
| `pkg/storage` | FUSE chunk 读写 |
| `pkg/transport` | chunk HTTP/gRPC/RDMA |
| `pkg/mountlog` | mount 客户端日志（`-v`） |

## 编码原则

- **最小 diff**：只改任务相关文件
- **匹配现有风格**：命名、错误处理、日志
- **URI prefix**：Helm 默认 `/memoryfs`；客户端自动 DetectPrefix
- **Mount chunk I/O**：`NewHTTPChunkStore`，不走 gRPC/RDMA
- **节点 chunk 服务**：`MultiTransport`（RDMA→gRPC→HTTP）

## 测试

```bash
go test ./pkg/<pkg>/... -v -count=1
go test ./... -count=1
```

## 部署相关

- Chart：`deploy/helm/memoryfs/`
- 节点脚本：`deploy/scripts/node-start.sh`
- Mount 脚本：`deploy/scripts/mount-start.sh`
- 文档：`deploy/README.md`、`docs/reference/`

## 日志

- Mount：`-v` 详细，`-debug` FUSE 内核调试
- chunk 读取会按 registry 尝试各副本节点；写入由目标节点负责复制
- 容器：`nerdctl logs -f memoryfs-mount`

## 不要

- 未经设计改 Helm 默认值的大范围重构
- 在 mount 路径重新引入 gRPC-first chunk（已知 hang 问题）
- 提交 `.env`、密钥、instance secret
