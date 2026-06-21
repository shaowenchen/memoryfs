# MemoryFS — Agent Guide

本文件是 AI 编程助手的项目入口。遵循 [Superpowers 软件工程方法论](https://www.chenshaowen.com/blog/superpowers-software-engineering-methodology-for-ai.html)。

## 优先级

1. **用户显式指令**（本文件、直接请求）— 最高
2. **`.cursor/skills/` 中的 Superpowers skills** — 覆盖默认行为
3. **默认系统提示** — 最低

## 项目概览

MemoryFS 是分布式文件系统：Raft 元数据 + 多副本 Chunk，支持 FUSE 挂载与 HTTP 管理面板。

| 层级 | 路径 | 说明 |
|------|------|------|
| 命令入口 | `cmd/node`, `cmd/mount`, `cmd/status`, `cmd/benchmark` | 二进制入口 |
| 核心库 | `pkg/meta`, `pkg/chunk`, `pkg/raftnode`, `pkg/service` | 元数据、存储、Raft、业务 |
| 客户端 | `pkg/client`, `pkg/fusefs`, `pkg/storage` | FUSE 与 HTTP 客户端 |
| 传输 | `pkg/transport` | HTTP / gRPC / RDMA chunk 传输 |
| 部署 | `deploy/helm`, `deploy/scripts` | Helm Chart 与运维脚本 |
| 文档 | `docs/` | 架构、CLI、用例 |
| 设计规格 | `docs/superpowers/specs/` | 功能设计文档 |
| 实现计划 | `docs/superpowers/plans/` | 细粒度任务计划 |
| 进度 | `.superpowers/sdd/` | SDD 执行进度 |

## 开发工作流（Superpowers）

```
brainstorming → writing-plans → TDD 实现 → verification → commit
```

| 阶段 | Skill | 产出 |
|------|-------|------|
| 设计 | `brainstorming` | `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md` |
| 计划 | `writing-plans` | `docs/superpowers/plans/YYYY-MM-DD-<topic>.md` |
| 实现 | `test-driven-development` | 先写失败测试，再写最少代码 |
| 调试 | `systematic-debugging` | 四阶段根因分析 |
| 完成 | `verification-before-completion` | 有证据才宣称完成 |

**HARD-GATE：** 新功能/行为变更在用户批准设计之前，禁止写实现代码。

## 全局约束

- **语言**：Go 1.22+，遵循现有 `pkg/` 包布局
- **测试**：`go test ./...`；新行为必须先有失败测试
- **提交**：修复/功能完成后 commit；用户未要求时不 push
- **范围**：最小 diff，不重构无关代码
- **FUSE mount**：客户端用 HTTP chunk I/O；节点 URL 需带 URI prefix（Helm 默认 `/memoryfs`）
- **文档**：设计/计划放 `docs/superpowers/`；架构细节见 `docs/ARCHITECTURE.md`

## 常用命令

```bash
go test ./...                          # 全量测试
go build -o bin/node ./cmd/node        # 编译节点
go build -o bin/mount ./cmd/mount      # 编译 FUSE 客户端
make help                              # Makefile 目标
helm template deploy/helm/memoryfs     # 渲染 Chart
```

## 关键文档

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — 架构与协议
- [docs/CLI.md](docs/CLI.md) — CLI 参数
- [deploy/README.md](deploy/README.md) — Helm 部署
- [docs/superpowers/README.md](docs/superpowers/README.md) — Superpowers 工作流说明

## Cursor 集成

- Bootstrap rule：`.cursor/rules/using-superpowers.mdc`（会话启动时加载）
- 项目 skills：`.cursor/skills/*/SKILL.md`
- 安装官方 Superpowers 插件（可选）：Agent 聊天中 `/add-plugin superpowers`
