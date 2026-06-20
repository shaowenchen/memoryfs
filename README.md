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

参数：

- `replicaCount` — 节点数（每 K8s 节点最多 1 个 Pod）
- `replicaFactor` — 数据副本数
- `node.storageGB` — 每节点最大存储（GB）；Pod 内存 limit/request = storageGB+1Gi

## 挂载

默认挂载目录 **`/data/memoryfs`**。本机 FUSE（需 `--privileged` 与 `/dev/fuse`）：

```bash
kubectl -n memoryfs port-forward svc/memoryfs 8080:8080

docker run -it --rm --privileged \
  -v /data/memoryfs:/data/memoryfs \
  --network host \
  shaowenchen/memoryfs:latest \
  mount -mount /data/memoryfs \
  -nodes http://127.0.0.1:8080 \
  -replica-factor 2 -f
```

`-nodes` 填任意一个可达节点或 Service 即可；`-replica-factor` 与 `replicaFactor` 一致。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
