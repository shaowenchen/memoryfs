# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

默认：**emptyDir**（Pod 重启数据不保留）、Chunk **memory** 后端。

```bash
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}"
```

启用 **PVC 落盘**（生产推荐）：

```bash
helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}" \
  --set node.persistence.enabled=true \
  --set node.persistence.size=100Gi
```

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
kubectl delete namespace memoryfs   # 可选；启用 PVC 时会删除持久化数据
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
