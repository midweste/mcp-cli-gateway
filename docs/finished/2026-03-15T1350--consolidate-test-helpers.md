# Consolidate Gateway Test Helpers

> Created: 2026-03-15 13:50 (local)
> Status: Debt

## Requirement

### Deduplicate newTestGateway / newFullTestGateway

- **What**: `newTestGateway` in `commands_test.go` and `newFullTestGateway` in `dispatch_test.go` are nearly identical — both create config, store, registry, seed pacing, and construct a Gateway. The only difference is `newFullTestGateway` accepts an `Executor` and creates a `pacing.Manager`, while `newTestGateway` passes `nil` for both.
- **Where**: [commands_test.go:17-37](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/gateway/commands_test.go#L17-L37), [dispatch_test.go:32-53](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/gateway/dispatch_test.go#L32-L53)
- **Why**: DRY violation. If the Gateway constructor signature or setup sequence changes, two places need updating. The existing `testutil` package already centralizes `NewTestStore` and `InsertRequest` but doesn't cover Gateway construction.
- **How**: Add `NewTestGateway(t, executor Executor) (*Gateway, *Store)` to the `testutil` package. Since `gateway.Executor` is an interface defined in `gateway.go`, `testutil` would need to import `gateway` — check for circular imports. If circular, define a `testutil.Executor` type alias or move the interface to `domain`. Alternatively, keep it in `gateway` package as a single `newTestGateway(t, exec Executor)` and call it from both test files (same-package helper).
- **Priority**: Low
- **Effort**: Low — straightforward deduplication with one import consideration
