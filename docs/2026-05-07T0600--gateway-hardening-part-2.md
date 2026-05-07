# Gateway Hardening — Part 2 of 3: Structural Refactoring

> Created: 2026-05-07 06:00 (CDT)
> Status: Planned
> Source: /triage consolidation of `.supersweep_progress.md` + finished debt docs
> Supersedes: See Part 1 accountability ledger
> Prerequisite: [part-1](./2026-05-07T0600--gateway-hardening-part-1.md) must be implemented and passing tests
> Next: [part-3](./2026-05-07T0600--gateway-hardening-part-3.md)

## Requirement

Structural refactoring that depends on Part 1's status constants and correctness fixes. These changes decompose complex functions, improve interface design, and add missing test coverage.

> [!IMPORTANT]
> Part 1 must be fully implemented and all tests passing before starting this part. The status constants introduced in Part 1 will be used throughout these refactors.

---

### Phase 1: Dispatch Decomposition

The highest-debt function in the codebase. Must be decomposed before other gateway refactoring.

#### Extract sub-functions from Dispatch (CC-01 / prior debt)

- **What**: `Dispatch()` is 280+ lines (404 total in file) with 5+ responsibilities: tier resolution, queue/concurrency checking, bucket rebalancing, pacing wait, request insertion, CLI execution, rate-limit retry, success handling, sandbox conflict retry, and failure handling. Deep nesting reaches 4-5 levels.
- **Where**: `internal/gateway/dispatch.go:29-308`
- **Why**: Violates SRP. Extremely hard to unit test individual steps. This was previously documented in `2026-03-15T1350--decompose-dispatch-function.md` (marked "Done" but the decomposition was never implemented — dispatch.go remains 404 lines).
- **How**: Extract into private methods on `*Gateway`:
  1. `checkCapacity(ctx, model, alias, maxConcurrent, maxQueue) (shouldQueue bool, err error)` — queue/concurrency check + bucket rebalance
  2. `waitForPacing(ctx, model) (time.Duration, error)` — read pacing, compute wait, reserve slot
  3. `executeAndParse(ctx, model, prompt, cwd, sandbox, prov) (stdout string, exitCode int, rateLimited bool, err error)` — build command, run with timeout, detect rate limit
  4. `handleSuccess(ctx, requestID, model, stdout, prov) *domain.DispatchResult` — parse output, update pacing
  Keep `Dispatch()` as the orchestrator calling these in sequence within the retry loop.
- **Priority**: High | **Effort**: Medium — pure refactor, existing tests must pass unchanged
- **Source**: `.supersweep_progress.md` (CC-01), `decompose-dispatch-function.md`

#### Extract `bestEffortUpdate` helper (CC-17)

- **What**: 8+ instances of `_ = g.store.UpdateStatus(ctx, id, status, fields)` in Dispatch. The intentional error-ignoring pattern should be explicit.
- **Where**: `internal/gateway/dispatch.go` (scattered)
- **How**: Add helper:
  ```go
  func (g *Gateway) bestEffortUpdate(ctx context.Context, id int64, status string, fields map[string]any) {
      if err := g.store.UpdateStatus(ctx, id, status, fields); err != nil {
          g.logger.Warn("best-effort update", "id", id, "status", status, "error", err)
      }
  }
  ```
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CC-17)

#### Document error strategy (SA-08)

- **What**: `DispatchResult.Error` vs the `error` return value semantics are undocumented. `DispatchResult.Error` represents user-visible operational failures (queue full, timeout, rate limit exhausted). The `error` return represents infrastructure failures (DB errors, context cancellation).
- **Where**: `internal/gateway/dispatch.go` — add doc comment block above `Dispatch()`
- **Priority**: Medium | **Effort**: Low
- **Source**: `.supersweep_progress.md` (SA-08)

#### Extract load balancer from resolveTier (UB-08)

