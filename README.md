# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
kubectl label node <node-name> memoryfs.io/node=true

helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 \
  --set node.storageGB=32
```

只需配置 **`replicaCount`**（节点数）和 **`node.storageGB`**（每节点最大存储 GB）；Pod 内存 limit/request 自动为 storageGB+1Gi。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
