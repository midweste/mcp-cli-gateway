# Supersweep Progress

> Target: @internal/
> Mode: default
> Started: 2026-05-06
> Skills: 4 relevant / 8 total

## Queue

| # | Command | Status | Findings | Changes |
|---|---------|--------|----------|---------|
| 1 | `/personas clean-code @internal/` | ✅ done | 18 | 0 |
| 2 | `/personas uncle-bob-craft @internal/` | ✅ done | 11 | 0 |
| 3 | `/personas code-simplification @internal/` | ✅ done | 8 | 0 |
| 4 | `/personas senior-architect @internal/` | ✅ done | 10 | 0 |

**Total raw findings: 47 | Deduplicated: 33 | After consolidation: 33**

---

## Cross-Cutting Themes

### Theme 1: `Dispatch()` is the primary debt hotspot
Findings CC-01, UB-01, CS-01, CS-02, CS-03 all converge on `gateway/dispatch.go:Dispatch()`. At 280+ lines with 4-5 nesting levels and 5+ responsibilities, this is the single highest-value refactor target.

### Theme 2: Stringly-typed status constants
Finding SA-02 is the most impactful SSOT gap. Status strings appear in ~20 locations across 4 packages with no compile-time protection against typos.

### Theme 3: Missing idiomatic Go patterns
Findings CC-05, CC-11, CC-14, CS-06 all point to non-idiomatic stdlib usage (`fmt.Sscanf` vs `strconv`, manual prefix matching vs `strings.HasPrefix`, etc.)

### Theme 4: `registerTools` monolith
Findings CC-02, UB-02, UB-03 converge on `server/tools.go`. The 350-line inline handler pattern violates SRP and OCP.

### Theme 5: Interface granularity
Findings UB-04, UB-05, SA-04 suggest the 13-method `gateway.Store` interface should be composed from smaller interfaces.

### Theme 6: Testability gaps
Findings SA-06, UB-07 highlight that `domain.NowUnix()` couples domain to the system clock, making time-dependent tests non-deterministic.

---

## Accumulated Findings

### Pass 1: clean-code (2026-05-06)

#### CC-01: `Dispatch` function is too long (280+ lines)
- **What**: Handles tier resolution, queue checking, bucket rebalancing, pacing wait, request insertion, CLI execution, rate-limit retry, success handling, sandbox conflict retry, and failure handling.
- **Where**: `internal/gateway/dispatch.go:29-308`
- **Why**: Violates SRP and "functions should do one thing". Extremely hard to unit test in isolation.
- **How**: Extract named methods: `resolveAlias()`, `enqueueIfBusy()`, `waitForPacing()`, `executeAndHandleResult()`.
- **Priority**: high | **Effort**: medium | **Skill**: clean-code | **Status**: open

#### CC-02: `registerTools` is a 350-line monolith
- **What**: Defines all 9 tool registrations with inline closures.
- **Where**: `internal/server/tools.go:12-365`
- **Why**: Too long. Hard to navigate and each handler is tested indirectly.
- **How**: Extract each tool handler as a named method on `MCPServer`.
- **Priority**: medium | **Effort**: medium | **Skill**: clean-code | **Status**: open

