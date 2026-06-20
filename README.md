# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
helm upgrade --install memoryfs ./deploy/helm/memoryfs \
  -n memoryfs --create-namespace
```

默认镜像：`shaowenchen/memoryfs:latest`。国内集群：

```bash
helm upgrade --install memoryfs ./deploy/helm/memoryfs \
  -n memoryfs --create-namespace \
  --set image.repository=registry.cn-beijing.aliyuncs.com/opshub/shaowenchen-memoryfs \
  --set image.tag=latest
```

启用 **定时落盘**：

```bash
helm upgrade --install memoryfs ./deploy/helm/memoryfs \
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
