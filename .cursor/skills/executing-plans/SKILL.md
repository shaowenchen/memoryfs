---
name: executing-plans
description: Use when implementing an approved plan inline in the current session with checkpoints
---

# Executing Plans (Inline)

## When

用户批准 `docs/superpowers/plans/` 中的计划，选择 **inline 执行**（非 subagent 流水线）。

## Process

1. 读取计划，在 `.superpowers/sdd/current-plan.md` 记录路径
2. **逐任务**执行，不可跳过：
   - 读任务 Files / Interfaces
   - TDD：失败测试 → 实现 → 通过
   - `make verify` 或任务指定命令
   - 更新 `.superpowers/sdd/task-NNN-status.md`
3. 每 **3–5 个任务** 或遇到阻塞时，向用户汇报进度
4. 全部完成后 invoke `verification-before-completion`
5. invoke `finishing-a-development-branch`

## MemoryFS

```bash
go test ./pkg/<pkg>/... -run TestName -v -count=1
make verify
```

## 禁止

- 跳过失败测试
- 同时做多个未计划的任务
- 未验证就 commit
