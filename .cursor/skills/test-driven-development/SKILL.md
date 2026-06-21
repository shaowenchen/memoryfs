---
name: test-driven-development
description: Use when implementing any feature or bugfix in Go — before writing production code
---

# Test-Driven Development (MemoryFS)

## Iron Law

```
NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST
```

先写了实现？删掉，从测试重来。

## Cycle

1. **RED** — `go test ./pkg/foo/... -run TestBar -v` → 必须 FAIL
2. **GREEN** — 最少代码 → PASS
3. **REFACTOR** — 保持绿灯

## Go 约定

- 测试文件：`*_test.go`，与被测包同目录
- 表驱动测试优先
- 集成测试可用 `httptest.NewServer`
- FUSE 相关：优先测 `pkg/fusefs`、`pkg/client`、`pkg/storage` 逻辑，避免必须 mount 的测试

## 示例

```go
func TestNormalizeGRPCStripsPath(t *testing.T) {
    got := normalizeGRPC("http://127.0.0.1:19800/memoryfs")
    want := "127.0.0.1:19801"
    if got != want {
        t.Fatalf("got %q want %q", got, want)
    }
}
```

## 验证清单

- [ ] 每个新行为有测试
- [ ] 亲眼看到测试失败（正确原因）
- [ ] `go test ./...` 全绿
- [ ] 无跳过的 hook（除非用户要求）

## Bug 修复

先写复现 bug 的失败测试，再修复。
