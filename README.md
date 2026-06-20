# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
kubectl label node <node-name> memoryfs.io/node=true

helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32
```

只需配置 **`replicaCount`**（节点数）、**`replicaFactor`**（数据副本数）和 **`node.storageGB`**（每节点最大存储 GB）；Pod 内存 limit/request 自动为 storageGB+1Gi。Chart 强制每个 K8s 节点最多运行 1 个 MemoryFS Pod（`replicaCount` 不能超过已标签节点数）。

## 挂载

**K8s 节点挂载**（每节点 FUSE，业务 Pod 用 hostPath 共享）：

```bash
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32 \
  --set mount.enabled=true
```

挂载目录默认为 **`/var/lib/memoryfs`**（`mount.hostPath`）。

**本机挂载**（需 `--privileged` 与 `/dev/fuse`）：

```bash
kubectl -n memoryfs port-forward svc/memoryfs 8080:8080

docker run -it --rm --privileged \
  -v /mnt/memoryfs:/mnt/memoryfs \
  --network host \
  shaowenchen/memoryfs:latest \
  mount -mount /mnt/memoryfs \
  -nodes http://127.0.0.1:8080 \
  -replica-factor 2 -f
```

`-replica-factor` 须与安装时的 **`replicaFactor`** 一致。`-nodes` 填任意一个可达节点或 Service 地址即可，客户端会自动发现集群。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
