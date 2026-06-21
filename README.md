# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，FUSE 挂载，HTTP 管理面板。

## 快速开始

```bash
# 1. 标记节点并安装集群
kubectl label node <node-name> memoryfs.io/node=true
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 --set replicaFactor=2 --set node.storageGB=32

# 2. FUSE 挂载 — 见 docs/reference/MOUNT.md
```

## 文档

| 文档 | 说明 |
|------|------|
| [docs/reference/](docs/reference/) | 架构、CLI、挂载、用例 |
| [deploy/README.md](deploy/README.md) | Helm 部署与运维 |
| [docs/reference/MOUNT.md](docs/reference/MOUNT.md) | FUSE 挂载详解 |
| [AGENTS.md](AGENTS.md) | AI / 开发者入口 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | 贡献流程 |

## License

MIT
