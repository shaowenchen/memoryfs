# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
kubectl label node <node-name> memoryfs.io/node=true
```

```bash
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32
```

参数：

- `replicaCount` — 节点数（每 K8s 节点最多 1 个 Pod）
- `replicaFactor` — 数据副本数
- `node.storageGB` — 每节点最大存储（GB）；Pod 内存 limit/request = storageGB+1Gi

## 挂载

默认挂载目录 **`/mnt/memoryfs`**。`-nodes` **填任意一个可达节点**即可；chunk 在其它节点时，该节点会向 peer 拉取后再返回。

```bash
mkdir -p /mnt/memoryfs

nerdctl pull docker.io/shaowenchen/memoryfs:latest
```

```bash
nerdctl run -it --rm --privileged \
  --device /dev/fuse \
  -v /mnt/memoryfs:/mnt/memoryfs:shared \
  --network host \
  docker.io/shaowenchen/memoryfs:latest \
  mount -mount /mnt/memoryfs \
  -nodes http://10.0.0.3:8080 \
  -f
```

- `--device /dev/fuse`：FUSE 必需
- `:shared`：容器内挂载对宿主机 `/mnt/memoryfs` 可见
- 也可逗号分隔多个 nodes 节点

拉镜像若报 `text/html`，说明 registry 返回了错误页（网络/限流），先单独 `nerdctl pull docker.io/shaowenchen/memoryfs:latest` 排查；或在已能拉取该镜像的节点上 `nerdctl save` / `load`。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
