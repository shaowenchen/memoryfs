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

默认 **hostNetwork** 部署（HTTP/Raft/gRPC/RDMA 直接监听主机端口；每节点最多 1 个 Pod）。节点有 RDMA 设备（如 InfiniBand / RoCE）时自动走 RDMA 传输，否则降级 gRPC/HTTP；Chart 已挂载 `/dev/infiniband` 并注入 `IPC_LOCK`。

## 挂载

默认挂载目录 **`/data/memoryfs`**。`-nodes` **填任意一个可达节点**即可（如 node3）；chunk 在其它节点时，该节点会向 peer 拉取后再返回。

```bash
docker run -it --rm --privileged \
  -v /data/memoryfs:/data/memoryfs \
  --network host \
  shaowenchen/memoryfs:latest \
  mount -mount /data/memoryfs \
  -nodes http://10.0.0.3:8080 \
  -f
```

也可逗号分隔多个 nodes 节点。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
