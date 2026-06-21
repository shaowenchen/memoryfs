# FUSE 挂载

默认挂载目录 **`/mnt/memoryfs`**。

- **任意节点可挂载**：`-nodes` 填集群中任一节点的 **host IP:19800**（可逗号分隔多个），客户端会自动发现全部节点
- 节点注册到集群的是 **宿主机 IP**，不是 K8s 内部 DNS（宿主机上 nerdctl 无法解析 `*.svc.cluster.local`）
- **全集群数据可见**：元数据在各节点；chunk 按 registry 副本位置从对应数据节点直接读取
- **多节点高可用**：多个 seed URL，读写会尝试其他副本/节点

## 前提

- 集群已部署且节点 Ready（见 [deploy/README.md](../../deploy/README.md)）
- 宿主机有 `/dev/fuse`，mount 容器需 `--privileged`

## 挂载步骤

**挂载容器必须一直运行**。容器退出后，宿主机目录会变成 stale mount，`ls`/`df` 报 `Transport endpoint is not connected`。

```bash
# 1. 清理 stale 挂载
bash deploy/scripts/unmount-stale.sh /memoryfs /memoryfs1 /memoryfs2 /mnt/memoryfs

# 2. 创建目录
mkdir -p /mnt/memoryfs

# 3. 后台启动（不要用 -it --rm）
nerdctl pull shaowenchen/memoryfs:latest
nerdctl run -d --privileged --name memoryfs-fuse \
  --device /dev/fuse \
  -v /mnt/memoryfs:/mnt/memoryfs:shared \
  --network host \
  --restart unless-stopped \
  shaowenchen/memoryfs:latest \
  mount -mount /mnt/memoryfs \
  -nodes http://<host>:19800 \
  -v -f
```

`df -h` 的总容量与已用量为**集群汇总**（各 Ready 节点 `disk_quota_bytes` / 用量之和）。3 节点 × 32GB 配置时约显示 **96G**。可逗号分隔多个 `-nodes`。

## 验证

```bash
nerdctl logs -f memoryfs-fuse
df -h /mnt/memoryfs          # 只查当前路径，勿 df | grep memoryfs
ls /mnt/memoryfs
echo test > /mnt/memoryfs/hello.txt
```

## 停止

```bash
nerdctl rm -f memoryfs-fuse
fusermount -u /mnt/memoryfs
```

## 排查

| 现象 | 处理 |
|------|------|
| `connection refused` / DNS `server misbehaving` | 节点列表含 `*.svc.cluster.local` 说明注册地址不对；升级含 host IP 修复的镜像后 **重启全部 Pod**（follower 会 re-join 刷新地址），或清数据重装 |
| Operation not supported | 拉最新镜像（需 FUSE Open 支持） |
| write hang / `context canceled` on chunk PUT | 升级含 FUSE I/O 修复的 mount 镜像；FUSE 请求 ctx 过早取消会导致写失败 |
| 小写入后 `cat` 读不到 / 跨节点看不到 | 写入按 **4 MiB Block** 缓冲；满块自动同步，尾部块在 `fsync` 或关闭文件后同步。升级最新 mount 镜像 |
| lookup `entry not found` 日志 | 创建文件前的 lookup 属正常；新版本不再轮询全部节点刷 warn |

**日志级别：**

- `-v` — FUSE 读写、chunk HTTP、meta 请求
- `-debug` — FUSE 内核级调试

## 本地开发

```bash
make node    # 单节点
make mount   # 挂载到 /tmp/memoryfs
```
