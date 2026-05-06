# Personas Sweep Debt — Design Limitations

> Created: 2026-05-06 18:18 (CDT)
> Status: Debt
> Source: [personas-sweep-quality-audit](../finished/2026-05-06T1818--personas-sweep-quality-audit.md)

## Items

### 1. PID Tracking Stores Gateway PID, Not Child PID

- **Where**: `internal/gateway/dispatch.go` L116-L160, `internal/gateway/commands.go` L310-L312
- **What**: `os.Getpid()` records the gateway's PID, not the spawned CLI agent's. `gateway_cancel` kills the entire gateway process.
- **Impact**: Canceling one job kills all in-flight jobs. The systemd service restarts, but all concurrent work is lost.
- **Fix**: Modify `Executor` interface to return child PID; store per-job process reference for targeted cancellation.
- **Priority**: Medium
- **Effort**: Medium

### 2. Missing Package-Level Doc Comments

- **Where**: All packages (`server`, `gateway`, `config`, `domain`, `pacing`, `provider`, `database`)
- **What**: No `// Package foo provides...` comments on `package` declarations.
- **Impact**: `go doc` output is unhelpful; godoc pages have no descriptions.
- **Fix**: Add package-level doc comments to each package.
- **Priority**: Low
- **Effort**: Low

### 3. Unnecessary sync.Mutex in batch.go

- **Where**: `internal/gateway/batch.go` L92
- **What**: Pre-allocated results slice with unique indices per goroutine. Mutex adds no correctness.
- **Impact**: Negligible performance overhead; no correctness issue.
- **Fix**: Remove mutex. Optional — harmless as-is.
- **Priority**: Low
- **Effort**: Low

### 4. Redundant DB Queries on Retry Iterations

- **Where**: `internal/gateway/dispatch.go` L75-L82
- **What**: `CountRunning` and `CountPending` called every retry iteration, but their results are only used on `attempt == 0`.
- **Impact**: Unnecessary DB reads on retries (rare path — negligible).
- **Fix**: Gate queries behind `attempt == 0` check.
- **Priority**: Low
- **Effort**: Low
