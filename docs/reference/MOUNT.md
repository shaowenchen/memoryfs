# FUSE 挂载

默认挂载目录 **`/mnt/memoryfs`**。`-nodes` 填要连接的节点 HTTP 地址；chunk 读写仅走该节点本地数据（不自动从其他副本拉取）。

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
nerdctl run -d --privileged --name memoryfs-mount \
  --device /dev/fuse \
  -v /mnt/memoryfs:/mnt/memoryfs:shared \
  --network host \
  --restart unless-stopped \
  shaowenchen/memoryfs:latest \
  mount -mount /mnt/memoryfs \
  -nodes http://10.0.0.3:8080 \
  -size-gb 32 -v -f
```

`-size-gb` 应与 Helm `node.storageGB` 一致（供 `df` 显示容量）。可逗号分隔多个 `-nodes`。

## 验证

```bash
nerdctl logs -f memoryfs-mount
df -h /mnt/memoryfs          # 只查当前路径，勿 df | grep memoryfs
ls /mnt/memoryfs
echo test > /mnt/memoryfs/hello.txt
```

## 停止

```bash
nerdctl stop memoryfs-mount && nerdctl rm memoryfs-mount
fusermount -u /mnt/memoryfs
```

## 排查

| 现象 | 处理 |
|------|------|
| Transport endpoint not connected | mount 容器已退出；清理 stale 后重启 |
| Operation not supported | 拉最新镜像（需 FUSE Open 支持） |
| write hang / 失败 | `nerdctl logs` 看 chunk PUT；确认 URI prefix `/memoryfs` |
| connection refused | 检查节点 Pod、`curl http://<node>:8080/health` |

**日志级别：**

- `-v` — FUSE 读写、chunk HTTP、meta 请求
- `-debug` — FUSE 内核级调试

## 本地开发

```bash
make node    # 单节点
make mount   # 挂载到 /tmp/memoryfs
```
