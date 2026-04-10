# 3-Tier Refactor, Provider Ordering & Env-Aware Config

> Created: 2026-04-07 08:00 (CDT)
> Status: Done
> Finished: 2026-04-07 08:33 (CDT)

## Requirement

### Model-Agnostic 3-Tier Architecture

- **What**: Remove all hardcoded model names and pacing from core codebase; move to providers
- **Where**: `internal/config/config.go`, `internal/provider/`, all test files
- **Why**: Model-specific code scattered across the core makes adding providers and updating models fragile
- **How**: Expand Provider interface with `TierModels()` and `ModelPacing()`, make `Merge()` build config dynamically
- **Priority**: High
- **Effort**: Medium

### Provider Ordering

- **What**: Add `GATEWAY_PROVIDER_ORDER` env var to control which provider gets requests first per tier
- **Where**: `internal/config/config.go`, `internal/config/merge.go`, `cmd/main.go`, `.env`
- **Why**: When loads are equal, the gateway needs a deterministic way to prefer one provider over another
- **How**: Sort tier aliases by provider priority in `sortTiersByProviderOrder()`, read from env
- **Priority**: Medium
- **Effort**: Low

### Env-Aware Config Class

- **What**: Centralize all `GATEWAY_*` env var reading in Config, use ALL_CAPS accessor methods
- **Where**: `internal/config/config.go`, all callers (dispatch.go, manager.go, store.go, main.go)
- **Why**: Env parsing was scattered in main.go; callers couldn't tell which values were env-overridable
- **How**: `LoadEnvOverrides()` reads 15 env vars; ALL_CAPS methods signal env-awareness at call sites
- **Priority**: Medium
- **Effort**: Medium

### Config File Separation

- **What**: Split monolithic config.go into focused files
- **Where**: `internal/config/config.go` → `config.go` + `merge.go` (existing `paths.go`)
- **Why**: config.go mixed struct definition, env loading, merge logic, tier sorting, and alias helpers
- **How**: Extract merge/tier/sort logic into `merge.go`; config.go owns struct + defaults + env + accessors
- **Priority**: Low
- **Effort**: Low

## Progress

| # | Item | Status |
|---|------|--------|
| 1 | Model-agnostic 3-tier architecture | ✅ Done |
| 2 | Provider ordering (GATEWAY_PROVIDER_ORDER) | ✅ Done |
| 3 | Env-aware Config class | ✅ Done |
| 4 | Config file separation | ✅ Done |

## Walkthrough

> Executed: 2026-04-07 08:33 (CDT)

### Plan vs Reality

| Phase | Planned | Outcome | Notes |
|-------|---------|---------|-------|
| 1 | Model-agnostic config | ✅ Done | Expanded Provider interface with TierModels()/ModelPacing(), removed all model names from core |
| 2 | Provider ordering | ✅ Done | GATEWAY_PROVIDER_ORDER env var, sortTiersByProviderOrder(), 4 test cases |
| 3 | Env-aware Config | ✅ Done | LoadEnvOverrides() covers 15 vars, ALL_CAPS accessors, all callers migrated |
| 4 | File separation | ✅ Done | config.go (192 lines) + merge.go (150 lines) + paths.go — clean SRP |

### Files Created / Modified

| File | Purpose/Change |
|------|---------------|
| [config.go](../internal/config/config.go) | Rewritten: struct, defaults, LoadEnvOverrides(), ALL_CAPS accessors, env helpers |
| [merge.go](../internal/config/merge.go) | NEW: ProviderDescriptor interface, Merge(), tier sorting, alias generation |
| [config_test.go](../internal/config/config_test.go) | Added TestLoadEnvOverrides (int/float/string/list/defaults) |
| [merge_test.go](../internal/config/merge_test.go) | Added TestProviderOrder (4 cases: gemini-first, claude-first, alpha, partial) |
| [dispatch.go](../internal/gateway/dispatch.go) | Migrated to ALL_CAPS accessors (PROJECT_ROOT, MAX_RETRIES, TIMEOUT_SECONDS, QUEUE_POLL_INTERVAL) |
| [manager.go](../internal/pacing/manager.go) | Migrated to ALL_CAPS accessors (STREAK_THRESHOLD, SPEEDUP_FACTOR, etc.) |
| [store.go](../internal/database/store.go) | Migrated to ALL_CAPS accessors (DB_PATH, CLEANUP_DAYS) |
| [main.go](../cmd/mcp-gemini-gateway/main.go) | Uses LoadEnvOverrides(), ALL_CAPS accessors throughout |
| [.env](../.env) | Comprehensive: all 15+ GATEWAY_* vars documented |
| [.env.example](../.env.example) | Same as .env — full reference |
| [gemini.go](../internal/provider/gemini.go) | Added TierModels() and ModelPacing() to GeminiProvider |
| [codex.go](../internal/provider/codex.go) | Added TierModels() and ModelPacing() to CodexProvider |
| [claude.go](../internal/provider/claude.go) | Added TierModels() and ModelPacing() to ClaudeProvider |
| [provider.go](../internal/provider/provider.go) | Expanded interface: TierModels(), ModelPacing() |
| [server.go](../internal/server/server.go) | Model-agnostic tier descriptions |
| [testutil.go](../internal/testutil/testutil.go) | Updated MockProvider for new interface |
| [dispatch_test.go](../internal/gateway/dispatch_test.go) | Updated local mock for new interface |
| [store_test.go](../internal/database/store_test.go) | Local dbTestProvider for new interface |
| [batch_test.go](../internal/gateway/batch_test.go) | Adjusted for 3-tier bucket counts |
| [commands_test.go](../internal/gateway/commands_test.go) | Adjusted pacing count for 3-tier |

### Decisions Made

1. **3 tiers (lite/fast/deep) over 5**: Reduced from 5 to 3 canonical tiers — multi-model support per tier handles capacity
2. **Provider ordering by env**: `GATEWAY_PROVIDER_ORDER=gemini,codex,claude` — deterministic tie-breaking at zero load
3. **ALL_CAPS accessor convention**: Signals env-overridability at call sites — unusual in Go but explicit signaling was the user's requirement
4. **Alphabetical default ordering**: When no order is specified, providers sort alphabetically for determinism
5. **File separation**: config.go owns the class (struct + env + accessors), merge.go owns runtime build logic — clean SRP

### Open Debt

None identified. All code is clean and tested.

### Test Results

All 8 packages pass:
```
config       0.018s
database     0.386s
domain       0.006s
gateway      3.504s
pacing       0.007s
provider     0.005s
server       0.295s
```
