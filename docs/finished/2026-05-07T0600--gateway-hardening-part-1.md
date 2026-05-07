# Gateway Hardening — Part 1 of 3: Foundation & Correctness

> Created: 2026-05-07 06:00 (CDT)
> Status: Done
> Finished: 2026-05-07 06:25 (CDT)
> Source: /triage consolidation of `.supersweep_progress.md` + 4 finished debt docs
> Supersedes: `.supersweep_progress.md`, `2026-03-15T1350--aggregate-stats-in-sql.md`, `2026-03-15T1350--consolidate-test-helpers.md`, `2026-03-15T1350--decompose-dispatch-function.md`, `2026-03-15T1350--server-tool-handler-coverage.md`
> Prerequisite: None
> Next: [part-2](./2026-05-07T0600--gateway-hardening-part-2.md)

## Requirement

Foundation work that must compile and pass tests before any structural refactoring begins. These items introduce shared constants, fix a correctness bug, and repair a schema gap — all consumed by later phases.

---

### Phase 1: Status Constants SSOT — `domain` package

Items that establish typed constants consumed by every other package.

#### Status string constants (SA-02)

- **What**: Status literals `"running"`, `"waiting"`, `"queued"`, `"retrying"`, `"done"`, `"failed"`, `"cancelled"` appear as bare strings in ~40+ locations across `database/store.go`, `gateway/dispatch.go`, `gateway/commands.go`, `gateway/batch.go`, `domain/types.go`, and all test files. A single typo silently breaks filtering, query results, or state transitions.
- **Where**: `internal/domain/types.go` (new constants), then all consumers
- **Why**: Highest-impact SSOT gap in the codebase. Compile-time safety prevents an entire class of silent bugs.
- **How**:
  1. Add to `domain/types.go`:
     ```go
     // Request statuses.
     const (
         StatusQueued    = "queued"
         StatusWaiting   = "waiting"
         StatusRunning   = "running"
         StatusRetrying  = "retrying"
         StatusDone      = "done"
         StatusFailed    = "failed"
         StatusCancelled = "cancelled"
     )
     ```
  2. Replace every bare `"running"` → `domain.StatusRunning` etc. across all production code files.
  3. Replace test file literals where the value is used for status logic (DB inserts, assertions). Literal strings in test descriptions/messages can stay.
- **Priority**: High
- **Effort**: Low — mechanical find-and-replace, no logic changes
- **Source**: `.supersweep_progress.md` (SA-02)

**Phase Dependencies**: None
**Files Touched**: `domain/types.go`, `database/store.go`, `gateway/dispatch.go`, `gateway/commands.go`, `gateway/batch.go`, `server/tools.go`, + all test files
**Estimated Effort**: Low

---

### Phase 2: Correctness Fixes

#### `resolveTier` fallback dispatches to unavailable provider (CC-07)

- **What**: When no provider passes the availability check in `resolveTier()`, the function falls back to `aliases[0]` — which may be an unavailable (offline) provider. This can cause dispatch to a provider whose CLI binary doesn't exist, producing a confusing subprocess error.
- **Where**: `internal/gateway/dispatch.go:343-345`
- **Why**: The fallback defeats the purpose of the availability check loop. If all providers for a tier are down, the dispatch should fail with a clear error, not silently pick an unavailable one.
- **How**:
  1. Change the `len(candidates) == 0` branch to always return `""` (empty string):
     ```go
     if len(candidates) == 0 {
         return ""
     }
     ```
  2. In `Dispatch()`, the caller already has a `model, ok = ...` check after `resolveTier` — verify it handles the empty-string case and returns a descriptive "no available providers for tier X" error.
- **Priority**: High
- **Effort**: Low — 3-line change + verify caller handling
- **Source**: `.supersweep_progress.md` (CC-07)

#### Schema index misses 'queued' status (CC-16)

