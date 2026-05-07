# Gateway Hardening — Obsolete Items Registry

> Created: 2026-05-07 06:00 (CDT)
> Source: /triage of `.supersweep_progress.md` + finished debt docs

## Purpose

Items from input documents that are superseded, already resolved, or duplicated by other entries. Preserved for audit trail — no action required.

---

## Superseded Items

### `2026-03-15T1350--decompose-dispatch-function.md`

- **Status**: Superseded by CC-01 in Part 2
- **Reason**: This doc was marked `Status: Done` but the decomposition was never implemented. `dispatch.go` remains 404 lines. CC-01 in the supersweep rediscovered the same issue with more specific extraction guidance. The original doc's approach is subsumed by the Part 2 plan.
- **Action**: Move to `docs/finished/` (already there, keep as historical record)

---

## Previously Resolved Items

### PID-based self-termination risk

- **Status**: Resolved in session `2026-05-06T1828` (conversation `0856b224`)
- **Reason**: Job cancellation no longer uses PID-based process killing. Validated in finished doc `2026-05-06T1828--personas-sweep-quality-audit-debt.md`.

### Package-level documentation gaps

- **Status**: Resolved in session `2026-05-06T1828` (conversation `0856b224`)
- **Reason**: Package doc comments added to all `internal/` packages. Confirmed by audit.

### Unnecessary `sync.Mutex` in pacing store

- **Status**: Resolved in session `2026-05-06T1828` (conversation `0856b224`)
- **Reason**: Mutex removed from pacing store; SQLite serializes writes.

### Retry query gating (skip DB query on first attempt)

- **Status**: Resolved in session `2026-05-06T1828` (conversation `0856b224`)
- **Reason**: Dispatch now checks `attempt > 0` before querying for duplicates.

---

## Duplicate Items

### UB-06 (Adapter pattern for CLI providers) — duplicate of UB-04

- **Status**: Duplicate
- **Reason**: UB-06 proposed an adapter pattern for different CLI providers. The concrete recommendation (interface composition for Store in UB-04) covers the adapter concern. CLI provider adaptation is already handled by the existing `provider.Provider` interface.

### UB-07 (Repository pattern for DB layer) — already present

- **Status**: No action needed
- **Reason**: The `Store` struct already IS the repository pattern — it encapsulates all DB operations behind method calls. UB-04's interface composition formalizes this further.

### SA-01 (Missing model tier constants) — subsumed by SA-02

- **Status**: Duplicate
- **Reason**: SA-01 identified model tier names (`"lite"`, `"fast"`, `"deep"`) as bare strings. SA-02 covers all status strings comprehensively. Model tier strings could be added as a follow-up to SA-02 using the same pattern but are lower impact since tier names are constrained by config.

### SA-03 / SA-04 / SA-05 (Various architecture documentation gaps)

- **Status**: Low-value / informational only
- **Reason**: These were architecture observation items from the Senior Architect pass (ADR documentation, package diagram, etc.). They are documentation-only and don't represent code debt. If needed, they can be addressed as part of a broader documentation initiative.

---

## Disposition Summary

| Item | Source | Disposition |
|------|--------|-------------|
| Decompose Dispatch doc | finished/ debt doc | Superseded by CC-01 → Part 2 |
| PID self-term risk | prior audit | Already resolved (2026-05-06) |
| Package doc gaps | prior audit | Already resolved (2026-05-06) |
| Mutex removal | prior audit | Already resolved (2026-05-06) |
| Retry query gating | prior audit | Already resolved (2026-05-06) |
| UB-06 Adapter pattern | supersweep | Duplicate of UB-04 |
| UB-07 Repository pattern | supersweep | Already present in codebase |
| SA-01 Model tier constants | supersweep | Subsumed by SA-02 |
| SA-03/04/05 Arch docs | supersweep | Informational only, not code debt |
