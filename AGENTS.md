# MemoryFS — Agent Guide

AI 编程助手项目入口。遵循 [Superpowers 方法论](https://www.chenshaowen.com/blog/superpowers-software-engineering-methodology-for-ai.html)。

## 优先级

1. 用户显式指令
2. `.cursor/skills/` Superpowers skills
3. 默认系统提示

## 仓库结构

```
memoryfs/
├── cmd/                    # 二进制入口（node, mount, status, benchmark）
├── pkg/                    # 核心库
├── api/                    # protobuf / gRPC 定义
├── deploy/                 # Helm + 运维脚本
├── docs/
│   ├── reference/          # 稳定参考文档
│   └── superpowers/        # 设计 specs + 实现 plans
├── .cursor/skills/         # Agent 行为规范
├── .superpowers/sdd/       # SDD 执行进度
├── AGENTS.md               # 本文件
└── CONTRIBUTING.md         # 贡献流程
```

## 包职责

| 包 | 职责 |
|----|------|
| `pkg/meta`, `pkg/raftnode` | 元数据与 Raft |
| `pkg/service` | 节点业务逻辑 |
| `pkg/chunk` | 本地 chunk 存储 |
| `pkg/client`, `pkg/fusefs`, `pkg/storage` | FUSE 客户端 |
| `pkg/transport` | chunk HTTP/gRPC/RDMA |
| `pkg/mountlog` | mount 客户端日志 |

## 工作流

```
brainstorming → specs/ → writing-plans → plans/ → TDD → verification → commit
```

| 阶段 | Skill | 产出 |
|------|-------|------|
| 设计 | `brainstorming` | `docs/superpowers/specs/YYYY-MM-DD-*-design.md` |
| 计划 | `writing-plans` | `docs/superpowers/plans/YYYY-MM-DD-*.md` |
| 实现 | `test-driven-development` | 失败测试先行 |
| 调试 | `systematic-debugging` | 四阶段根因分析 |
| 完成 | `verification-before-completion` | 有证据才宣称完成 |
| 收尾 | `finishing-a-development-branch` | 合并/PR 决策 |

**HARD-GATE：** 新功能在用户批准设计前禁止写实现代码。

## 全局约束

- Go 1.22+，`go test ./...` 必须通过
- 最小 diff，匹配现有 `pkg/` 风格
- FUSE mount：HTTP-only chunk；URI prefix 默认 `/memoryfs`
- 基准架构见 `docs/superpowers/specs/2026-06-20-memoryfs-system-design.md`

## 常用命令

```bash
make verify                              # test + build（提交前）
go test ./pkg/<pkg>/... -run Test -v     # 单包测试
helm template deploy/helm/memoryfs         # 渲染 Chart
nerdctl logs -f memoryfs-mount           # mount 排查
```

## 关键文档

- [docs/reference/ARCHITECTURE.md](docs/reference/ARCHITECTURE.md)
- [docs/reference/MOUNT.md](docs/reference/MOUNT.md)
- [deploy/README.md](deploy/README.md)
- [docs/superpowers/README.md](docs/superpowers/README.md)

## Cursor

- Bootstrap：`.cursor/rules/using-superpowers.mdc`
- Skills：`.cursor/skills/*/SKILL.md`
- 可选插件：`/add-plugin superpowers`