- **What**: Partial index `idx_requests_active` covers `WHERE status IN ('waiting', 'running', 'retrying')` but excludes `'queued'`. Multiple queries (`StatusCounts`, `ListActive`, `CountPending`) filter for `IN ('queued', 'waiting', 'running', 'retrying')` — the index can't serve these queries optimally.
- **Where**: `internal/database/schema.go:31-33`
- **Why**: The partial index was created before `queued` status existed (added during the queue-then-poll feature). Now `queued` rows miss the index, forcing table scans on the most frequent query path.
- **How**:
  1. Update the index definition in `SchemaSQL`:
     ```sql
     CREATE INDEX IF NOT EXISTS idx_requests_active
         ON requests(model, status)
         WHERE status IN ('queued', 'waiting', 'running', 'retrying');
     ```
  2. Add a migration in `runMigrations()` to drop and recreate the index for existing databases:
     ```go
     // Migration: recreate idx_requests_active to include 'queued' status.
     _, _ = s.db.ExecContext(ctx, "DROP INDEX IF EXISTS idx_requests_active")
     _, _ = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_requests_active
         ON requests(model, status)
         WHERE status IN ('queued', 'waiting', 'running', 'retrying')`)
     ```
- **Priority**: Medium
- **Effort**: Low — schema change + migration
- **Source**: `.supersweep_progress.md` (CC-16)

**Phase Dependencies**: Phase 1 (status constants must exist — migration code should use `domain.StatusQueued` etc.)
**Files Touched**: `gateway/dispatch.go`, `database/schema.go`, `database/store.go`
**Estimated Effort**: Low

---

### Phase 3: Idiomatic Go Quick Wins

Small, independent fixes that improve readability. All touch different files.

#### `strings.HasPrefix` instead of manual slice (CC-05)

- **What**: `alias[:len(prefix)] == prefix` → `strings.HasPrefix(alias, prefix)`
- **Where**: `internal/config/merge.go:86`
- **Priority**: Low | **Effort**: Low

#### `strconv.ParseFloat` instead of `fmt.Sscanf` (CC-11)

- **What**: `parseFloat` uses `fmt.Sscanf("%f", &v)` — replace with `strconv.ParseFloat(s, 64)` for idiomatic Go.
- **Where**: `internal/gateway/helpers.go:58-65`
- **How**: Replace body:
  ```go
  func parseFloat(s string) (float64, error) {
      v, err := strconv.ParseFloat(s, 64)
      if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
          return 0, fmt.Errorf("invalid number: %s", s)
      }
      return v, nil
  }
  ```
- **Priority**: Low | **Effort**: Low

#### `panic(msg)` instead of `panic(fmt.Sprintf(msg))` (CC-14)

- **What**: `panic(fmt.Sprintf("no CLI providers..."))` — no format args, use `panic("...")` directly.
- **Where**: `internal/provider/registry.go:59`
- **Priority**: Low | **Effort**: Low

#### Remove `-i` shell flag (CC-10)

- **What**: `exec.Command(shell, "-l", "-i", "-c", "echo $PATH")` — the `-i` flag forces interactive mode which may trigger `.bashrc` PS1 evaluation, alias expansion, and other side effects that pollute PATH output.
- **Where**: `internal/provider/resolve.go:26`
- **How**: Change to `exec.Command(shell, "-l", "-c", "echo $PATH")` — login shell alone provides the full PATH.
- **Priority**: Medium | **Effort**: Low

#### `init()` + `sync.Once` redundancy (CC-09)

- **What**: `init()` calls `ensureShellPATH()` which uses `sync.Once`. The `init()` is redundant since the `sync.Once` already guarantees single execution.
- **Where**: `internal/provider/resolve.go:15-17`
- **How**: Keep `init()` as the explicit call site (Go convention), but remove the `sync.Once` wrapper inside `ensureShellPATH` since `init()` guarantees single execution per process and the function is not exported. **OR** keep `sync.Once` and remove `init()` if lazy initialization is preferred. Either way, eliminate the redundancy.
- **Priority**: Low | **Effort**: Low

#### Named constant for ListFailed limit (CC-12)

- **What**: Hard-coded `20` → `const maxErrorResults = 20`.
- **Where**: `internal/gateway/commands.go:240`
- **Priority**: Low | **Effort**: Low

#### Extra blank line formatting (CC-18)

- **What**: Remove extra blank line at end of `resolveTier`.
- **Where**: `internal/gateway/dispatch.go:370`
- **Priority**: Low | **Effort**: Low

**Phase Dependencies**: None — independent of Phase 1 and 2 but logically sequenced after to avoid merge conflicts in shared files.
**Files Touched**: `config/merge.go`, `gateway/helpers.go`, `provider/registry.go`, `provider/resolve.go`, `gateway/commands.go`, `gateway/dispatch.go`
**Estimated Effort**: Low

---

## Implementation Sequence

| Phase | Description | Items | Files Touched | Dependencies | Parallelism |
|-------|-------------|-------|---------------|--------------|-------------|
| 1 | Status constants SSOT | SA-02 | domain/types.go + all consumers | None | sequential (one file defines, then replace across codebase) |
| 2 | Correctness fixes | CC-07, CC-16 | dispatch.go, schema.go, store.go | Depends: Phase 1 | parallel: dispatch.go vs schema.go |
| 3 | Idiomatic quick wins | CC-05, CC-10, CC-11, CC-12, CC-14, CC-09, CC-18 | merge.go, helpers.go, registry.go, resolve.go, commands.go, dispatch.go | After Phase 2 (avoids merge conflicts) | parallel:all |

## Verification Plan

### Automated Tests
```bash
go build ./...
go test ./internal/... -v
go vet ./...
```

### Manual Verification
- Grep for remaining bare status strings: `grep -rn '"running"\|"waiting"\|"queued"' internal/ --include='*.go' | grep -v '_test.go' | grep -v 'domain/types.go'` — should return zero production-code hits.
- Verify `resolveTier` returns empty when no providers available (check existing test or add one).

---

## Accountability Ledger

Every item from every input doc accounted for:

| ID | Original Item | Source Doc | Disposition | Destination |
|----|--------------|-----------|-------------|-------------|
| SA-02 | Status string constants | .supersweep_progress.md | Valid → Part 1, Phase 1 | part-1.md |
| CC-07 | resolveTier fallback | .supersweep_progress.md | Valid → Part 1, Phase 2 | part-1.md |
| CC-16 | Schema index misses queued | .supersweep_progress.md | Valid → Part 1, Phase 2 | part-1.md |
| CC-05 | strings.HasPrefix | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-10 | Remove -i shell flag | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-11 | strconv.ParseFloat | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-12 | Magic number 20 | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-14 | fmt.Sprintf static panic | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-09 | init+sync.Once redundancy | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-18 | Extra blank line | .supersweep_progress.md | Valid → Part 1, Phase 3 | part-1.md |
| CC-01 | Dispatch decomposition | .supersweep_progress.md | Valid → Part 2, Phase 1 | part-2.md |
| CC-02 | registerTools monolith | .supersweep_progress.md | Valid → Part 2, Phase 2 | part-2.md |
| CC-03 | Duplicated ID parsing | .supersweep_progress.md | Valid → Part 2, Phase 2 | part-2.md |
| CC-17 | Fire-and-forget helper | .supersweep_progress.md | Valid → Part 2, Phase 1 | part-2.md |
| UB-04 | Store interface composition | .supersweep_progress.md | Valid → Part 2, Phase 3 | part-2.md |
| CC-13 | ForEach error propagation | .supersweep_progress.md | Valid → Part 2, Phase 1 | part-2.md |
| SA-06 | Injectable clock | .supersweep_progress.md | Valid → Part 2, Phase 3 | part-2.md |
| SA-08 | Error strategy docs | .supersweep_progress.md | Valid → Part 2, Phase 1 | part-2.md |
| SA-09 | Batch goroutine limit | .supersweep_progress.md | Valid → Part 2, Phase 3 | part-2.md |
| UB-08 | Load balancer extraction | .supersweep_progress.md | Valid → Part 2, Phase 1 | part-2.md |
| UB-11 | MCPServer naming | .supersweep_progress.md | Valid → Part 2, Phase 2 | part-2.md |
| CC-04 | Merge double-iteration | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CC-06 | UpdatePacing SET clause | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CC-08 | LoadEnvOverrides helpers | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CC-15 | NowUnix precision | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CS-04 | AssignModelsForBatch simplify | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CS-05 | Repeated exec-duration calc | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CS-06 | ParseDuration branching | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CS-07 | Table-driven migrations | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| CS-08 | Redundant slice copy | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| UB-03 | Tool registration OCP | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| UB-09 | Stringly-typed column maps | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| SA-07 | Middleware/hook pattern | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| SA-10 | os.Setenv global state | .supersweep_progress.md | Valid → Part 3 | part-3.md |
| — | Decompose Dispatch | decompose-dispatch-function.md | Duplicate of CC-01 → Part 2 | obsolete.md |
| — | Aggregate Stats in SQL | aggregate-stats-in-sql.md | Valid → Part 3 | part-3.md |
| — | Consolidate test helpers | consolidate-test-helpers.md | Valid → Part 3 | part-3.md |
| — | Tool handler coverage | server-tool-handler-coverage.md | Valid → Part 2 | part-2.md |

---

## Walkthrough

> Executed: 2026-05-07 06:25 (CDT)

### Plan vs Reality

| Phase | Planned | Outcome | Notes |
|-------|---------|---------|-------|
| 1 — Status Constants SSOT | Add `domain.Status*` constants, replace ~40+ bare strings | ✅ Done | ~120+ replacements across 9 files (production + tests) |
| 2 — Correctness Fixes | Fix `resolveTier` fallback, rebuild schema index | ✅ Done | Migration added for existing DBs |
| 3 — Idiomatic Quick Wins | 7 independent refactors (CC-05/09/10/11/12/14/18) | ✅ Done | CC-11 introduced intentional stricter parsing |

### Files Created / Modified

| File | Purpose/Change |
|------|----------------|
| [types.go](../internal/domain/types.go) | Added `StatusQueued` through `StatusCancelled` constants |
| [store.go](../internal/database/store.go) | Migrated to domain constants + idempotent index migration |
| [schema.go](../internal/database/schema.go) | Added `'queued'` to `idx_requests_active` partial index |
| [dispatch.go](../internal/gateway/dispatch.go) | Domain constants, fixed `resolveTier` fallback, removed blank line |
| [commands.go](../internal/gateway/commands.go) | Domain constants + `defaultFailedLimit` named constant |
| [helpers.go](../internal/gateway/helpers.go) | `strconv.ParseFloat` replacing `fmt.Sscanf` |
| [merge.go](../internal/config/merge.go) | `strings.HasPrefix` replacing manual slice comparison |
| [registry.go](../internal/provider/registry.go) | Static panic string, removed unused `fmt` import |
| [resolve.go](../internal/provider/resolve.go) | Removed `-i` shell flag + `sync.Once` redundancy |
| [store_test.go](../internal/database/store_test.go) | Migrated functional status strings to domain constants |
| [commands_test.go](../internal/gateway/commands_test.go) | Migrated functional status strings to domain constants |
| [dispatch_test.go](../internal/gateway/dispatch_test.go) | Migrated status strings + updated `"1x"` test expectation |
| [poll_test.go](../internal/gateway/poll_test.go) | Migrated functional status strings to domain constants |

### Decisions Made

1. **SSOT scope**: Test files use constants for functional args (DB inserts, assertions) but keep literal strings in error messages/descriptions — readability over dogma.
2. **CC-11 behavioral change**: `strconv.ParseFloat` correctly rejects partial input like `"1x"` that `fmt.Sscanf` silently consumed. Updated test to reflect the stricter (correct) behavior rather than preserving the lenient bug.
3. **CC-09 resolution**: Chose to keep `init()` and inline the logic (removing `sync.Once`) since `init()` guarantees single execution and the function was unexported. This is the idiomatic Go approach.
4. **CC-14**: Replaced `panic(fmt.Sprintf(...))` with `panic(...)` rather than converting to error returns — the startup-time invariant check is correct use of panic.

### Open Debt

None. All 10 items from this Part completed.

