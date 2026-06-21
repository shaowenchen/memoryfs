---
name: finishing-a-development-branch
description: Use when all tasks are done — verify, present merge/PR options, cleanup
---

# Finishing a Development Branch

## Checklist

1. **验证**
   ```bash
   make verify
   go test ./... -count=1
   ```

2. **变更摘要** — 列出修改文件、关联 spec/plan 路径

3. **呈现选项**（等用户选择）：
   - 合并到 master
   - 创建 PR（`gh pr create`）
   - 保留分支继续
   - 丢弃变更

4. **清理**
   - 删除 worktree（若用过）
   - 归档 `.superpowers/sdd/` 任务状态

## MemoryFS 注意

- 默认 push 到 `origin/master`（除非用户禁止）
- Helm/部署变更：提醒 `helm upgrade` 或镜像 rebuild
- FUSE 变更：提醒 `nerdctl pull` + 重启 mount 容器

## 禁止

- 测试未通过就 merge
- 未经用户确认 force push
