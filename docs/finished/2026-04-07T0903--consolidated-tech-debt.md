# Consolidated Tech Debt

> Created: 2026-04-07 08:46 (CDT)
> Status: Done
> Finished: 2026-04-07 09:03 (CDT)

## Items

### 1. Aggregate Stats in SQL

- **What**: `Stats()` in `commands.go:122-199` loads all completed requests into Go memory via `ListCompleted()`, then computes counts, averages, p95, and peak concurrency in-memory. Should use SQL aggregation.
- **Where**: `internal/gateway/commands.go`, `internal/database/store.go`
- **Why**: O(n) memory at scale. SQL can compute COUNT/AVG/SUM server-side.
- **Priority**: Low
- **Effort**: Medium

### 2. Consolidate Gateway Test Helpers

- **What**: `newTestGateway` in `commands_test.go:19` and `newFullTestGateway` in `dispatch_test.go:34` are near-identical. Both create config, store, registry, seed pacing, and construct a Gateway.
- **Where**: `internal/gateway/commands_test.go`, `internal/gateway/dispatch_test.go`
- **Why**: DRY violation. Two places to update when Gateway constructor changes.
- **Priority**: Low
- **Effort**: Low

### 3. Server Tool Handler Coverage

- **What**: `registerTools` in `tools.go` is at 49.4% coverage. About half the MCP tool handlers lack test coverage.
- **Where**: `internal/server/tools.go`, `internal/server/tools_test.go`
- **Why**: MCP tool handlers are the API boundary — untested boundary code risks regressions.
- **Priority**: Low
- **Effort**: Medium

### 4. Update README for Multi-Provider Architecture

- **What**: README says "Requirements: Gemini CLI" and config table only lists `GATEWAY_DB_PATH`. Missing 15+ env vars and 2 additional providers.
- **Where**: `README.md`
- **Why**: Stale docs mislead new users about multi-provider support.
- **Priority**: Low
- **Effort**: Low
