# Superpowers 工程方法论

MemoryFS 采用 [Superpowers](https://github.com/obra/superpowers) 方法论组织 AI 辅助开发：先设计、再计划、TDD 实现、有证据才完成。

参考：[Superpowers：给 AI 编程助手一套完整的软件工程方法论](https://www.chenshaowen.com/blog/superpowers-software-engineering-methodology-for-ai.html)

## 目录

| 路径 | 用途 |
|------|------|
| [specs/](specs/) | 功能设计（brainstorming 产出） |
| [plans/](plans/) | 实现计划（writing-plans 产出） |
| [specs/2026-06-20-memoryfs-system-design.md](specs/2026-06-20-memoryfs-system-design.md) | 系统基准规格（approved） |
| [../reference/](../reference/) | 稳定参考文档（架构、CLI、挂载） |

## 工作流

```
1. brainstorming   → specs/YYYY-MM-DD-<topic>-design.md
2. writing-plans   → plans/YYYY-MM-DD-<topic>.md
3. TDD 实现        → go test，逐任务提交
4. verification    → 运行测试/命令，贴出证据
5. finishing       → commit / PR / 合并决策
```

## 文件命名

- 设计：`YYYY-MM-DD-<kebab-topic>-design.md`
- 计划：`YYYY-MM-DD-<kebab-topic>.md`

## SDD 进度

子 agent 驱动开发时，进度记录在 [`.superpowers/sdd/`](../../.superpowers/sdd/README.md)。

## Skills 位置

| 类型 | 路径 |
|------|------|
| 项目 skills | `.cursor/skills/` |
| Cursor 内置 | `~/.cursor/skills-cursor/` |
| 官方 Superpowers 插件 | 通过 `/add-plugin superpowers` 安装 |
