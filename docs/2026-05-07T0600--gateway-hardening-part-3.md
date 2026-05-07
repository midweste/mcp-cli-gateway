# Gateway Hardening — Part 3 of 3: Polish, Performance & Future-Proofing

> Created: 2026-05-07 06:00 (CDT)
> Status: Planned
> Source: /triage consolidation of `.supersweep_progress.md` + finished debt docs
> Supersedes: See Part 1 accountability ledger
> Prerequisite: [part-2](./2026-05-07T0600--gateway-hardening-part-2.md) must be implemented and passing tests

## Requirement

Lower-priority optimizations, DRY cleanups, and structural improvements. Each item is independently valuable but not blocking other work. Sequenced last because none are urgent and some build on Part 2's refactored interfaces.

> [!NOTE]
> Items in this part can be cherry-picked individually. The phases represent logical groupings, not hard dependencies within the part.

---

### Phase 1: Config & SQL Optimizations

#### Merge double-iteration in `mergeModels` (CC-04)

- **What**: `mergeModels` iterates `overrides` twice — once to build a map, once to merge. Could combine into a single pass.
- **Where**: `internal/config/merge.go:38-78`
- **How**: Merge both loops into a single range:
  ```go
  for name, override := range overrides {
      base, exists := result[name]
      if !exists {
          result[name] = override; continue
      }
      // merge fields...
  }
  ```
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CC-04)

#### Extract `LoadEnvOverrides` sub-helpers (CC-08)

- **What**: `LoadEnvOverrides()` is 60+ lines mixing parsing logic for different env var types (strings, ints, durations, maps).
- **Where**: `internal/config/env.go:14-80`
- **How**: Extract `parseEnvInt`, `parseEnvMap`, etc.
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CC-08)

#### Build SQL SET clause dynamically in UpdatePacing (CC-06)

- **What**: `UpdatePacing` always `SET`s all 6 pacing columns even when only 1-2 changed.
- **Where**: `internal/database/store.go:309-319`
- **How**: Accept changed-field map and build SET dynamically. Consider if complexity is worth it — the current approach is clear and SQLite handles the redundant writes cheaply.
- **Priority**: Low | **Effort**: Medium
- **Source**: `.supersweep_progress.md` (CC-06)

#### Table-driven migrations (CS-07)

- **What**: Sequential `ExecContext` calls in `runMigrations` → table-driven list.
- **Where**: `internal/database/store.go:81-120`
- **How**:
  ```go
  migrations := []struct{ name, sql string }{
      {"add prompt_hash", `ALTER TABLE requests ...`},
      {"add label", `ALTER TABLE requests ...`},
      // ...
  }
  for _, m := range migrations {
      if _, err := s.db.ExecContext(ctx, m.sql); err != nil {
          // SQLite ALTER errors for already-existing columns are expected
      }
  }
  ```
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CS-07)

#### Move stats aggregation to SQL (prior debt)

- **What**: `Stats()` loads all completed requests into Go memory, then computes counts/averages/p95/peak in-memory. For 10K+ jobs, this creates allocation pressure.
- **Where**: `internal/gateway/commands.go:122-199`, `internal/database/store.go` (ListCompleted)
- **How**: Replace with SQL aggregate query (COUNT, AVG, SUM with CASE). P95 needs SQLite window functions or approximation.
- **Priority**: Low | **Effort**: Medium
- **Source**: `aggregate-stats-in-sql.md`

**Phase Dependencies**: None within part; Part 2 should be done first
**Files Touched**: `config/merge.go`, `config/env.go`, `database/store.go`, `gateway/commands.go`
**Estimated Effort**: Medium (aggregate stats), Low (others)

---

### Phase 2: Control Flow & DRY

#### Simplify ParseDuration branching (CS-06)

- **What**: `parseDuration` has nested branches with repeated `parseFloat` calls.
- **Where**: `internal/gateway/helpers.go:16-56`
- **How**: Use a suffix-to-multiplier map:
  ```go
  multipliers := map[byte]float64{'h': float64(time.Hour), 'm': float64(time.Minute), 'd': 24*float64(time.Hour)}
  ```
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CS-06)

#### Remove redundant slice copy in `getProvider` (CS-08)

- **What**: `getProvider` copies the aliases slice before returning; callers don't mutate.
- **Where**: `internal/provider/registry.go:47-51`
- **How**: Return the slice directly. If mutation safety is needed, document it.
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CS-08)

#### Simplify `AssignModelsForBatch` (CS-04)

- **What**: Sequential `registry.Provider(model)` calls per item → pre-compute unique models first, look up once.
- **Where**: `internal/gateway/batch.go:14-42`
- **How**: Build a `seen map[string]string` and short-circuit lookups.
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CS-04)

#### DRY exec-duration calculation (CS-05)

