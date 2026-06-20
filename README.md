# MemoryFS

基于 [MemoryFS 存储系统的一些构想](https://www.chenshaowen.com/blog/something-about-memoryfs-storage-system.html) 实现的分布式内存文件系统。

## 架构

每个 **Node** 同时提供三种访问接口：

| 接口 | 端口 | 用途 |
|------|------|------|
| **HTTP REST** | 8080 | 通用 API、调试、FUSE 客户端 |
| **gRPC** | 9090 | 高性能元数据 + Chunk 流式传输 |
| **RDMA** | 9092 | 实验性：默认降级为 gRPC（`-tags rdma` 启用 TCP 优化传输） |

```
FUSE/mount ──HTTP/gRPC──► Node 集群
                            ├── KV + Raft (元数据，强一致)
                            ├── Chunk × RF (多副本，disk/tiered/memory)
                            └── Lifecycle + GC/TTL (drain/ready，滚动更新)
```

详细场景见 [docs/USECASES.md](docs/USECASES.md)。

## 部署

生产部署、扩缩容、备份恢复见 **[deploy/README.md](deploy/README.md)**。

快速启动 3 节点集群：

```bash
make deploy-up
make deploy-status
```

Kubernetes Helm（[Release 安装](https://github.com/shaowenchen/memoryfs/releases)）：

```bash
VERSION=0.1.0
helm upgrade --install memoryfs \
  "https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz" \
  -n memoryfs --create-namespace \
  --set image.tag="v${VERSION}"
```

## 三种协议

### HTTP API
```bash
curl http://127.0.0.1:8080/health
curl -X POST http://127.0.0.1:8080/v1/fs/lookup \
  -d '{"parent_ino":1,"name":"test.txt"}'
curl -X PUT http://127.0.0.1:8080/chunks/2_0 --data-binary @file.bin
```

### gRPC
```bash
grpcurl -plaintext 127.0.0.1:9090 memoryfs.v1.MemoryFS/Health
grpcurl -plaintext -d '{"parent_ino":1,"name":"test.txt"}' \
  127.0.0.1:9090 memoryfs.v1.MemoryFS/Lookup
```

### RDMA（实验性）
默认构建下 RDMA 自动降级到 gRPC。Linux 上 `-tags rdma` 启用优化 TCP 传输（非硬件 RDMA）：
```bash
go build -tags rdma -o bin/node ./cmd/node
```

传输优先级：**RDMA → gRPC → HTTP**（自动 fallback，RDMA 不可用时透明降级）。

## 前置依赖

- **Linux**（仅支持 Linux 部署）
- FUSE3（`apt install fuse3`）
- Go 1.26+（本地开发）

## Docker 镜像（推荐）

单一镜像 `shaowenchen/memoryfs`，通过启动参数选择服务：

```bash
# 构建
make docker-build

# Node 单节点
docker run -d --name memoryfs-node \
  -p 8080:8080 -p 9090:9090 \
  -v memoryfs-data:/data \
  shaowenchen/memoryfs:latest \
  node -standalone -id n1 -http :8080 -grpc :9090 -data /data

# Node 集群 bootstrap
docker run -d --name memoryfs-n1 \
  -p 8080:8080 -p 9090:9090 \
  -v n1-data:/data \
  shaowenchen/memoryfs:latest \
  node -bootstrap -id n1 -http :8080 -grpc :9090 -raft :8081 -data /data/n1

# FUSE 挂载（需 privileged + /dev/fuse）
docker run -it --rm --privileged \
  --device /dev/fuse --cap-add SYS_ADMIN \
  -v /tmp/memoryfs:/mnt/memoryfs \
  shaowenchen/memoryfs:latest \
  mount -mount /mnt/memoryfs -nodes http://node:8080 -f
```

```bash
docker compose up node1 node2    # 启动集群
docker compose up mount          # FUSE 挂载（Linux privileged）
```

## 快速开始（本地）

### 单节点
```bash
go run ./cmd/node -standalone -id n1 -http :8080 -grpc :9090 -data ./data
go run ./cmd/mount -mount /tmp/memoryfs -nodes http://127.0.0.1:8080 -f
```

### 三节点集群
```bash
go run ./cmd/node -bootstrap -id n1 -http :8080 -grpc :9090 -raft :8081 -data ./data
go run ./cmd/node -id n2 -http :8090 -grpc :9190 -raft :8091 -data ./data -join http://127.0.0.1:8080
go run ./cmd/mount -mount /tmp/memoryfs -nodes http://127.0.0.1:8080,http://127.0.0.1:8090 -f
```

## 滚动更新与数据完整性

### 问题
每个节点 chunk 默认落盘；纯 `memory` 后端时直接重启会丢未副本化的数据。

### 解决方案

#### 1. 元数据：Raft 保证
- 元数据通过 Raft 复制到多数节点（Raft log 持久化到 `{data}/{id}/raft.db`）
- 滚动更新顺序：**Follower 先更新 → Leader 最后更新**
- 任意时刻集群维持 quorum，元数据不丢

#### 2. 数据：多副本 (RF 可配置) + 本地磁盘

- 每个 chunk 默认 **RF=2** 写到不同节点（可通过 `-replica-factor` 调整）
- 每个节点将 chunk **落盘到本地磁盘**（`-chunk-dir`，默认 `{data}/{id}/chunks`）
- 副本位置记录在 KV：`memoryfs:chunkloc:{id}`
- 节点重启后从磁盘恢复；磁盘为空时从 peer **自动 rebuild**

```
写 chunk "2_0" (RF=3)
  ├─► Node1: /data/chunks/2/2_0  (disk)
  ├─► Node2: /data/chunks/2/2_0  (disk)
  └─► Node3: /data/chunks/2/2_0  (disk)
```

滚动更新时：
1. 定时 `-flush-interval` 将 chunk 落盘/fsync 到本地磁盘（`buffered` 后端先写内存再批量落盘）
2. `drain` 在停止前再次落盘，并确保本地 chunk 已在 RF 个节点上有副本
3. 新 Pod 启动 → `ready` → 从 peer 拉取缺失 chunk 到本地磁盘
4. 有 PVC 时磁盘数据直接保留，rebuild 更快

#### 3. 节点生命周期

```
active ──► draining ──► drained ──► (停止/更新) ──► ready ──► active
```

| 阶段 | 行为 |
|------|------|
| **active** | 正常读写 |
| **draining** | 拒绝新 chunk 写入；迁移本地 chunk 到副本节点 |
| **drained** | 所有 chunk 已有足够副本，可安全停止 |
| **ready** | 新启动节点标记就绪，接受流量 |

#### 4. K8s 滚动更新流程

```yaml
lifecycle:
  preStop:
    exec:
      command:
        - /bin/sh
        - -c
        - |
          curl -X POST http://localhost:8080/v1/lifecycle/drain
          until curl -s http://localhost:8080/health | grep drained; do sleep 2; done
  postStart:
    exec:
      command: ["curl", "-X", "POST", "http://localhost:8080/v1/lifecycle/ready"]
```

进程收到 SIGTERM 时也会自动执行 drain。

#### 5. 集群 Epoch
- 每次节点加入/退出，epoch +1
- 客户端可通过 `/health` 或 gRPC `Health` 检查 epoch 变化
- 防止滚动更新期间读到不一致视图

## 命令行参数

### node

| 参数 | 默认 | 说明 |
|------|------|------|
| `-id` | n1 | 节点 ID |
| `-http` | :8080 | HTTP 地址 |
| `-grpc` | :9090 | gRPC 地址 |
| `-rdma` | :9092 | RDMA 地址 |
| `-raft` | :8081 | Raft 地址 |
| `-replica-factor` | 2 | Chunk 跨节点副本数 |
| `-chunk-dir` | `{data}/{id}/chunks` | 本地 chunk 落盘目录 |
| `-chunk-backend` | disk | chunk 存储：`disk`、`tiered`、`buffered` 或 `memory` |
| `-mem-cache-mb` | 0 | 内存读缓存 MB（`tiered` 默认 512） |
| `-disk-quota-gb` | 0 | 本地磁盘配额 GB（0=不限） |
| `-gc-interval` | 5m | 孤儿 chunk GC 间隔（0=关闭） |
| `-flush-interval` | 30s | 本地 chunk 定时落盘/fsync（0=仅 shutdown 时落盘） |
| `-default-ttl` | 0 | 新建文件 TTL（0=关闭） |
| `-max-file-age` | 0 | 按 mtime 过期清理（0=关闭） |
| `-api-token` | | 可选 API Bearer Token（保护写操作） |

## 运维面板

启动节点后访问：

```
http://127.0.0.1:8080/dashboard
```

面板功能：集群节点状态、Leader/Epoch、磁盘用量、副本修复队列、Drain/Ready/GC/Repair 操作。

Prometheus 指标：`GET /metrics`

```bash
curl -s http://127.0.0.1:8080/metrics | grep memoryfs_
```
| `-bootstrap` | | 初始化集群 |
| `-standalone` | | 单节点模式 |
| `-join` | | 加入集群 |

## 构建与 CI

```bash
make proto          # 生成 gRPC 代码
make build          # bin/node bin/mount
make test
make docker-build   # 本地构建镜像
```

GitHub Actions 参考 [ops](https://github.com/shaowenchen/ops) 项目：
- push 到 `main`/`master` → 测试 + 多架构镜像构建（`linux/amd64,linux/arm64`）
- 推送 DockerHub：`shaowenchen/memoryfs:latest`
- 同步阿里云 ACR：`registry.cn-beijing.aliyuncs.com/opshub/shaowenchen-memoryfs:latest`
- Secrets：`DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`、`ACR_USERNAME`、`ACR_PASSWORD`

## License

MIT
