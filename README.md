# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk 落盘，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}"
```

验证：

```bash
kubectl -n memoryfs get pod
kubectl -n memoryfs port-forward svc/memoryfs 8080:8080
# http://127.0.0.1:8080/memoryfs/dashboard
```

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
kubectl delete namespace memoryfs   # 可选，会删除 PVC 数据
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
