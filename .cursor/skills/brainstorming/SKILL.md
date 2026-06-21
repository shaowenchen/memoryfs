---
name: brainstorming
description: MUST use before any creative work — new features, behavior changes, or modifications. Design before code.
---

# Brainstorming

## HARD-GATE

**禁止**在用户批准设计之前写代码、搭脚手架或调用实现类 skill。

## Checklist

1. 探索项目上下文（AGENTS.md、docs/reference/ARCHITECTURE.md、相关 pkg/、git log）
2. 一次只问一个澄清问题（目的、约束、成功标准）
3. 提出 2–3 种方案 + 权衡 + 推荐
4. 分块展示设计，每块等用户确认
5. 写入设计文档：`docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`
6. 用户审阅 spec 批准后，调用 `writing-plans` skill

## MemoryFS 设计要点

设计时必须考虑：

- 元数据走 Raft（leader 写、follower 转发）
- Chunk 多副本（RF、节点选路、repair）
- HTTP URI prefix（Helm 默认 `/memoryfs`）
- FUSE 客户端：HTTP-only chunk、mount 容器生命周期
- 现有包边界：`pkg/meta`、`pkg/service`、`pkg/fusefs`、`pkg/client`

## 设计文档模板

```markdown
# <Topic> Design

**Date:** YYYY-MM-DD
**Status:** draft | approved

## Goal
## Context
## Approaches Considered
## Recommended Design
### Architecture
### Data Flow
### Error Handling
### Testing Strategy
## Open Questions
```

## 下一步

设计批准后 → **仅** invoke `writing-plans`，不要直接写代码。