#### CC-03: Duplicated ID-parsing pattern in tools.go
- **What**: `float64 → int64` parsing duplicated in `gateway_retry` and `gateway_result`.
- **Where**: `internal/server/tools.go:320-324, 349-353`
- **Why**: DRY violation.
- **How**: Extract `argInt64(args, key) (int64, error)` helper.
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-04: `Merge` double-iterates providers
- **What**: Iterates providers twice — once for Models/pacing, again for Tiers.
- **Where**: `internal/config/merge.go:39-67`
- **Why**: Redundant alias computation.
- **How**: Build all maps in a single pass.
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-05: Manual prefix matching instead of `strings.HasPrefix`
- **What**: `alias[:len(prefix)] == prefix` instead of `strings.HasPrefix`.
- **Where**: `internal/config/merge.go:86`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-06: Inconsistent SET-clause building in store.go
- **What**: `UpdatePacing` uses manual first-flag; `UpdateStatus` initializes differently.
- **Where**: `internal/database/store.go:452-472 vs 401-418`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-07: `resolveTier` fallback to unavailable provider ⚠️
- **What**: Falls back to `aliases[0]` even when no candidates pass availability check.
- **Where**: `internal/gateway/dispatch.go:344-345`
- **Why**: Fallback defeats the availability check loop — could dispatch to offline provider.
- **How**: Return empty string, let caller handle "no available provider" error.
- **Priority**: high | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-08: `LoadEnvOverrides` inconsistent helper usage
- **What**: `QueuePollInterval` and `ProviderOrder` parsed inline while others use typed helpers.
- **Where**: `internal/config/config.go:112-124`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-09: `init()` + `sync.Once` redundancy in resolve.go
- **What**: `init()` wraps `ensureShellPATH()` which already uses `sync.Once`.
- **Where**: `internal/provider/resolve.go:15-17`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-10: Interactive shell flag `-i` may cause side effects
- **What**: `exec.Command(shell, "-l", "-i", "-c", ...)` uses `-i` which triggers `.bashrc`.
- **Where**: `internal/provider/resolve.go:26`
- **How**: Remove `-i`, keep `-l`.
- **Priority**: medium | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-11: `parseFloat` uses `fmt.Sscanf` instead of `strconv.ParseFloat`
- **Where**: `internal/gateway/helpers.go:58-65`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-12: Magic number 20 in ListFailed limit
- **Where**: `internal/gateway/commands.go:240`
- **How**: Add `const maxErrorResults = 20`.
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-13: Status callback swallows errors via ForEach
- **What**: `Status()`, `Pacing()`, `Stats()` use callback preventing error propagation.
- **Where**: `internal/gateway/commands.go:22, 96, 136`
- **Priority**: medium | **Effort**: medium | **Skill**: clean-code | **Status**: open

#### CC-14: `MustHaveProviders` uses `fmt.Sprintf` on static string
- **Where**: `internal/provider/registry.go:59`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-15: `NowUnix()` truncates sub-second precision
- **Where**: `internal/domain/types.go:170-172`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-16: Schema index misses 'queued' status ⚠️
- **What**: `idx_requests_active` excludes 'queued' but many queries include it.
- **Where**: `internal/database/schema.go:31-33`
- **Priority**: medium | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-17: Fire-and-forget UpdateStatus calls could use a helper
- **Where**: `internal/gateway/dispatch.go` (8+ instances)
- **How**: Extract `g.bestEffortUpdate(ctx, id, status, fields)`.
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

#### CC-18: Extra blank line at end of `resolveTier`
- **Where**: `internal/gateway/dispatch.go:370`
- **Priority**: low | **Effort**: low | **Skill**: clean-code | **Status**: open

### Pass 2: uncle-bob-craft (2026-05-06)

#### UB-03: Tool registration not open for extension
- **What**: Adding a tool requires modifying `registerTools()`. A slice-of-definitions pattern would be more OCP.
- **Where**: `internal/server/tools.go`
- **Priority**: low | **Effort**: medium | **Skill**: uncle-bob-craft | **Status**: open

#### UB-04: `gateway.Store` interface has 13 methods (too wide)
- **What**: Mixes read, write, and pacing operations.
- **Where**: `internal/gateway/gateway.go:14-32`
- **How**: Compose from `RequestReader`, `RequestWriter`, `PacingStore` sub-interfaces.
- **Priority**: medium | **Effort**: medium | **Skill**: uncle-bob-craft | **Status**: open

#### UB-08: Load balancing logic inline in dispatch
- **What**: `resolveTier` reimplements load balancing inline.
- **Where**: `internal/gateway/dispatch.go:310-370`
- **How**: Extract as a `LoadBalancer` strategy.
- **Priority**: low | **Effort**: medium | **Skill**: uncle-bob-craft | **Status**: open

#### UB-09: Stringly-typed column maps in UpdateStatus/UpdatePacing
- **What**: `map[string]any` column maps bypass compile-time safety.
- **Where**: `internal/database/store.go:401, 440`
- **How**: Consider typed update structs.
- **Priority**: low | **Effort**: medium | **Skill**: uncle-bob-craft | **Status**: open

#### UB-11: `MCPServer` name redundant with package
- **What**: In `server` package, `MCPServer` should just be `Server`.
- **Where**: `internal/server/server.go:14`
- **Priority**: low | **Effort**: low | **Skill**: uncle-bob-craft | **Status**: open

### Pass 3: code-simplification (2026-05-06)

#### CS-04: `AssignModelsForBatch` 3-branch conditional simplifiable
- **Where**: `internal/gateway/batch.go:51-67`
- **Priority**: low | **Effort**: low | **Skill**: code-simplification | **Status**: open

