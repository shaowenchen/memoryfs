# Contributing

MemoryFS 采用 [Superpowers](https://github.com/obra/superpowers) 方法论进行 AI 辅助开发。

## 人类贡献者

1. Fork / 分支
2. 功能变更：先在 `docs/superpowers/specs/` 写设计（或 issue 讨论）
3. TDD：`go test ./...` 必须通过
4. PR 描述链接相关 spec/plan

## AI Agent

阅读 [AGENTS.md](../AGENTS.md)，遵循 `.cursor/skills/` 工作流：

```
brainstorming → writing-plans → TDD → verification → commit
```

## 目录约定

| 路径 | 用途 |
|------|------|
| `docs/reference/` | 稳定参考文档 |
| `docs/superpowers/specs/` | 功能设计 |
| `docs/superpowers/plans/` | 实现计划 |
| `.superpowers/sdd/` | 执行进度 |
| `.cursor/skills/` | Agent 行为规范 |

## 验证

```bash
make verify    # test + build
go test ./... -count=1
```

## 提交

- 消息聚焦 **why**
- 不 force push main
- 设计/计划文档随功能一并提交

参考：[Superpowers 方法论介绍](https://www.chenshaowen.com/blog/superpowers-software-engineering-methodology-for-ai.html)
