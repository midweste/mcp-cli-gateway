# MCP CLI Gateway

Multi-provider gateway for dispatching prompts to CLI agents (Gemini, Codex, Claude) with adaptive pacing, job queuing, batch dispatch, and full observability — exposed as an MCP server over stdio.

The gateway auto-selects the best available provider based on load and CLI availability. You pick a tier (`lite`, `fast`, `deep`) and the gateway handles provider selection, rate limiting, retries, and job persistence.

---

## Getting Started

### Requirements

- Go 1.25+
- At least one CLI agent installed and on `PATH`:
  - [Gemini CLI](https://github.com/google/gemini-cli)
  - [Codex CLI](https://github.com/openai/codex) (optional)
  - [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) (optional)

### Install

```bash
git clone https://github.com/midweste/mcp-cli-gateway.git
cd mcp-cli-gateway
make setup   # copy .env, tidy deps, create data dir
make build   # build the binary
```

### Register as MCP Server

#### Claude Code

Add to your `mcp.json` config:

```json
{
  "mcpServers": {
    "mcp-cli-gateway": {
      "command": "/absolute/path/to/mcp-cli-gateway/mcp-cli-gateway",
      "args": []
    }
  }
}
```

#### Antigravity (Gemini)

Add to your `mcp_config.json`:

```json
{
  "mcpServers": {
    "mcp-cli-gateway": {
      "command": "/absolute/path/to/mcp-cli-gateway/mcp-cli-gateway",
      "args": []
    }
  }
}
```

> Replace `/absolute/path/to` with the actual directory where the binary expects to run.

---

## Model Tiers

Tiers are provider-agnostic — the gateway auto-selects the best available provider within each tier:

| Tier | Use for | Gemini | Codex | Claude |
| ---- | ------- | ------ | ----- | ------ |
| `lite` | Quick lookups, simple edits | gemini-2.5-flash-lite | gpt-5.1-codex-mini | haiku |
| `fast` | Code gen, tests, refactoring | gemini-2.5-flash | gpt-5.3-codex | sonnet |
| `deep` | Architecture, complex reasoning | gemini-2.5-pro | gpt-5.4 | opus |

---

## MCP Tools

| Tool | Mutates | Purpose |
| ---- | ------- | ------- |
| `gateway_dispatch` | ✓ | Send a prompt to a CLI agent (auto-selects provider) |
| `gateway_batch_dispatch` | ✓ | Dispatch multiple prompts in parallel |
| `gateway_status` | | Queue health per tier (`ok`/`busy`/`slow`/`saturated`) |
| `gateway_jobs` | | List active jobs with timing |
| `gateway_pacing` | | Adaptive rate-limit state |
| `gateway_stats` | | Historical performance stats |
| `gateway_errors` | | Recent failures with details |
| `gateway_cancel` | ✓ | Cancel jobs by ID, model, or batch |
| `gateway_retry` | ✓ | Retry a failed job |
| `gateway_result` | | Get full job details and response text |

---

## CLI Usage

A bash wrapper is included for terminal use without an MCP client:

```bash
# Run a single job
./scripts/dispatch.sh dispatch fast "Write unit tests for auth.go"

# Batch dispatch
./scripts/dispatch.sh batch '[{"model":"fast","prompt":"..."},{"model":"lite","prompt":"..."}]'

# Check health
./scripts/dispatch.sh status

# View active jobs
./scripts/dispatch.sh jobs

# Get performance stats
./scripts/dispatch.sh stats 1h

# Cancel a job
./scripts/dispatch.sh cancel --id 42

# Retry a failed job
./scripts/dispatch.sh retry 42
```

---

## Configuration

Configuration uses env vars (`.env`) or CLI flags. Priority: **flag > env > default**.

| Env Var | Default | Purpose |
| ------- | ------- | ------- |
| `GATEWAY_DB_PATH` | `data/mcp-cli-gateway.sqlite` | SQLite database path |
| `GATEWAY_PROVIDER_ORDER` | alphabetical | Provider priority (e.g., `gemini,codex,claude`) |
| `GATEWAY_TIMEOUT_SECONDS` | `420` | Per-job timeout |
| `GATEWAY_MAX_RETRIES` | `3` | Retry attempts on rate limit |
| `GATEWAY_CLEANUP_DAYS` | `7` | Days to keep old request records |

### Provider Enable/Disable

Providers are auto-detected by CLI availability. Override with env vars:

```bash
GATEWAY_PROVIDER_GEMINI=true    # force enable
GATEWAY_PROVIDER_CODEX=false    # force disable
GATEWAY_PROVIDER_CLAUDE=true    # force enable
```

> See [`.env.example`](.env.example) for the full list of tuning variables.

---

## Data Storage

All data lives next to the binary — never in project directories:

```
mcp-cli-gateway/
├── data/
│   └── mcp-cli-gateway.sqlite   # Job queue, pacing state, request history
├── .env                          # Configuration overrides
└── mcp-cli-gateway              # Binary
```

---

# Developer Guide

## Build & Test

| Command | Purpose |
| ------- | ------- |
| `make setup` | First-time setup (copy `.env`, tidy deps) |
| `make build` | Build the binary |
| `make run` | Build + run |
| `make test` | Run all tests |
| `make coverage` | Tests with per-function coverage report |
| `make coverage-html` | Browsable HTML coverage |
| `make vet` | Run `go vet` |
| `make check` | Vet + test in one command |
| `make clean` | Remove build artifacts |
| `make install` | Install binary to `GOPATH/bin` |

## Architecture

```
cmd/mcp-cli-gateway/main.go    Entry point, DI wiring
internal/
├── config/     Config struct, env-aware accessors, path resolution
├── domain/     Shared types, ModelRegistry
├── database/   SQLite store (4 segregated interfaces)
├── pacing/     Adaptive rate-limit manager (per-model gap tracking)
├── provider/   Provider implementations (Gemini, Codex, Claude)
├── gateway/    Orchestrator: dispatch, batch, polling, commands
├── server/     MCP server, tool registrations, stdio transport
└── testutil/   Shared test helpers
scripts/
└── dispatch.sh CLI wrapper for terminal-based MCP calls
```

### Request Flow

```
MCP Client → stdio → MCPServer.registerTools() → gateway.Dispatch()
  → pacing.Manager (rate-limit check)
  → provider.Registry (select provider for tier)
  → provider.Run() (exec CLI subprocess)
  → database.Store (persist result)
  → response → stdio → MCP Client
```

### Key Design Decisions

- **Provider-agnostic tiers**: Users never reference specific models — just `lite`, `fast`, or `deep`. The gateway resolves to the best available provider.
- **Adaptive pacing**: Each model alias tracks its own gap/backoff state. Successful requests tighten the gap; rate limits widen it. Streak bonuses reward consecutive successes.
- **SQLite persistence**: Jobs, pacing state, and request history survive restarts. Stale PIDs are cleaned on startup.
- **Substitution buckets**: If a provider is overloaded, the gateway can rebalance within the same tier to another provider.

## Pacing Tuning

Advanced pacing variables for rate-limit management:

| Env Var | Default | Purpose |
| ------- | ------- | ------- |
| `GATEWAY_CEILING_MS` | `10000` | Max gap between requests to a single model |
| `GATEWAY_BACKOFF_INITIAL_MS` | `1500` | Initial backoff after a rate limit |
| `GATEWAY_BACKOFF_MAX_MS` | `8000` | Max backoff (exponential cap) |
| `GATEWAY_SPEEDUP_FACTOR` | `0.90` | Multiply gap by this on success |
| `GATEWAY_SLOWDOWN_FACTOR` | `1.3` | Multiply gap by this on rate limit |
| `GATEWAY_STREAK_THRESHOLD` | `3` | Consecutive successes to trigger bonus |
| `GATEWAY_STREAK_SPEEDUP` | `0.85` | Gap multiplier on streak bonus |
| `GATEWAY_QUEUE_POLL_SECONDS` | `3` | How often to check the queue |
