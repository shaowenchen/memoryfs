# MemoryFS

基于 [MemoryFS 构想](https://www.chenshaowen.com/blog/something-about-memoryfs-storage-system.html) 的分布式文件系统：Raft 元数据 + 多副本 Chunk 落盘，支持 FUSE 挂载。

## 快速开始

**Docker Compose（3 节点）：**

```bash
make deploy-up
make status
```

**Kubernetes Helm：**

```bash
VERSION=0.1.0
helm upgrade --install memoryfs \
  "https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}"
```

```bash
kubectl -n memoryfs port-forward svc/memoryfs 8080:8080
# 面板: http://127.0.0.1:8080/memoryfs/dashboard
```

**本地开发：**

```bash
go run ./cmd/node -standalone -id n1 -http :8080 -data ./data
go run ./cmd/mount -mount /tmp/memoryfs -nodes http://127.0.0.1:8080 -f
```

## 镜像命令

```bash
memoryfs node ...        # 存储节点
memoryfs mount ...       # FUSE 挂载
memoryfs status ...      # 集群状态
memoryfs benchmark ...   # 性能测试
```

## 文档

| 文档 | 说明 |
|------|------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 架构、协议、生命周期 |
| [docs/CLI.md](docs/CLI.md) | 命令行参数与运维 |
| [docs/USECASES.md](docs/USECASES.md) | 使用场景 |
| [deploy/README.md](deploy/README.md) | 部署、Helm、扩缩容 |

## 构建

```bash
make build    # bin/node bin/mount bin/status bin/benchmark
make test
```

## License

MIT
