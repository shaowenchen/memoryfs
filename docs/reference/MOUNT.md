# FUSE 挂载

默认挂载目录 **`/mnt/memoryfs`**。

- **任意节点可挂载**：`-nodes` 填集群中任一节点的 **host IP:19800**（可逗号分隔多个），客户端会自动发现全部节点
- 节点注册到集群的是 **宿主机 IP**，不是 K8s 内部 DNS（宿主机上 nerdctl 无法解析 `*.svc.cluster.local`）
- **全集群数据可见**：元数据在各节点；chunk 按 registry 副本位置从对应数据节点直接读取
- **多节点高可用**：多个 seed URL，读写会尝试其他副本/节点

## 前提

- 集群已部署且节点 Ready（见 [deploy/README.md](../../deploy/README.md)）
- 宿主机有 `/dev/fuse`，mount 容器需 `--privileged`
- 挂载目录必须用 **`bind-propagation=shared`** 绑定进容器，否则 FUSE 只挂在容器内命名空间，宿主机 `df`/`ls`/`dd` 看不到或报 `Input/output error`

## 挂载步骤

**挂载容器必须一直运行**。容器退出后，宿主机目录会变成 stale mount，`ls`/`df` 报 `Transport endpoint is not connected`。

```bash
# 1. 清理 stale 挂载
bash deploy/scripts/unmount-stale.sh /memoryfs /memoryfs1 /memoryfs2 /mnt/memoryfs

# 2. 创建目录
mkdir -p /mnt/memoryfs

# 3. 后台启动（参考 3FS：bind-propagation=shared + host 网络）
export IMAGE=shaowenchen/memoryfs:latest
export MEMORYFS_NODES=http://<host>:19800   # 任一节点的宿主机 IP:19800，可逗号分隔

nerdctl pull "${IMAGE}"
nerdctl run --name memoryfs-fuse \
  --privileged \
  -d --restart always \
  --network host \
  --device /dev/fuse \
  --mount type=bind,source=/mnt/memoryfs,target=/mnt/memoryfs,bind-propagation=shared \
  "${IMAGE}" \
  mount -mount /mnt/memoryfs \
  -nodes "${MEMORYFS_NODES}" \
  -v -f
```

等价简写（部分 nerdctl 版本也支持）：

```bash
nerdctl run -d --privileged --name memoryfs-fuse \
  --device /dev/fuse \
  -v /mnt/memoryfs:/mnt/memoryfs:shared \
  --network host \
  --restart always \
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

# 1GB 顺序写测试
time dd if=/dev/zero of=/mnt/memoryfs/test.img bs=1M count=1024 oflag=direct status=progress
sync
ls -lh /mnt/memoryfs/test.img
```

## 停止

```bash
nerdctl rm -f memoryfs-fuse
umount /mnt/memoryfs
```

## 排查

| 现象 | 处理 |
|------|------|
| 宿主机 `df` 无 memoryfs / `dd` 报 `Input/output error` 且 mount 日志无 Create | 未用 `bind-propagation=shared` 绑定挂载点，或镜像未含 `AllowOther`；按上文 3FS 方式重挂 |
| `connection refused` / DNS `server misbehaving` | 节点列表含 `*.svc.cluster.local` 说明注册地址不对；升级含 host IP 修复的镜像后 **重启全部 Pod**（follower 会 re-join 刷新地址），或清数据重装 |
| Operation not supported | 拉最新镜像（需 FUSE Open 支持） |
| write hang / `context canceled` on chunk PUT | 升级含 FUSE I/O 修复的 mount 镜像；FUSE 请求 ctx 过早取消会导致写失败 |
| 小写入后 `cat` 读不到 | 确认 mount/node 为最新镜像；每次 write 已直发 Leader，失败看 mount 日志 |
| lookup `entry not found` 日志 | 创建文件前的 lookup 属正常；新版本不再轮询全部节点刷 warn |

**日志级别：**

- `-v` — FUSE 读写、chunk HTTP、meta 请求
- `-debug` — FUSE 内核级调试

## 本地开发

```bash
make node    # 单节点
make mount   # 挂载到 /tmp/memoryfs
```
