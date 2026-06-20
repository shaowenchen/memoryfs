# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
VERSION=0.1.3
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace
```

Chart 默认镜像标签为 `v${VERSION}`（与 Release 一致）。国内集群可改用阿里云镜像：

```bash
helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
  --set image.repository=registry.cn-beijing.aliyuncs.com/opshub/shaowenchen-memoryfs
```

启用 **定时落盘**：

```bash
helm upgrade --install memoryfs "${CHART}" \
  -n memoryfs --create-namespace \
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
