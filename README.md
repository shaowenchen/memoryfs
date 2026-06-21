# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，FUSE 挂载，HTTP 管理面板。

## 快速开始

### 1. 给节点打标签

```bash
kubectl label node <node-name> memoryfs.io/node=true
```

需打标签的节点数 ≥ Helm `replicaCount`（每节点最多 1 个 MemoryFS Pod）。

### 2. 安装集群

```bash
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32
```

### 3. 挂载

宿主机需有 `/dev/fuse`。挂载容器**必须保持运行**（默认目录 `/mnt/memoryfs`）：

```bash
mkdir -p /mnt/memoryfs

nerdctl run -d --privileged --name memoryfs-fuse \
  --device /dev/fuse \
  -v /mnt/memoryfs:/mnt/memoryfs:shared \
  --network host \
  --restart unless-stopped \
  shaowenchen/memoryfs:latest \
  mount -mount /mnt/memoryfs \
  -nodes http://<host>:19800 \
  -v -f
```

`-nodes` 填任一节点 HTTP 地址（可逗号分隔多个）。详见 [docs/reference/MOUNT.md](docs/reference/MOUNT.md)。

### 4. 卸载

卸载 FUSE：

```bash
nerdctl rm -f memoryfs-fuse
fusermount -u /mnt/memoryfs
```

卸载集群：

```bash
helm uninstall memoryfs -n memoryfs
```

节点 hostPath 上的数据目录（默认 `/data/memoryfs`）不会随 Helm 删除，需自行清理。

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
