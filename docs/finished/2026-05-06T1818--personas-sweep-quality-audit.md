# Personas Sweep — Multi-Perspective Code Quality Audit

> Created: 2026-05-06 18:12 (CDT)
> Status: Done
> Finished: 2026-05-06 18:18 (CDT)
> Debt: [personas-sweep-quality-audit-debt](../2026-05-06T1818--personas-sweep-quality-audit-debt.md)

## Requirement

### 10-Persona Code Sweep with --fix

- **What**: Run all 10 persona lenses (Security Auditor, Performance Engineer, API Designer, Error Handling Specialist, Testing Advocate, Dependency Analyst, Documentation Reviewer, Maintainability Expert, Concurrency Specialist, UX/DX Advocate) against the mcp-cli-gateway codebase. Apply `--fix` for any actionable findings.
- **Where**: All source files under `internal/` and `cmd/`
- **Why**: Post-hardening quality assurance — verify the CWD security changes integrate cleanly and catch any pre-existing issues across the codebase.
- **How**: Read all source files, apply each persona's checklist from the /sniff smell checklist, fix inline where safe, document design limitations.
- **Priority**: Medium
- **Effort**: Low

## Context

Follows the CWD hardening work in `docs/finished/2026-05-06T1807--harden-cwd-validation.md`. All 11 source files were read and analyzed through 10 distinct quality lenses.

## Decisions

1. **3 inline fixes applied**: Dead code removal (`_ = i`), empty prompt validation (dispatch + batch), silent error → warning log (rebalance CountRunning).
2. **4 findings documented only**: PID tracking (architecture change needed), unnecessary mutex (harmless), missing package docs, redundant retry queries. These require design decisions outside sweep scope.

## Progress

| Phase | Status   | Notes |
| ----- | -------- | ----- |
| 1     | ✅ Done | Read all 11 source files |
| 2     | ✅ Done | Apply 10 persona lenses |
| 3     | ✅ Done | Fix 3 actionable issues |
| 4     | ✅ Done | Document 4 design limitations |
| 5     | ✅ Done | Verify: go vet + tests + build |

## Walkthrough

> Executed: 2026-05-06 18:18 (CDT)

### Plan vs Reality

| Phase | Planned | Outcome | Notes |
| ----- | ------- | ------- | ----- |
| 1 | Read codebase | ✅ Done | 11 files, ~2600 lines total |
| 2 | 10 persona analysis | ✅ Done | All perspectives applied |
| 3 | Fix findings | ✅ Done | 3 of 7 findings fixable inline |
| 4 | Document debt | ✅ Done | 4 design limitations filed |
| 5 | Verify | ✅ Done | All tests pass, build clean |

### Files Created / Modified

| File | Purpose/Change |
| ---- | -------------- |
| [roots.go](../../internal/server/roots.go) | Removed dead `_ = i` in `hasProjectMarker` for-range loop |
| [tools.go](../../internal/server/tools.go) | Added empty prompt validation for `gateway_dispatch` and `gateway_batch_dispatch` |
| [dispatch.go](../../internal/gateway/dispatch.go) | Changed silent error drop to warning log after bucket rebalance `CountRunning` |

### Decisions Made

1. **Dead code**: `_ = i` was leftover from pre-Go-1.22 syntax — safe to remove.
2. **Prompt validation**: Empty prompts waste dispatch cycles and trigger auto-retry — early reject with actionable error is better DX.
3. **Silent error**: `running, _ = ...` defaults to 0 on error, bypassing concurrency check — warning log preserves non-blocking flow while adding observability.
4. **PID tracking not fixed**: Requires `Executor` interface change (returns child PID) — architectural decision outside sweep scope.

### Open Debt

- PID tracking stores gateway PID, not child PID — `gateway_cancel` kills entire process
- Missing package-level doc comments across all packages
- Unnecessary `sync.Mutex` in `batch.go` (harmless, defensive)
- Redundant `CountRunning`/`CountPending` queries on retry iterations