- **What**: `startedAt - createdAt` and `finishedAt - startedAt` computed in 3+ places.
- **Where**: `internal/gateway/dispatch.go`, `internal/gateway/commands.go`
- **How**: Add `domain.ExecDuration(r *Request) float64` helper.
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CS-05)

#### Consolidate test gateway helpers (prior debt)

- **What**: `newTestGateway` (commands_test.go) and `newFullTestGateway` (dispatch_test.go) are nearly identical.
- **Where**: `internal/gateway/commands_test.go:17-37`, `internal/gateway/dispatch_test.go:32-53`
- **How**: Merge into a single helper in a shared test file or `testutil` package. Watch for circular imports.
- **Priority**: Low | **Effort**: Low
- **Source**: `consolidate-test-helpers.md`

#### `NowUnix` int vs float precision (CC-15)

- **What**: `NowUnix()` returns `float64(time.Now().Unix())` — the Unix() call already truncates to seconds, then wraps in float64. If sub-second precision is needed, use `float64(time.Now().UnixMicro()) / 1e6`.
- **Where**: `internal/domain/types.go:170-172`
- **How**: Evaluate if sub-second precision matters for gateway timing. If yes, update to microseconds. If no, document the intent.
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CC-15)

**Phase Dependencies**: CS-05 depends on Part 2 dispatch decomposition being in place
**Files Touched**: `gateway/helpers.go`, `provider/registry.go`, `gateway/batch.go`, `domain/types.go`, test files
**Estimated Effort**: Low

---

### Phase 3: Architectural Patterns (Optional / Future)

These are forward-looking improvements. Only implement if the codebase grows to warrant the added abstraction.

#### Tool registration via OCP pattern (UB-03)

- **What**: `registerTools` is a single monolithic function. Adding a tool requires editing one function. An OCP pattern would have each tool register itself.
- **Where**: `internal/server/tools.go`
- **How**: Each tool could be a `ToolHandler` interface with `Schema()` and `Handle()`. Registration iterates a slice. Consider if this is warranted for 9 tools.
- **Priority**: Low | **Effort**: Medium
- **Source**: `.supersweep_progress.md` (UB-03)
- **Gate**: Only implement if tool count exceeds 15 or if tools need to be dynamically enabled/disabled.

#### Replace string-keyed column maps with typed structs (UB-09)

- **What**: `map[string]any` used for DB update fields means column names are unchecked strings.
- **Where**: `internal/database/store.go:137-150`
- **How**: Define an `UpdateFields` struct with optional fields:
  ```go
  type UpdateFields struct {
      Status    *string
      Stdout    *string
      ExitCode  *int
      // ...
  }
  ```
  Build SET clause from non-nil fields.
- **Priority**: Low | **Effort**: Medium
- **Source**: `.supersweep_progress.md` (UB-09)

#### Middleware/hook pattern for Dispatch (SA-07)

- **What**: Cross-cutting concerns (logging, pacing, metrics) could be injected via middleware rather than hardcoded in Dispatch.
- **Where**: `internal/gateway/dispatch.go`
- **How**: Premature for current scale. Document as a future consideration.
- **Priority**: Low | **Effort**: High
- **Source**: `.supersweep_progress.md` (SA-07)
- **Gate**: Only implement if dispatch has 3+ cross-cutting concerns that change independently.

#### Isolate `os.Setenv` side effect in `resolve.go` (SA-10)

- **What**: `ensureShellPATH()` mutates global `os.Environ` via `os.Setenv("PATH", ...)`.
- **Where**: `internal/provider/resolve.go:54`
- **How**: Return the augmented PATH and pass it via `cmd.Env` in `provider.Run()` instead of global mutation. Requires changing `Run()` to accept env overrides.
- **Priority**: Low | **Effort**: Medium
- **Source**: `.supersweep_progress.md` (SA-10)

**Phase Dependencies**: Phase 2 of this part; Part 2 must be done
**Files Touched**: `server/tools.go`, `database/store.go`, `gateway/dispatch.go`, `provider/resolve.go`
**Estimated Effort**: Medium-High (if implementing) / Low (if documenting as future)

---

## Implementation Sequence

| Phase | Description | Items | Dependencies | Parallelism |
|-------|-------------|-------|--------------|-------------|
| 1 | Config & SQL | CC-04, CC-08, CC-06, CS-07, SQL-stats | Part 2 complete | parallel:all |
| 2 | Control flow & DRY | CS-06, CS-08, CS-04, CS-05, test-helpers, CC-15 | Part 2 complete | parallel:all |
| 3 | Architectural (optional) | UB-03, UB-09, SA-07, SA-10 | Part 2 + Phase 1-2 | case-by-case |

## Verification Plan

### Automated Tests
```bash
go build ./...
go test ./internal/... -v -count=1
go vet ./...
```

### Manual Verification
- Benchmark stats aggregation before/after SQL migration (if implemented)
- Verify no new lint warnings with `golangci-lint run ./internal/...`
