# Personas Sweep Debt — Design Limitations

> Created: 2026-05-06 18:18 (CDT)
> Status: Done
> Finished: 2026-05-06 18:28 (CDT)
> Source: [personas-sweep-quality-audit](finished/2026-05-06T1818--personas-sweep-quality-audit.md)

## Items

### 1. PID Tracking Stores Gateway PID, Not Child PID

- **Where**: `internal/gateway/dispatch.go`, `internal/gateway/commands.go`
- **Status**: ✅ Fixed
- **Fix Applied**: Removed `os.Getpid()` storage (PID now defaults to 0). Removed `killProcess()` function and its SIGTERM/SIGKILL logic — Cancel now only marks DB status. Removed unused `os`, `syscall` imports and `gracefulShutdownDelay` constant.

### 2. Missing Package-Level Doc Comments

- **Where**: All packages (`server`, `gateway`, `config`, `domain`, `pacing`, `provider`, `database`, `testutil`)
- **Status**: ✅ Fixed
- **Fix Applied**: Created `doc.go` files with `// Package foo ...` comments for all 8 packages.

### 3. Unnecessary sync.Mutex in batch.go

- **Where**: `internal/gateway/batch.go`
- **Status**: ✅ Fixed
- **Fix Applied**: Removed `sync.Mutex` declaration, `mu.Lock()`, and `mu.Unlock()` calls. Each goroutine writes to a unique pre-allocated index — no synchronization needed.

### 4. Redundant DB Queries on Retry Iterations

- **Where**: `internal/gateway/dispatch.go` L73-82
- **Status**: ✅ Fixed
- **Fix Applied**: Moved `CountRunning` and `CountPending` calls inside `if attempt == 0` guard. Variables declared with zero-value defaults for retry iterations where they're unused.

## Walkthrough

> Executed: 2026-05-06 18:28 (CDT)

### Files Created / Modified

| File | Purpose/Change |
| ---- | -------------- |
| [dispatch.go](internal/gateway/dispatch.go) | Removed `os.Getpid()`, gated queries behind `attempt==0`, removed `os` import |
| [commands.go](internal/gateway/commands.go) | Removed `killProcess()` call, function, and unused imports (`os`, `syscall`) |
| [batch.go](internal/gateway/batch.go) | Removed unnecessary `sync.Mutex` |
| [config/doc.go](internal/config/doc.go) | [NEW] Package doc comment |
| [database/doc.go](internal/database/doc.go) | [NEW] Package doc comment |
| [domain/doc.go](internal/domain/doc.go) | [NEW] Package doc comment |
| [gateway/doc.go](internal/gateway/doc.go) | [NEW] Package doc comment |
| [pacing/doc.go](internal/pacing/doc.go) | [NEW] Package doc comment |
| [provider/doc.go](internal/provider/doc.go) | [NEW] Package doc comment |
| [server/doc.go](internal/server/doc.go) | [NEW] Package doc comment |
| [testutil/doc.go](internal/testutil/doc.go) | [NEW] Package doc comment |

### Open Debt

None — all 4 items resolved.
