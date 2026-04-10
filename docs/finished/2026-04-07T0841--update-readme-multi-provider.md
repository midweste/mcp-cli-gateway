# Update README for Multi-Provider Architecture

> Created: 2026-04-07 08:41 (CDT)
> Status: Debt
> Source: /personas sweep of mcp-gemini-gateway

## Requirement

### Update Requirements Section

- **What**: README says "Requirements: Gemini CLI" — should mention Codex and Claude as optional providers
- **Where**: `README.md`, Requirements section
- **Why**: The gateway now supports 3 providers but the README only mentions Gemini, misleading new users
- **How**: Update to list all 3 CLIs with "install at least one of:" language matching the runtime error message
- **Priority**: Low
- **Effort**: Low

### Update Configuration Table

- **What**: README config table only lists `GATEWAY_DB_PATH` — missing 15+ GATEWAY_* env vars
- **Where**: `README.md`, Configuration section
- **Why**: All env vars are documented in `.env.example` but the README table is stale, creating discoverability gap
- **How**: Expand the config table to list all vars from `.env.example`, or reference `.env.example` as the canonical reference
- **Priority**: Low
- **Effort**: Low
