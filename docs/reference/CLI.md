# 命令行与运维

单一镜像 `shaowenchen/memoryfs` 包含 `node`、`mount`、`status`、`benchmark`。

```bash
memoryfs node [flags]       # 存储节点
memoryfs node-env           # 从 MEMORYFS_* 环境变量启动（K8s）
memoryfs mount [flags]      # FUSE 客户端
memoryfs status [flags]     # 集群状态
memoryfs benchmark [flags]  # 性能测试
```

## node 参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `-id` | n1 | 节点 ID |
| `-http` / `-grpc` / `-raft` / `-rdma` | :19800 等 | 监听地址（见 `pkg/ports`） |
| `-advertise-http` / `-advertise-raft` | | 集群内宣告地址 |
| `-data` | ./data | Raft/元数据目录 |
| `-chunk-dir` | `{data}/{id}/chunks` | Chunk 落盘目录 |
| `-chunk-backend` | disk | disk / tiered / buffered / memory |
| `-replica-factor` | 2 | 跨节点副本数 |
| `-mem-cache-mb` | 0 | tiered 内存缓存 MB |
| `-disk-quota-gb` | 0 | 磁盘配额 |
| `-gc-interval` | 5m | 孤儿 chunk GC（0=关） |
| `-flush-interval` | 30s | 定时落盘/fsync |
| `-default-ttl` / `-max-file-age` | 0 | 文件过期 |
| `-uri-prefix` | 空 | HTTP 路径前缀（Helm 默认 `/memoryfs`） |
| `-api-token` | | 写操作 Bearer Token |
| `-bootstrap` | | 初始化 Raft 集群 |
| `-standalone` | | 单节点 |
| `-join` | | 加入集群（Leader HTTP URL） |

## mount 参数

| 参数 | 说明 |
|------|------|
| `-mount` | 挂载点（必填） |
| `-nodes` | 节点 HTTP 列表，逗号分隔（必填）；填一个即可 |
| `-f` | 前台运行 |

`df -h` 容量由客户端从集群 overview 各节点 `disk_quota_bytes` 自动汇总。

## status / benchmark

```bash
memoryfs status -nodes http://127.0.0.1:19800
memoryfs status -nodes http://127.0.0.1:19800 -json

memoryfs benchmark -nodes http://127.0.0.1:19800 \
  -writes 50 -reads 50 -workers 4 -size 4194304
```

未指定 `-uri-prefix` 时自动探测 `/memoryfs`。环境变量：`MEMORYFS_NODES`、`MEMORYFS_URI_PREFIX`、`MEMORYFS_API_TOKEN`。

## 运维面板

Helm 默认经 Service 访问：

```
http://<svc>:19800/memoryfs/dashboard
```

Prometheus：`GET /metrics`（无前缀）

## 构建

```bash
make proto build test docker-build
```

CI：push 到 master 构建并推送 `shaowenchen/memoryfs:latest`（amd64）。
