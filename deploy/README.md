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
              │  StatefulSet (3+ 节点) + PVC           │
              │  Headless Service (稳定 DNS)           │
              │  PDB minAvailable=2                      │
              └────────────────────┬────────────────────┘
                                   │
         ┌────────────┬────────────┼────────────┬────────────┐
         ▼            ▼            ▼            ▼            ▼
      node-0       node-1       node-2       node-3 ...   (scale)
    Raft+Meta    Chunk+Meta   Chunk+Meta   Chunk+Meta
    tiered+PVC   tiered+PVC   tiered+PVC   tiered+PVC
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
| **tiered 后端** | 热读走内存，持久化走 PVC，重启可恢复 |
| **Headless Service** | 节点间 `-advertise-http/raft` 使用稳定域名 |
| **PDB minAvailable=2** | 滚动更新/节点维护时保持 quorum |
| **preStop drain** | 缩容/重启前把本地 chunk 复制到 peer |

### 部署方式对比

| 方式 | 适用 | 扩缩容 | 持久化 |
|------|------|--------|--------|
| **Helm / K8s** | 生产 | `helm upgrade --set replicaCount=N` | PVC |
| **docker-compose.cluster.yml** | 开发/小规模 | 手动加 service + join | Docker volume |
| **裸机 systemd** | 边缘/无 K8s | `scale-up.sh` / `scale-down.sh` | 本地磁盘 |

---

## 快速开始

### 方式 A：Docker Compose（3 节点）

```bash
cd deploy
cp .env.example .env
docker compose -f docker-compose.cluster.yml up -d

# 查看集群状态
chmod +x scripts/*.sh
./scripts/cluster-status.sh http://127.0.0.1:8080
```

挂载点：`deploy/mnt/`（mount 服务）

### 方式 B：Kubernetes Helm（推荐生产）

```bash
# 安装 3 节点集群
helm upgrade --install memoryfs ./deploy/helm/memoryfs \
  --namespace memoryfs --create-namespace \
  --set replicaCount=3 \
  --set replicaFactor=2 \
  --set node.persistence.size=100Gi

# 查看 Pod
kubectl -n memoryfs get pods -l component=node

# 集群状态（通过 Service）
kubectl -n memoryfs port-forward svc/memoryfs 8080:8080 &
./deploy/scripts/cluster-status.sh http://127.0.0.1:8080
```

启用 FUSE DaemonSet（每节点挂载）：

```bash
helm upgrade memoryfs ./deploy/helm/memoryfs \
  --set mount.enabled=true \
  --set mount.hostPath=/var/lib/memoryfs
```

---

## 扩缩容

### 扩容（Scale Up）

**K8s：**

```bash
# 1. 增加副本数
helm upgrade memoryfs ./deploy/helm/memoryfs --set replicaCount=5

# 2. 新 Pod 自动 join + ready + rebuild
kubectl -n memoryfs rollout status sts/memoryfs

# 3. 更新 FUSE mount 节点列表（若使用 DaemonSet，需更新 values 中 replicaCount 后 upgrade）
```

**Docker / 裸机：**

```bash
# 启动新节点进程/容器后
./deploy/scripts/scale-up.sh \
  http://n1:8080 \        # leader 或任意节点
  n4 \                    # 新节点 ID
  n4:8081 \               # raft 地址（需集群可达）
  http://n4:8080          # http 地址
```

### 缩容（Scale Down）

**原则：先 drain → leave → 再停进程/删 Pod**

```bash
# 1. 对要下线的节点执行
./deploy/scripts/scale-down.sh http://n4:8080 http://n1:8080

# 2. K8s 再降低 replicaCount（从最高 ordinal 开始删）
helm upgrade memoryfs ./deploy/helm/memoryfs --set replicaCount=3

# 3. 可选：保留 PVC 以备恢复；确认无数据后再删 PVC
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
# 手动控制（可选）
./deploy/scripts/rolling-update.sh http://memoryfs-0.memoryfs-headless:8080
```

### Docker Compose

```bash
# 依次重启 n3 → n2 → n1
docker compose -f deploy/docker-compose.cluster.yml restart n3
./deploy/scripts/node-ready.sh http://n3:8080
# ... 重复 n2, n1
```

---

## 备份与恢复

### 单节点备份

```bash
# Docker volume 路径或 K8s exec
./deploy/scripts/backup.sh /var/lib/docker/volumes/memoryfs_n1-data/_data

# K8s PVC
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

### 场景 1：节点换盘 / PVC 扩容

1. `node-drain.sh` 目标节点
2. 停止节点，挂载新盘，复制 `/data`
3. 启动 + `node-ready.sh`

### 场景 2：集群整体迁移到新 K8s

1. 对所有节点执行 backup
2. 在新集群部署相同 `replicaCount` 和节点 ID（StatefulSet ordinal 保持一致最佳）
3. restore 各节点 PVC
4. 按 ordinal 顺序启动 0 → 1 → 2
5. `cluster-status.sh` 验证

### 场景 3：跨云 chunk 迁移

依赖 RF：新节点 join 后 `ready` 自动从 peer pull 属于它的 chunk 副本，无需手动拷贝。

---

## 运维脚本一览

| 脚本 | 用途 |
|------|------|
| `cluster-status.sh` | 集群健康、leader、各节点 stats |
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

## 运维 API

| 接口 | 方法 | 用途 |
|------|------|------|
| `/dashboard` | GET | Web 运维面板 |
| `/metrics` | GET | Prometheus 指标 |
| `/v1/cluster/overview` | GET | 集群聚合状态 |
| `/v1/repair` | GET | 副本修复队列 |
| `/v1/repair/run` | POST | 手动触发副本修复 |
| `/health` | GET | 健康检查 + epoch |
| `/v1/stats` | GET | 节点存储统计 |
| `/v1/gc` | POST | 孤儿 chunk 清理 |

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
| `MEMORYFS_CHUNK_BACKEND` | disk / tiered / memory |

完整列表见 `deploy/scripts/node-start.sh`。

---

## 生产 Checklist

- [ ] 至少 3 节点，RF=2
- [ ] 每节点独立 PVC（建议 SSD）
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
| 节点 `draining` 卡住 | 检查 peer 可达；必要时 `drain?force=true` |
| chunk 缺失 | `node-rebuild.sh` 或 restart（自动 ready） |
| Raft 无 leader | 保证 quorum 节点在线；检查 8081 互通 |
| 磁盘满 | 调整 quota；`-max-file-age`；手动 GC |
| 扩容后 mount 不可见新节点 | 更新 mount `-nodes` 列表或 helm upgrade mount |

更多场景见 [docs/USECASES.md](../docs/USECASES.md)。
