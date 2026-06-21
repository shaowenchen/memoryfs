# MemoryFS

分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

## 安装

```bash
kubectl label node <node-name> memoryfs.io/node=true
```

```bash
helm upgrade --install memoryfs \
  https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz \
  -n memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32
```

参数：

- `replicaCount` — 节点数（每 K8s 节点最多 1 个 Pod）
- `replicaFactor` — 数据副本数
- `node.storageGB` — 每节点最大存储（GB）

## 挂载

默认挂载目录 **`/mnt/memoryfs`**。`-nodes` 填要挂载的节点；只读写该节点本地 chunk 数据。

**挂载容器必须一直运行**。容器退出后，宿主机上的目录会变成 stale mount，`ls`/`df` 会报 `Transport endpoint is not connected`。

```bash
# 1. 清理旧的 stale 挂载（如有）
bash deploy/scripts/unmount-stale.sh /memoryfs /memoryfs1 /memoryfs2 /mnt/memoryfs

# 2. 创建宿主机目录
mkdir -p /mnt/memoryfs

# 3. 后台启动 mount（不要用 -it --rm，容器退出会 unmount）
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

查看日志与验证：

```bash
nerdctl logs -f memoryfs-mount
df -h /mnt/memoryfs          # 只查当前挂载点，不要用 df | grep memoryfs
ls /mnt/memoryfs
echo test > /mnt/memoryfs/hello.txt
```

停止挂载：

```bash
nerdctl stop memoryfs-mount && nerdctl rm memoryfs-mount
fusermount -u /mnt/memoryfs  # 或 bash deploy/scripts/unmount-stale.sh /mnt/memoryfs
```

也可逗号分隔多个 `-nodes` 节点。`-size-gb` 应与 Helm 的 `node.storageGB` 一致（供 `df` 显示容量）。

**排查日志**：加 `-v` 会打印 FUSE 读写、chunk HTTP、元数据请求；`-debug` 打开 FUSE 内核级调试。

```bash
nerdctl logs -f memoryfs-mount   # 查看 mount 容器日志
```

## 卸载

```bash
helm uninstall memoryfs -n memoryfs
```

## 文档

[docs/](docs/) · [部署与运维](deploy/README.md) · [AI 开发指南](AGENTS.md)

## AI 辅助开发

本项目采用 [Superpowers 方法论](https://www.chenshaowen.com/blog/superpowers-software-engineering-methodology-for-ai.html)：

- [AGENTS.md](AGENTS.md) — Agent 入口与全局约束
- [docs/superpowers/](docs/superpowers/) — 设计规格与实现计划
- `.cursor/skills/` — 项目 Skills（brainstorming、TDD、调试等）

可选：在 Cursor 中安装官方插件 `/add-plugin superpowers`。

## License

MIT
