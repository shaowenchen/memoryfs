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

默认 **hostNetwork** 部署（HTTP/Raft/gRPC/RDMA 直接监听主机端口；每节点最多 1 个 Pod）。

### RDMA（可选）

启用 RDMA 传输时额外需要：

```bash
--set node.rdma.enabled=true
```

- 节点有 RDMA 网卡（如 InfiniBand / RoCE），并安装相应驱动
- Pod **`privileged: true`** + **`IPC_LOCK`** capability（注册 pinned memory）
- 挂载 **`/dev/infiniband`**、**`/sys/class/infiniband`**
- 镜像需 **`go build -tags rdma`** 构建（默认镜像走 gRPC/HTTP）

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

也可逗号分隔多个 seed 节点；副本数从集群 `/v1/cluster/overview` 自动读取。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