- **What**: `resolveTier` (50 lines) reimplements load-balancing (find least-loaded alias, random tiebreak) inline.
- **Where**: `internal/gateway/dispatch.go:310-368`
- **How**: Could remain in dispatch.go as a private method but should be documented as load-balancing policy. Full extraction as a `LoadBalancer` interface is premature for current scale.
- **Priority**: Low | **Effort**: Low — rename and add doc comment for intent clarity
- **Source**: `.supersweep_progress.md` (UB-08)

#### Address ForEach error propagation (CC-13)

- **What**: `Status()`, `Pacing()`, `Stats()` use `registry.ForEach` callback which prevents error propagation — errors are logged but the caller sees success. This means partial results are silently returned.
- **Where**: `internal/gateway/commands.go:22, 96, 136`
- **How**: Two options:
  1. Document as intentional (status queries should return partial results on partial failure — losing one model's stats shouldn't fail the entire status response).
  2. Change ForEach to collect errors and return them alongside results.
  Option 1 is recommended — add a doc comment above each method explaining the partial-result contract.
- **Priority**: Medium | **Effort**: Low (if documenting) / Medium (if refactoring)
- **Source**: `.supersweep_progress.md` (CC-13)

**Phase Dependencies**: None (Part 1 must be done, but no intra-part dependency)
**Files Touched**: `gateway/dispatch.go`, `gateway/commands.go`
**Estimated Effort**: Medium

---

### Phase 2: Server Package Refactoring

#### Extract tool handlers from `registerTools` (CC-02)

- **What**: `registerTools()` is 350+ lines with all 9 tool handlers defined as inline closures. Each handler mixes schema definition with request handling logic.
- **Where**: `internal/server/tools.go:12-365`
- **How**: Extract each handler as a named method on `*MCPServer`:
  ```go
  func (s *MCPServer) handleDispatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { ... }
  func (s *MCPServer) handleBatchDispatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { ... }
  // etc.
  ```
  Keep `registerTools()` as the schema-only registration that wires handlers.
- **Priority**: Medium | **Effort**: Medium
- **Source**: `.supersweep_progress.md` (CC-02)

#### Extract `argInt64` helper (CC-03)

- **What**: Duplicated `float64 → int64` ID parsing in `gateway_retry` and `gateway_result`.
- **Where**: `internal/server/tools.go:320-324, 349-353`
- **How**: Add alongside existing `argStr`:
  ```go
  func argInt64(args map[string]any, key string) (int64, error) {
      v, ok := args[key].(float64)
      if !ok { return 0, fmt.Errorf("%s: required (number)", key) }
      return int64(v), nil
  }
  ```
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (CC-03)

#### Rename `MCPServer` → `Server` (UB-11)

- **What**: In the `server` package, `MCPServer` is redundant — the package name already provides context. Go convention is `server.Server`, not `server.MCPServer`.
- **Where**: `internal/server/server.go:14`
- **How**: Rename struct and update all references.
- **Priority**: Low | **Effort**: Low
- **Source**: `.supersweep_progress.md` (UB-11)

#### Add tool handler test coverage (prior debt)

- **What**: 9 MCP tool handlers have zero direct test coverage. The `server` package was at 6.6% coverage (only utility helpers tested).
- **Where**: `internal/server/tools_test.go`
- **Why**: These are the external API surface. Untested boundary code risks regressions.
- **How**: After handler extraction (CC-02), each named method can be tested independently with a mock Gateway. Start with read-only tools (status, jobs, pacing) as highest-value, lowest-risk.
- **Priority**: Medium | **Effort**: Medium
- **Source**: `server-tool-handler-coverage.md`

**Phase Dependencies**: Phase 1 (dispatch decomposition provides a pattern for handler extraction)
**Files Touched**: `server/server.go`, `server/tools.go`, `server/tools_test.go`
**Estimated Effort**: Medium

---

### Phase 3: Interface & Testability Improvements

#### Compose `gateway.Store` from sub-interfaces (UB-04)

