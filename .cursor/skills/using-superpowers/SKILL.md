---
name: using-superpowers
description: Use when starting any conversation — find and invoke project skills before ANY response or action
---

# Using Superpowers

## Rule

**Invoke relevant skills BEFORE any response or action.** Even 1% chance → read the skill file.

In Cursor: read `.cursor/skills/<skill-name>/SKILL.md` when a skill applies.

## Priority

1. User explicit instructions (AGENTS.md, direct requests)
2. Superpowers skills in `.cursor/skills/`
3. Default system behavior

## Red Flags

| Thought | Reality |
|---------|---------|
| "Just a simple question" | Questions are tasks. Check skills. |
| "Need more context first" | Skill check comes BEFORE clarifying. |
| "Let me explore the codebase" | Skills tell you HOW to explore. |
| "This skill is overkill" | Simple things become complex. Use it. |

## MemoryFS Entry Points

- [AGENTS.md](../../AGENTS.md) — project guide
- [docs/superpowers/README.md](../../docs/superpowers/README.md) — workflow
