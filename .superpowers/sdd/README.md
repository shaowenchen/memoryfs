# Subagent-Driven Development 进度

本目录记录 Superpowers SDD 流程的执行进度。

## 文件约定

| 文件 | 内容 |
|------|------|
| `current-plan.md` | 当前正在执行的计划路径（符号链接或一行路径） |
| `task-NNN-status.md` | 各任务状态：pending / in_progress / done / blocked |
| `review-NNN.md` | 代码审查记录 |

## 示例

```
.superpowers/sdd/
├── README.md
├── current-plan.md          → ../../docs/superpowers/plans/2026-06-20-fuse-mount-fix.md
├── task-001-status.md       # done: add NodeOpener
├── task-002-status.md       # in_progress: HTTP-only chunk store
└── review-001.md
```

Agent 开始执行计划时，在此创建/更新状态文件；全部任务完成后清理或归档。