- **What**: `gateway.Store` has 13 methods mixing reads, writes, and pacing. Consumers only use subsets.
- **Where**: `internal/gateway/gateway.go:14-32`
- **How**: Define sub-interfaces and embed:
  ```go
  type RequestReader interface {
      StatusCounts(ctx, model) (map[string]int, error)
      CountRunning(ctx, model) (int, error)
      CountPending(ctx, model) (int, error)
      ListActive(ctx) ([]Request, error)
      ListFailed(ctx, since float64, limit int) ([]Request, error)
      RunningModels(ctx) ([]string, error)
      GetByID(ctx, id int64) (*Request, error)
  }
  type RequestWriter interface {
      InsertRequest(ctx, req *Request) (int64, error)
      UpdateStatus(ctx, id, status, fields) error
  }
  type Store interface {
      RequestReader
      RequestWriter
      pacing.PacingStore
      CleanStalePIDs(ctx) error
  }
  ```
- **Priority**: Medium | **Effort**: Medium
- **Source**: `.supersweep_progress.md` (UB-04)

#### Injectable clock for domain.NowUnix (SA-06)

- **What**: `domain.NowUnix()` is a package-level function coupled to `time.Now()`. Tests depending on timestamps are non-deterministic.
- **Where**: `internal/domain/types.go:170-172`
- **How**: Add a settable clock function:
  ```go
  // Now returns the current time as a Unix timestamp. Override in tests for determinism.
  var Now func() float64 = NowUnix
  ```
  Tests can then set `domain.Now = func() float64 { return 1234567890.0 }`.
- **Priority**: Medium | **Effort**: Low
- **Source**: `.supersweep_progress.md` (SA-06)

#### Add goroutine limit to RunBatch (SA-09)

- **What**: `RunBatch` launches one goroutine per unique model in the batch with no limit. While practical limits are small (6-9 aliases), unbounded goroutine creation is a latent risk.
- **Where**: `internal/gateway/batch.go:94`
- **How**: Add a semaphore:
  ```go
  sem := make(chan struct{}, g.cfg.MAX_BATCH_GOROUTINES())
  // In goroutine: sem <- struct{}{}; defer func() { <-sem }()
  ```
  Default `MAX_BATCH_GOROUTINES` to 10 (well above typical 6-9 aliases).
- **Priority**: Medium | **Effort**: Low
- **Source**: `.supersweep_progress.md` (SA-09)

**Phase Dependencies**: Phase 2 (interface changes affect server tool handlers that reference Store)
**Files Touched**: `gateway/gateway.go`, `domain/types.go`, `gateway/batch.go`, `config/config.go`
**Estimated Effort**: Medium

---

## Implementation Sequence

| Phase | Description | Items | Files Touched | Dependencies | Parallelism |
|-------|-------------|-------|---------------|--------------|-------------|
| 1 | Dispatch decomposition | CC-01, CC-17, SA-08, UB-08, CC-13 | dispatch.go, commands.go | Part 1 complete | sequential (all in dispatch.go) |
| 2 | Server refactoring | CC-02, CC-03, UB-11, tool coverage | server/*.go | Phase 1 pattern | sequential (all in tools.go) |
| 3 | Interface & testability | UB-04, SA-06, SA-09 | gateway.go, types.go, batch.go | Phase 2 (interface consumed by server) | parallel:all |

## Verification Plan

### Automated Tests
```bash
go build ./...
go test ./internal/... -v -count=1
go vet ./...
# Coverage check after tool handler tests added:
go test ./internal/server/... -coverprofile=cover.out && go tool cover -func=cover.out | grep tools.go
```

### Manual Verification
- `wc -l internal/gateway/dispatch.go` — Dispatch() body should be <100 lines
- `wc -l internal/server/tools.go` — registerTools() should be <80 lines (schema-only)
- Verify `resolveTier("") ` case returns descriptive error (manual or test)
