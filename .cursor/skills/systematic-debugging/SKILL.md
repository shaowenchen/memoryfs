---
name: systematic-debugging
description: Use when encountering bugs, test failures, or unexpected behavior — before proposing fixes
---

# Systematic Debugging

## 四阶段（按顺序，不可跳过）

### 1. 复现

- 精确步骤、环境（K8s / nerdctl mount / 节点 IP）
- 收集日志：`nerdctl logs memoryfs-fuse`、`kubectl logs -n memoryfs`
- mount 客户端加 `-v` 看 chunk/meta 日志

### 2. 定位边界

- FUSE 层？`pkg/fusefs`
- 客户端 HTTP？`pkg/client`、`pkg/storage`
- 节点服务？`pkg/service`、`pkg/node`
- 集群/Raft？`pkg/raftnode`
- 部署？Helm values、hostNetwork、URI prefix

### 3. 形成假设

- 一次只改一个变量
- 用 `curl` 验证 HTTP API：`/health`、`/memoryfs/v1/cluster/overview`
- 区分 stale mount（Transport endpoint not connected）vs 写入失败

### 4. 验证修复

- 失败测试 → 修复 → 测试通过
- 在目标环境复现步骤确认
- invoke `verification-before-completion`

## MemoryFS 常见问题

| 现象 | 先查 |
|------|------|
| Transport endpoint not connected | mount 容器是否运行；fusermount -u stale |
| Operation not supported | FUSE Open 是否实现 |
| write hang | chunk URL prefix、gRPC fallback |
| connection refused | pod Ready、hostNetwork 端口 |
| join 404 | URI prefix `/memoryfs` |

## 禁止

- 未复现就改代码
- 同时改多处「试试看」
- 无测试的 bugfix（除非无法测，需说明）