#### CS-05: Repeated exec-duration calculation
- **What**: Same `if startedAt != nil && finishedAt != nil` pattern in Jobs and Errors.
- **Where**: `internal/gateway/commands.go:73-77, 247-251`
- **How**: Extract `execDuration()` helper.
- **Priority**: low | **Effort**: low | **Skill**: code-simplification | **Status**: open

#### CS-06: `ParseDuration` redundant branching
- **What**: Short-string case subsumed by switch default.
- **Where**: `internal/gateway/helpers.go:27-55`
- **Priority**: low | **Effort**: low | **Skill**: code-simplification | **Status**: open

#### CS-07: Migrations use repeated pattern instead of table-driven
- **Where**: `internal/database/store.go:99-134`
- **How**: Define as `[]struct{table, col, sql}` and loop.
- **Priority**: low | **Effort**: low | **Skill**: code-simplification | **Status**: open

#### CS-08: Redundant slice copy in sortTiersByProviderOrder
- **Where**: `internal/config/merge.go:108-136`
- **Priority**: low | **Effort**: low | **Skill**: code-simplification | **Status**: open

### Pass 4: senior-architect (2026-05-06)

#### SA-02: Status string literals scattered with no typed constants ⚠️
- **What**: "running", "waiting", "queued", etc. appear in ~20 locations across 4 packages.
- **Where**: database/store.go, gateway/dispatch.go, gateway/commands.go, gateway/batch.go
- **How**: Define `const StatusRunning = "running"` etc. in `domain` package.
- **Priority**: high | **Effort**: low | **Skill**: senior-architect | **Status**: open

#### SA-06: `NowUnix()` couples domain to system clock
- **What**: No way to inject a clock for deterministic testing.
- **Where**: `internal/domain/types.go:170-172`
- **How**: Add `var Now func() float64 = NowUnix` in domain for test overrides.
- **Priority**: medium | **Effort**: low | **Skill**: senior-architect | **Status**: open

#### SA-07: No middleware/hook pattern for dispatch pipeline
- **Where**: `internal/gateway/dispatch.go`
- **Priority**: low (premature for current scale) | **Effort**: high | **Skill**: senior-architect | **Status**: open

#### SA-08: Mixed error strategies need documentation
- **What**: DispatchResult.Error vs error return semantics undocumented.
- **Where**: `internal/gateway/dispatch.go`
- **How**: Add error strategy doc comment.
- **Priority**: medium | **Effort**: low | **Skill**: senior-architect | **Status**: open

#### SA-09: `RunBatch` has no goroutine count limit
- **Where**: `internal/gateway/batch.go:94`
- **How**: Add semaphore or configurable max-goroutine limit.
- **Priority**: medium | **Effort**: low | **Skill**: senior-architect | **Status**: open

#### SA-10: `os.Setenv("PATH")` mutates global state
- **Where**: `internal/provider/resolve.go:54`
- **How**: Document as intentionally global. Already protected by sync.Once.
- **Priority**: low | **Effort**: low | **Skill**: senior-architect | **Status**: open

---

## Prioritized Remediation Plan

### Tier 1 — High Priority, High Impact

| ID | Finding | Effort | Theme |
|----|---------|--------|-------|
| SA-02 | Status string constants SSOT | low | SSOT |
| CC-07 | `resolveTier` fallback to unavailable provider | low | Correctness |
| CC-16 | Schema index misses 'queued' | low | Performance |

### Tier 2 — Medium Priority

| ID | Finding | Effort | Theme |
|----|---------|--------|-------|
| CC-01 | Dispatch decomposition | medium | Complexity |
| CC-10 | Remove `-i` shell flag | low | Reliability |
| SA-06 | Injectable clock | low | Testability |
| SA-08 | Error strategy documentation | low | Documentation |
| SA-09 | Batch goroutine limit | low | Safety |
| CC-13 | ForEach error propagation | medium | Error handling |
| UB-04 | Store interface composition | medium | ISP |
| CC-02 | registerTools decomposition | medium | Complexity |

### Tier 3 — Low Priority (Incremental)

All remaining 20 findings are low-effort improvements that can be addressed opportunistically during related work.

---

## Audit Summary

| Metric | Value |
|--------|-------|
| Total findings | 33 (deduplicated) |
| High priority | 3 |
| Medium priority | 8 |
| Low priority | 22 |
| Architecture | Sound — clean DIP, proper layer boundaries |
| Biggest risk | `resolveTier` fallback (CC-07) — potential incorrect dispatch |
| Highest debt | `Dispatch()` function complexity (CC-01) |
| Quick wins | SA-02, CC-07, CC-16, CC-05, CC-11, CC-14 |
