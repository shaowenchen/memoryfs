---
name: writing-plans
description: Use when a spec is approved and you need an implementation plan before writing code
---

# Writing Plans

## Announce

"I'm using the writing-plans skill to create the implementation plan."

## Output

Save to: `docs/superpowers/plans/YYYY-MM-DD-<topic>.md`

## Plan Header (required)

```markdown
# [Feature] Implementation Plan

> **For agentic workers:** Use TDD per task. Track in `.superpowers/sdd/`.

**Goal:** [one sentence]
**Architecture:** [2-3 sentences]
**Tech Stack:** Go, go-fuse, Helm, …

## Global Constraints
- go test ./... must pass
- Minimal diff, match existing pkg/ conventions
- [from spec]

---
```

## Task Granularity

每个任务 2–5 分钟一步：

- Write failing test
- Run test → verify FAIL
- Minimal implementation
- Run test → verify PASS
- Commit

## Task Template

```markdown
### Task N: [Name]

**Files:**
- Modify: `pkg/.../file.go`
- Test: `pkg/.../file_test.go`

**Interfaces:**
- Consumes: …
- Produces: …

- [ ] Step 1: Write failing test
- [ ] Step 2: Run `go test ./pkg/... -run TestName -v` → expect FAIL
- [ ] Step 3: Minimal fix
- [ ] Step 4: Run test → expect PASS
- [ ] Step 5: Commit
```

## MemoryFS Commands

```bash
go test ./pkg/<package>/... -run TestName -v -count=1
go test ./... -count=1
go build ./cmd/...
```

## 禁止

- TBD / TODO / "add error handling" 等占位
- 无具体文件路径
- 无完整测试代码的任务

## 完成后

询问用户：逐任务 inline 执行，还是 subagent 流水线。然后 invoke `test-driven-development`。
