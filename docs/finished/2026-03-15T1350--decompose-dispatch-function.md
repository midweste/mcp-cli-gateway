# Decompose Dispatch Function

> Created: 2026-03-15 13:50 (local)
> Status: Done
> Finished: 2026-03-15 13:50 (CDT)

## Requirement

### Extract sub-functions from Dispatch()

- **What**: `Dispatch()` in `dispatch.go` is 260 lines with 7+ responsibilities — queue checking, bucket rebalancing, pacing wait, request insertion, Gemini CLI execution, rate-limit/retry handling, and output parsing. Extract into focused helper methods.
- **Where**: [dispatch.go](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/gateway/dispatch.go#L31-L294)
- **Why**: Violates SRP and "functions should do one thing" (Clean Code). Hard to unit test individual steps. High cognitive load for future maintainers. The retry loop contains nested control flow that makes the function difficult to reason about.
- **How**: Extract into private methods on `*Gateway`:
  1. `checkCapacity(ctx, model, alias) error` — queue/concurrency check + bucket rebalance
  2. `waitForPacing(ctx, model) (time.Duration, error)` — read pacing, compute wait, reserve slot
  3. `executeGemini(ctx, model, prompt, cwd, sandbox) (stdout, stderr string, exitCode int, err error)` — build command, run with timeout
  4. `handleResult(ctx, requestID, exitCode, stdout, stderr, attempt) (*DispatchResult, bool)` — parse output, detect rate limits, decide retry vs done
  Keep `Dispatch()` as the orchestrator calling these in sequence within the retry loop.
- **Priority**: Medium
- **Effort**: Medium — pure refactor, no behavioral change, existing tests should pass unchanged
