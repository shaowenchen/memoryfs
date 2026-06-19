# MemoryFS

基于 [MemoryFS 存储系统的一些构想](https://www.chenshaowen.com/blog/something-about-memoryfs-storage-system.html) 实现的分布式内存文件系统。

## 架构

```
┌─────────────┐     POSIX      ┌──────────────┐
│  FUSE 客户端 │ ◄────────────► │  本地挂载点   │
│  (cmd/mount) │                └──────────────┘
└──────┬──────┘
       │ 元数据
       ▼
┌─────────────┐     数据块      ┌──────────────┐
│    Redis    │                │ Worker 集群   │
│  (元数据存储) │                │ (cmd/worker)  │
└─────────────┘                └──────────────┘
                                     ▲
                               内存 chunk 存储
```

- **元数据层**：Redis 存储 inode、目录项、chunk 索引（参考 JuiceFS 使用 Redis 作为元数据引擎的思路）
- **数据层**：Worker 进程在内存中存储 4 MiB 大小的 chunk，支持横向扩展
- **客户端**：通过 FUSE（[go-fuse](https://github.com/hanwen/go-fuse)）提供 POSIX 文件系统接口

## 前置依赖

- Go 1.21+
- Redis
- macOS / Linux 上的 FUSE（macOS 需安装 [macFUSE](https://osxfuse.github.io/)）

## 快速开始

### 1. 启动 Redis

```bash
docker run -d --name memoryfs-redis -p 6379:6379 redis:7
```

### 2. 启动 Worker（可启动多个实现横向扩展）

```bash
go run ./cmd/worker -addr :8080 -redis 127.0.0.1:6379
# 第二个 worker
go run ./cmd/worker -addr :8081 -redis 127.0.0.1:6379
```

### 3. 挂载文件系统

```bash
mkdir -p /tmp/memoryfs
go run ./cmd/mount -mount /tmp/memoryfs -redis 127.0.0.1:6379 -f
```

### 4. 使用

```bash
echo "hello memoryfs" > /tmp/memoryfs/test.txt
cat /tmp/memoryfs/test.txt
ls -la /tmp/memoryfs
mkdir /tmp/memoryfs/subdir
```

### 5. 卸载

```bash
umount /tmp/memoryfs   # Linux
umount /tmp/memoryfs   # macOS
# 或 Ctrl+C 停止 mount 进程（使用 -f 前台模式时）
```

## 命令行参数

### worker

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | HTTP 监听地址 |
| `-redis` | `127.0.0.1:6379` | Redis 地址，用于注册 worker |

### mount

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mount` | (必填) | 挂载点路径 |
| `-redis` | `127.0.0.1:6379` | Redis 元数据地址 |
| `-workers` | | 手动指定 worker URL 列表（逗号分隔），不填则从 Redis 发现 |
| `-f` | false | 前台运行 |
| `-debug` | false | 开启 FUSE 调试日志 |

## 支持的 POSIX 操作

- 目录：`mkdir`、`rmdir`、`readdir`、`lookup`
- 文件：`create`、`read`、`write`、`unlink`、`truncate`
- 其他：`rename`、`chmod`（setattr）

## 构建

```bash
go build -o bin/worker ./cmd/worker
go build -o bin/mount ./cmd/mount
```

## 设计说明

本实现是文章构想的一个可运行原型：

1. **内存作为存储介质**：Worker 将 chunk 保存在进程内存中
2. **分布式与横向扩展**：多个 Worker 通过 Redis 注册，chunk 按 hash 路由到不同节点
3. **元数据与数据分离**：Redis 存元数据，Worker 存数据块
4. **POSIX via FUSE**：本地挂载后可像普通目录一样使用

## License

MIT
