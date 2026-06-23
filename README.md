# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，FUSE 挂载，HTTP 管理面板。

## 快速开始

### 1. 给节点打标签

```bash
kubectl label node <node-name> memoryfs.io/node=true
```

每个节点最多一个 Pod。

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

```bash
mkdir -p /mnt/memoryfs

export IMAGE=shaowenchen/memoryfs:latest
export MEMORYFS_NODES=http://<host>:19800

nerdctl run --name memoryfs-fuse \
  --privileged \
  -d --restart always \
  --network host \
  --device /dev/fuse \
  --mount type=bind,source=/mnt/memoryfs,target=/mnt/memoryfs,bind-propagation=shared \
  "${IMAGE}" \
  mount -mount /mnt/memoryfs \
  -nodes "${MEMORYFS_NODES}" \
  -v -f
```

验证挂载：

```bash
df -h /mnt/memoryfs
```

写入 1GB 测试：

```bash
time dd if=/dev/zero of=/mnt/memoryfs/test.img bs=1M count=1024 oflag=direct status=progress
```

读取测试：

```bash
time dd if=/mnt/memoryfs/test.img of=/dev/null bs=1M iflag=direct status=progress
```

### 4. 卸载

卸载 FUSE：

```bash
nerdctl rm -f memoryfs-fuse
umount /mnt/memoryfs
```

卸载集群：

```bash
helm uninstall memoryfs -n memoryfs
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
