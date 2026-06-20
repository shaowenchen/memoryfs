# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

从 [GitHub Release latest](https://github.com/shaowenchen/memoryfs/releases/tag/latest) 安装 Helm Chart（镜像 `latest`，拉取策略 `Always`）：

```bash
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace
```

国内集群（阿里云镜像）：

```bash
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set image.repository=registry.cn-beijing.aliyuncs.com/opshub/shaowenchen-memoryfs \
  --set image.tag=latest
```

升级 Chart 或镜像后，删除 Pod 触发重建即可拉取最新镜像：

```bash
kubectl -n memoryfs delete pod -l component=node
```

从仓库本地 Chart 安装（开发）见 [deploy/README.md](deploy/README.md)。

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md)

## License

MIT
