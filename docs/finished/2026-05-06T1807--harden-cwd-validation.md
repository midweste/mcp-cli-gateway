# Harden CWD Validation — Prevent Dispatch Into Unsafe Directories

> Created: 2026-05-06 15:50 (CDT)
> Status: Done
> Finished: 2026-05-06 18:07 (CDT)

## Requirement

### Guard Against Root/Home Directory Dispatch

- **What**: When MCP roots are unavailable or empty, the gateway treats `cwd` as "unrestricted" — allowing agents to be dispatched into `/`, `/home/user`, `~/.claude`, or any other dangerous directory. Add independent safety layers that work without depending on the MCP client.
- **Where**: `internal/server/roots.go`, `internal/server/roots_test.go`
- **Why**: An autonomous agent with write access running in `/home/eric` or `/` could cause catastrophic damage. The mcp-proxy runs as a systemd service whose CWD defaults to `$HOME`. If `ResolveProjectRoot()` falls back to CWD, agents inherit that unsafe directory.
- **How**: Implement a multi-layer defense: (1) structural depth check to block shallow paths, (2) project marker requirement, (3) actionable error when cwd is missing and roots are unavailable.
- **Priority**: High
- **Effort**: Low

## Context

The gateway has two CWD resolution layers that interact:
1. **`ResolveProjectRoot()`** (startup) — resolves a fallback via `PROJECT_ROOT` env > git root from CWD > git root from exe dir > CWD. When the mcp-proxy systemd service starts the gateway, CWD is likely `$HOME`.
2. **`validateCwdAgainstRoots()`** (dispatch time) — calls `RequestRoots()` on the MCP client. If the client errors (proxy can't forward) → blocks dispatch. If roots are empty → treats as unrestricted.

The Antigravity client does not implement MCP `roots/list`, so roots are always empty or error. The current "unrestricted" fallback combined with a `$HOME`-based `ProjectRoot` means agents could run in the home directory.

## Decisions

1. **Structural depth check** (replaces exact deny-list): Block ALL single-segment paths (`/tmp`, `/opt`, `/var`, etc.) and ALL `/home/<user>` paths. No hardcoded deny-list to maintain — the structure is the guard.
2. **Project marker**: Walk up from cwd looking for `.git` (directory or file for worktrees). Max depth: 6 levels.
3. **Roots error handling**: Fall through to `isSafeCwd()` instead of blocking dispatch.
4. **Infrastructure dirs**: Not blocked — user dispatches into `~/.claude/mcp/*` projects provided they contain `.git`.
5. **Depth guard (numeric)**: Rejected — unreliable.
6. **Actionable errors**: Every rejection message tells the agent exactly what to do to fix the problem. Empty cwd + no roots returns a message instructing the agent to provide an explicit `cwd` with a `.git` project.

## Progress

| Phase | Status   | Notes |
| ----- | -------- | ----- |
| 1     | ✅ Done | Add `isSafeCwd()` with structural depth check + project marker |
| 2     | ✅ Done | Refactor `validateCwdAgainstRoots()` to fall through to `isSafeCwd()` |
| 3     | ✅ Done | Add comprehensive tests (35 tests) |

## Walkthrough

> Executed: 2026-05-06 18:07 (CDT)

### Plan vs Reality

| Phase | Planned | Outcome | Notes |
| ----- | ------- | ------- | ----- |
| 1 | Deny-list + marker | ✅ Done | Evolved: deny-list replaced with structural depth check |
| 2 | Refactor roots flow | ✅ Done | Added empty-cwd actionable error path |
| 3 | Tests | ✅ Done | 35 tests covering all layers |

### Files Created / Modified

| File | Purpose/Change |
| ---- | -------------- |
| [roots.go](../internal/server/roots.go) | Added `isDeniedByDepth`, `isSafeCwd`, `hasProjectMarker` (maxDepth=6), `cwdRequiredMsg`, refactored `validateCwdAgainstRoots` |
| [roots_test.go](../internal/server/roots_test.go) | 35 tests: depth checks, project markers, symlinks, fallthrough, actionable error messages |

### Decisions Made

1. **Structural over explicit**: Replaced 6-item deny-list with depth-based structural check — blocks ALL `/<single-segment>` and ALL `/home/<user>` without maintenance burden.
2. **Max depth 6**: User specified 6 as the absolute maximum for `.git` walk-up (was 50).
3. **Empty cwd = error when roots unavailable**: Instead of silently skipping validation, the gateway now returns an actionable error telling the agent to provide `cwd`.
4. **Error messages as agent guidance**: Every rejection error includes explicit instructions on what the agent should do differently (e.g., "Provide a 'cwd' pointing to a project directory with a .git folder").

### Open Debt

None.
