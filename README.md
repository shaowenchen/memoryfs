# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}"
```

启用 **定时落盘**（按间隔将 Chunk sync 到节点本地磁盘，重启前也会落盘）：

```bash
helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}" \
  --set node.diskSync.enabled=true
```

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
