---
name: verification-before-completion
description: Use before claiming work is complete, fixed, or passing — require evidence
---

# Verification Before Completion

## Rule

**无证据，不宣称完成。**

## 必须提供的证据

| 声称 | 需要的证据 |
|------|------------|
| 测试通过 | `go test ./...` 完整输出（或相关包） |
| Bug 已修 | 失败测试 → 修复 → 通过的命令输出 |
| 可部署 | `go build ./cmd/...` 或 `helm template` 成功 |
| FUSE 可用 | mount 日志 + 宿主机 `ls`/`echo` 成功 |

## MemoryFS 检查清单

```bash
go test ./... -count=1
go build -o /dev/null ./cmd/memoryfs
# 若改 Helm：
helm template deploy/helm/memoryfs > /dev/null
```

## 禁止

- 「应该可以了」
- 只跑部分测试不说明
- 未读 linter 就提交（若改了代码）

## 提交

用户要求 commit 时：先 verification，再 `git commit`。见 `.cursor/rules/commit-after-fix.mdc`。
