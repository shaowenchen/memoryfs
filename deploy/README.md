# MemoryFS 部署指南

本文档描述 MemoryFS 的**推荐生产部署方案**，以及扩缩容、迁移、备份恢复等运维操作。

## 推荐架构

```
                    ┌─────────────────────────────────────┐
                    │  客户端 / 业务 Pod / CI Runner       │
                    │  FUSE mount 或 HTTP/gRPC 直连        │
                    └──────────────┬──────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │         Kubernetes (推荐)              │
              │  StatefulSet (3+ 节点) + hostPath       │
              │  Headless Service (稳定 DNS)           │
              │  PDB minAvailable=2                      │
              └────────────────────┬────────────────────┘
                                   │
         ┌────────────┬────────────┼────────────┬────────────┐
         ▼            ▼            ▼            ▼            ▼
      node-0       node-1       node-2       node-3 ...   (scale)
    Raft+Meta    Chunk+Meta   Chunk+Meta   Chunk+Meta
    hostPath   hostPath   hostPath   hostPath
         │            │            │
         └────────────┴────────────┘
              Chunk RF=2 跨节点副本
              元数据 Raft  quorum
```

### 为什么选 StatefulSet + 3 节点 + RF=2

| 决策 | 原因 |
|------|------|
| **StatefulSet** | Pod 有稳定 DNS（`pod-0.headless`），Raft 成员身份不变 |
| **3 节点起步** | Raft 容忍 1 节点故障；RF=2 时 chunk 可丢 1 副本 |
| **tiered / buffered** | 热读走内存；`diskSync` 开启时定时落盘到 hostPath |
| **Headless Service** | 节点间 `-advertise-http/raft` 使用稳定域名 |
| **PDB minAvailable=2** | 滚动更新/节点维护时保持 quorum |
| **preStop drain** | 缩容/重启前把本地 chunk 复制到 peer |

---

## 快速开始

```bash
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

helm upgrade --install memoryfs "${CHART}" \
  --namespace memoryfs --create-namespace \
  --set image.tag="v${VERSION}"
```

查看 Pod 并打开管理面板（经 Service port-forward）：

```bash
kubectl -n memoryfs get pods -l component=node

kubectl -n memoryfs port-forward svc/memoryfs 8080:8080 &
open http://127.0.0.1:8080/memoryfs/dashboard   # macOS；Linux 用 xdg-open
```

集群内访问：`http://memoryfs.memoryfs.svc:8080/memoryfs/dashboard`

本地开发可直接使用仓库内 Chart：

```bash
helm upgrade --install memoryfs ./deploy/helm/memoryfs \
  --namespace memoryfs --create-namespace
```

