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
| **Headless Service** | Pod 内 join 用 DNS 找 leader；**对外注册** `-advertise-http/raft` 用节点 **host IP**（供宿主机 FUSE 等集群外访问） |
| **PDB minAvailable=2** | 滚动更新/节点维护时保持 quorum |
| **preStop drain** | 缩容/重启前把本地 chunk 复制到 peer |

---

## 快速开始

```bash
kubectl label node <node-name> memoryfs.io/node=true
```

```bash
CHART=https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz

helm upgrade --install memoryfs "${CHART}" \
  --namespace memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32
```

参数：

- `replicaCount` — 节点数（每 K8s 节点最多 1 个 Pod）
- `replicaFactor` — 数据副本数
- `node.storageGB` — 每节点最大存储（GB）（Pod 内存规则见 [docs/reference/ARCHITECTURE.md](../docs/reference/ARCHITECTURE.md#资源与-storagegb)）

网络与 RDMA 见 [docs/reference/ARCHITECTURE.md](../docs/reference/ARCHITECTURE.md#kubernetes-部署)。

### 挂载

默认目录 **`/mnt/memoryfs`**。`-nodes` 指定节点，只访问该节点本地 chunk：

```bash
mount -mount /mnt/memoryfs \
  -nodes http://<host>:19800 \
  -f
```

扩容：为新节点打标签 `memoryfs.io/node=true`，再 `helm upgrade` 增大 `replicaCount`（可同时改 `node.storageGB`）。

```bash
kubectl label node node-4 node-5 memoryfs.io/node=true

helm upgrade memoryfs "${CHART}" -n memoryfs \
  --set replicaCount=5 \
  --set replicaFactor=2 \
  --set node.storageGB=32
```

查看 Pod 并打开管理面板（经 Service port-forward）：

```bash
kubectl -n memoryfs get pods -l component=node

kubectl -n memoryfs port-forward svc/memoryfs 19800:19800 &
open http://127.0.0.1:19800/memoryfs/dashboard   # macOS；Linux 用 xdg-open
```

集群内访问：`http://memoryfs.memoryfs.svc:19800/memoryfs/dashboard`

从仓库本地 Chart 安装（开发、改 Chart 调试）：

```bash
helm upgrade --install memoryfs ./deploy/helm/memoryfs \
  --namespace memoryfs --create-namespace
```

Helm 全部可配置项见下文 **[Helm 参数参考](#helm-参数参考)**。

---

## 扩缩容

### 扩容（Scale Up）

```bash
kubectl label node node-4 node-5 memoryfs.io/node=true

helm upgrade memoryfs "${CHART}" -n memoryfs \
  --set replicaCount=5 \
  --set replicaFactor=2 \
  --set node.storageGB=32

kubectl -n memoryfs rollout status sts/memoryfs
```

### 缩容（Scale Down）

**原则：先 drain → leave → 再停进程/删 Pod**

```bash
# 1. 对要下线的节点执行
./deploy/scripts/scale-down.sh http://n4:19800 http://n1:19800

# 2. 降低 replicaCount
helm upgrade memoryfs "${CHART}" -n memoryfs \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=32

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
kubectl -n memoryfs port-forward svc/memoryfs 19800:19800 &
./deploy/scripts/rolling-update.sh http://127.0.0.1:19800
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
./deploy/scripts/node-ready.sh http://node:19800
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

**安装时通常只需 `replicaCount`、`replicaFactor` 与 `node.storageGB`。** 其余为高级/默认值。

| 参数 | 默认 | 说明 |
|------|------|------|
| **`replicaCount`** | `3` | **集群节点数** |
| **`replicaFactor`** | `2` | **数据副本数**（chunk 跨节点复制份数） |
| **`node.storageGB`** | `3` | **每节点最大存储（GB）**；Pod 内存自动为 storageGB+1Gi |
| `hostNetwork` | `true` | 主机网络；节点用 hostIP 宣告地址 |
| `nodeSelector` | `memoryfs.io/node: "true"` | 仅调度到已打标签的节点 |
| `image.repository` | `shaowenchen/memoryfs` | 镜像仓库 |
| `image.tag` | `latest` | 镜像标签 |
| `imagePullPolicy` | `Always`（模板固定） | 每次 Pod 创建/重启拉取最新镜像 |
| `node.chunkBackend` | `memory` | 空值时随 `diskSync` 自动选 `memory`/`buffered` |
| `node.diskSync.enabled` | `false` | 定时落盘开关 |
| `node.diskSync.interval` | `30s` | 落盘/fsync 间隔（开关开启时） |
| `node.storage.type` | `hostPath` | `hostPath`=节点本地盘；`emptyDir`=临时卷 |
| `node.storage.hostPath` | `/data/memoryfs` | 节点本地盘根目录 |
| `node.storage.instanceId` | 随机 8 位 `[a-z0-9]` | 首次安装自动生成并写入 Secret；升级不变 |
| `node.gcInterval` | `5m` | 孤儿 chunk GC 间隔 |
| `node.lifecycle.postStartReady` | `false` | 已关闭（节点进程启动时自动 ready；postStart 易与等待 0 号冲突） |
| `node.lifecycle.preStopDrain` | `true` | 缩容/重启前 drain |
| `node.podManagementPolicy` | `OrderedReady` | 按序启动：pod-0 Ready 后再起 pod-1、pod-2 |
| `dashboard.uriPrefix` | `/memoryfs` | 管理面板与 HTTP API 路径前缀 |
| `apiToken` | | 写操作 Bearer Token（可选） |
| `metrics.enabled` | `false` | 启用 ServiceMonitor |
| `mount.enabled` | `false` | 部署 FUSE DaemonSet（同镜像） |

示例：

```bash
helm upgrade memoryfs "${CHART}" -n memoryfs \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.storageGB=100
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
| `MEMORYFS_INSTANCE_ID` | 部署实例 ID（Secret 注入，hostPath 模式） |
| `MEMORYFS_STORAGE_ROOT` | hostPath 根目录（如 `/data/memoryfs`），与 instance ID、Pod 名组合数据路径 |
| `MEMORYFS_REPLICA_FACTOR` | Chunk 副本数 |
| `MEMORYFS_CHUNK_BACKEND` | disk / tiered / buffered / memory |
| `MEMORYFS_URI_PREFIX` | HTTP 路径前缀（如 `/memoryfs`） |
| `MEMORYFS_API_TOKEN` | API Bearer Token |

完整列表见 `deploy/scripts/node-start.sh`。

---

## 发布 Helm Chart

push 到 `master` 后，GitHub Actions 会自动：

1. 运行测试
2. 推送 `shaowenchen/memoryfs:latest` 镜像
3. 若已有 Release `latest` 则先删除，再打包 Helm Chart 为 `memoryfs-latest.tar.gz` 并发布到 [GitHub Releases / latest](https://github.com/shaowenchen/memoryfs/releases/tag/latest)

安装与升级使用同一包（无需打 git 版本 tag）：

```bash
CHART=https://github.com/shaowenchen/memoryfs/releases/download/latest/memoryfs-latest.tar.gz
helm upgrade --install memoryfs "${CHART}" -n memoryfs --create-namespace
```

也可使用 Makefile：`make helm-install`（Release）或 `make helm-install-local`（本地 Chart）。

---

## 生产 Checklist

- [ ] 至少 3 节点，RF=2
- [ ] 每节点 hostPath 目录（建议 SSD，`/data/memoryfs`）
- [ ] PDB `minAvailable: 2`
- [ ] preStop drain hook 已启用
- [ ] 定期 backup 元数据目录
- [ ] 监控 `/v1/stats`（disk_bytes、node_state）
- [ ] 网络策略：仅集群内可访问 19800/19802（HTTP/Raft）
- [ ] 滚动更新：Follower → Leader 顺序

---

## 故障处理

| 现象 | 处理 |
|------|------|
| Pod 一直 Pending | 节点是否已打 `memoryfs.io/node=true`；已标签节点数是否 ≥ `replicaCount`（每节点最多 1 个 Pod，硬约束） |
| `PostStartHookError`（1/2） | 旧 Chart postStart 在 HTTP 未就绪时执行；升级最新 Chart（已关闭 postStart，启动时自动 ready） |
| Pod `ContainerCreating` 卡住 | `kubectl describe pod memoryfs-0` 看 Events；节点 `mkdir -p /data/memoryfs` |
| `ImagePullBackOff` | 确认 `latest` 镜像可拉 |
| `CrashLoopBackOff`（1/2） | `kubectl logs memoryfs-1 --previous`；升级 Chart 后 follower 会等 0 的 `/health` 再启动 |
| `meta store: not leader` | 确保 memoryfs-0 先 Ready；`helm upgrade` 拉取含修复的新镜像 |
| 节点 `draining` 卡住 | 检查 peer 可达；必要时 `drain?force=true` |
| chunk 缺失 | `node-rebuild.sh` 或 restart（自动 ready） |
| Raft 日志 `stale raft peer addresses ... :8081` / Pod CrashLoop | hostPath 残留旧 Raft（`:8081`）；**各节点** `sudo rm -rf /data/memoryfs/<instanceId>/`（instanceId 见 Secret 或 Pod 日志），再 `kubectl delete pod -n memoryfs --all` 或重装 |
| Pod CrashLoop `invalid value "32b" for -disk-quota-gb` | `MEMORYFS_DISK_QUOTA_GB` 必须是纯数字；`helm upgrade` 拉最新 Chart，或 `helm get values` 检查 `node.storageGB` 勿带单位 |
| Raft heartbeat `connection refused` 某节点 `:19802` | 该 Pod 未监听 Raft（`kubectl logs memoryfs-N`）；在**该物理机** `ss -lntp | grep 19802`；节点间 **19802/TCP 必须互通**（安全组/防火墙） |
| memoryfs-1/2 `Running` 但 `0/1` Ready、日志停在 `entering follower state` | 旧版在 HTTP 就绪前阻塞 join；升级最新镜像后 Pod 会先起 `/health`，后台 re-join。同时查 `memoryfs-0` 是否 leader、`19802` 互通 |
| memoryfs-0 `0/1`、Raft 连不上其它节点 19802 | 多为旧 Raft 状态（3 节点配置但 1/2 未起）；删 Pod 让 1/2 跟上，仍失败则在各节点清 `/data/memoryfs/<instanceId>/` 后重装 |
| Raft 无 leader | 保证 quorum 节点在线；检查 19802 互通 |
| 磁盘满 | 调整 quota；`-max-file-age`；手动 GC |
| 扩容后 mount 不可见新节点 | 重启 mount 或换 nodes 节点；客户端启动时会刷新 `/v1/cluster/nodes` |

更多场景见 [docs/reference/USECASES.md](../docs/reference/USECASES.md)。