Helm 全部可配置项见下文 **[Helm 参数参考](#helm-参数参考)**。

启用 FUSE DaemonSet（每节点挂载）：

```bash
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

helm upgrade memoryfs "${CHART}" \
  --namespace memoryfs \
  --set image.tag="v${VERSION}" \
  --set mount.enabled=true \
  --set mount.hostPath=/var/lib/memoryfs
```

---

## 扩缩容

### 扩容（Scale Up）

**K8s：**

```bash
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"

# 1. 增加副本数
helm upgrade memoryfs "${CHART}" \
  --namespace memoryfs \
  --set image.tag="v${VERSION}" \
  --set replicaCount=5

# 2. 新 Pod 自动 join + ready + rebuild
kubectl -n memoryfs rollout status sts/memoryfs

# 3. 更新 FUSE mount 节点列表（若使用 DaemonSet，需更新 values 中 replicaCount 后 upgrade）
```

### 缩容（Scale Down）

**原则：先 drain → leave → 再停进程/删 Pod**

```bash
# 1. 对要下线的节点执行
./deploy/scripts/scale-down.sh http://n4:8080 http://n1:8080

# 2. K8s 再降低 replicaCount（从最高 ordinal 开始删）
VERSION=0.1.0
CHART="https://github.com/shaowenchen/memoryfs/releases/download/v${VERSION}/memoryfs-${VERSION}.tgz"
helm upgrade memoryfs "${CHART}" --namespace memoryfs --set image.tag="v${VERSION}" --set replicaCount=3

# 3. 确认 hostPath 数据目录可保留或已备份后再缩容
```

**注意：**
- 不要同时下线超过 `N - quorum` 个节点（3 节点集群最多 1 个）
- 缩容前确认 RF 副本已在其他节点落盘

---

## 滚动更新

### K8s（自动）

StatefulSet 滚动更新 + preStop drain + postStart ready，顺序由 K8s 控制。

**最佳实践：Follower 先更新，Leader 最后**

```bash
kubectl -n memoryfs port-forward svc/memoryfs 8080:8080 &
./deploy/scripts/rolling-update.sh http://127.0.0.1:8080
```

---

## 备份与恢复

### 单节点备份

```bash
# hostPath 数据目录（节点上）
./deploy/scripts/backup.sh /data/memoryfs/{instanceId}/memoryfs-0

# 或从 Pod 内打包
kubectl -n memoryfs exec memoryfs-0 -- tar -czf - /data > backup-node0.tar.gz
```

备份内容：
- `{data}/{id}/` — Raft snapshot + 元数据
- `{data}/chunks/` — 本地 chunk 文件

### 恢复

```bash
# 1. 停止节点
# 2. 恢复数据目录
./deploy/scripts/restore.sh backup-node0.tar.gz /data

# 3. 启动节点并 rebuild
./deploy/scripts/node-ready.sh http://node:8080
```

### 灾难恢复（全集群丢失，有部分 chunk 备份）

1. 从备份恢复 **至少 quorum 数量** 节点的元数据（Raft snapshot）
2. 启动集群，确认 `/v1/cluster/nodes` 正常
3. 对每个节点执行 `node-ready`（从 peer rebuild 缺失 chunk）
4. 若元数据全丢但 chunk 文件还在：**无法自动恢复目录树**（元数据在 Raft 中）

**结论：元数据备份与 chunk RF 同样重要。**

---

## 数据迁移

### 场景 1：节点换盘 / 扩容 hostPath 目录

1. `node-drain.sh` 目标节点
2. 停止节点，挂载新盘，复制 `/data`
3. 启动 + `node-ready.sh`

### 场景 2：集群整体迁移到新 K8s

1. 对所有节点执行 backup
2. 在新集群部署相同 `replicaCount` 和节点 ID（StatefulSet ordinal 保持一致最佳）
3. restore 各节点 hostPath 数据目录
4. 按 ordinal 顺序启动 0 → 1 → 2
5. `cluster-status.sh` 验证

### 场景 3：跨云 chunk 迁移

依赖 RF：新节点 join 后 `ready` 自动从 peer pull 属于它的 chunk 副本，无需手动拷贝。

---

## 运维脚本一览

| 脚本 | 用途 |
|------|------|
| `status`（CLI） | 集群存储状态、各节点 chunk/磁盘（推荐） |
| `benchmark`（CLI） | Chunk 写读吞吐性能测试 |
| `cluster-status.sh` | 集群健康、leader、各节点 stats（shell 版） |
| `node-drain.sh` | 迁移 chunk 副本，准备下线 |
| `node-ready.sh` | 标记 active + rebuild |
| `node-leave.sh` | drain + 从集群移除 |
| `scale-up.sh` | 新节点 join |
| `scale-down.sh` | 优雅缩容 |
| `rolling-update.sh` | 交互式滚动更新 |
| `node-rebuild.sh` | 手动触发 rebuild |
| `node-gc.sh` | 孤儿 chunk 清理 |
| `backup.sh` / `restore.sh` | 单节点数据备份恢复 |

---

## Helm 参数参考

| 参数 | 默认 | 说明 |
|------|------|------|
| `replicaCount` | `3` | StatefulSet 节点数 |
| `replicaFactor` | `2` | Chunk 跨节点副本数 |
| `image.repository` | `shaowenchen/memoryfs` | 镜像仓库 |
| `image.tag` | `latest` | 镜像标签（Release 安装建议设 `v0.1.0`） |
| `image.pullPolicy` | `Always` | 镜像拉取策略 |
| `node.chunkBackend` | `memory` | 空值时随 `diskSync` 自动选 `memory`/`buffered` |
| `node.diskSync.enabled` | `false` | 定时落盘开关 |
| `node.diskSync.interval` | `30s` | 落盘/fsync 间隔（开关开启时） |
| `node.storage.type` | `hostPath` | `hostPath`=节点本地盘；`emptyDir`=临时卷 |
| `node.storage.hostPath` | `/data/memoryfs` | 节点本地盘根目录 |
| `node.storage.instanceId` | 随机 8 位 `[a-z0-9]` | 首次安装自动生成并写入 Secret；升级不变 |
| `node.gcInterval` | `5m` | 孤儿 chunk GC 间隔 |
| `node.diskQuotaGB` | `0` | 本地磁盘配额（落盘开启时可设限） |
| `node.lifecycle.preStopDrain` | `true` | 缩容/重启前 drain |
| `dashboard.uriPrefix` | `/memoryfs` | 管理面板与 HTTP API 路径前缀 |
| `apiToken` | | 写操作 Bearer Token（可选） |
| `metrics.enabled` | `false` | 启用 ServiceMonitor |
| `mount.enabled` | `false` | 部署 FUSE DaemonSet（同镜像） |

示例：启用定时落盘 + 本地 hostPath（生产）

```bash
# 各节点预先创建根目录（Chart 也会 DirectoryOrCreate）
sudo mkdir -p /data/memoryfs

helm upgrade memoryfs "${CHART}" -n memoryfs \
  --set image.tag="v${VERSION}" \
  --set node.diskSync.enabled=true \
  --set node.diskSync.interval=30s \
  --set node.storage.type=hostPath \
  --set node.diskQuotaGB=100
```

数据落在节点 `/data/memoryfs/{实例ID}/memoryfs-0` 等路径下（ID 为 8 位小写字母+数字，首次安装随机生成，存于 Secret `{release}-instance`）。

关闭路径前缀（根路径访问 `/dashboard`）：

```bash
helm upgrade memoryfs "${CHART}" -n memoryfs --set dashboard.uriPrefix=
```

---

## 运维 API

| 路径 | 方法 | 用途 |
|------|------|------|
| `{uriPrefix}/dashboard` | GET | Web 管理面板（默认 `/memoryfs/dashboard`） |
| `{uriPrefix}/v1/cluster/overview` | GET | 面板数据 API |
| `/health` | GET | 健康检查（探针，无前缀） |
| `/metrics` | GET | Prometheus 指标（探针，无前缀） |
| `{uriPrefix}/v1/repair/run` | POST | 手动触发副本修复 |
| `{uriPrefix}/v1/stats` | GET | 节点存储统计 |
| `{uriPrefix}/v1/gc` | POST | 孤儿 chunk 清理 |

---

## 环境变量（node-env 模式）

| 变量 | 说明 |
|------|------|
| `MEMORYFS_ID` | 节点 ID |
| `MEMORYFS_BOOTSTRAP` | `true` 初始化集群 |
| `MEMORYFS_JOIN` | Leader HTTP URL |
| `MEMORYFS_HTTP_URL` | 对外宣告的 HTTP 地址 |
| `MEMORYFS_RAFT_URL` | 对外宣告的 Raft 地址 |
| `MEMORYFS_REPLICA_FACTOR` | Chunk 副本数 |
| `MEMORYFS_CHUNK_BACKEND` | disk / tiered / buffered / memory |
| `MEMORYFS_URI_PREFIX` | HTTP 路径前缀（如 `/memoryfs`） |
| `MEMORYFS_API_TOKEN` | API Bearer Token |

完整列表见 `deploy/scripts/node-start.sh`。

---

## 发布 Helm Chart

维护者推送版本 tag 后，GitHub Actions 会自动：

1. 运行测试
2. 打包 `memoryfs-{version}.tgz` 并上传到 [GitHub Releases](https://github.com/shaowenchen/memoryfs/releases)
3. 推送 `shaowenchen/memoryfs:v{version}` 镜像

```bash
git tag v0.1.0
git push origin v0.1.0
```

用户安装时使用 Release 链接：

```bash
https://github.com/shaowenchen/memoryfs/releases/download/v0.1.0/memoryfs-0.1.0.tgz
```

---

## 生产 Checklist

- [ ] 至少 3 节点，RF=2
- [ ] 每节点 hostPath 目录（建议 SSD，`/data/memoryfs`）
- [ ] PDB `minAvailable: 2`
- [ ] preStop drain hook 已启用
- [ ] 定期 backup 元数据目录
- [ ] 监控 `/v1/stats`（disk_bytes、node_state）
- [ ] 网络策略：仅集群内可访问 8080/8081
- [ ] 滚动更新：Follower → Leader 顺序

---

## 故障处理

| 现象 | 处理 |
|------|------|
| `ImagePullBackOff` | 确认镜像存在：优先 `--set image.tag=latest`；国内可用 ACR 镜像 |
| `meta store: not leader` | 确保 memoryfs-0 先 Ready；`helm upgrade` 拉取含修复的新镜像 |
| 节点 `draining` 卡住 | 检查 peer 可达；必要时 `drain?force=true` |
| chunk 缺失 | `node-rebuild.sh` 或 restart（自动 ready） |
| Raft 无 leader | 保证 quorum 节点在线；检查 8081 互通 |
| 磁盘满 | 调整 quota；`-max-file-age`；手动 GC |
| 扩容后 mount 不可见新节点 | 更新 mount `-nodes` 列表或 helm upgrade mount |

更多场景见 [docs/USECASES.md](../docs/USECASES.md)。
