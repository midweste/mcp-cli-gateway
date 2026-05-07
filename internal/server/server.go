package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/midweste/mcp-cli-gateway/internal/gateway"
)

// Server wraps the MCP server and gateway.
type Server struct {
	mcp          *server.MCPServer
	gateway      *gateway.Gateway
	logger       *slog.Logger
	aliasToTier  map[string]string // codex-fast → fast, gemini-deep → deep
}

// New creates a new MCP server with all gateway tools registered.
func New(gw *gateway.Gateway, logger *slog.Logger, tiers map[string][]string) *Server {
	// Build reverse map: alias → tier name
	a2t := make(map[string]string)
	for tier, aliases := range tiers {
		for _, alias := range aliases {
			a2t[alias] = tier
		}
	}
	s := &Server{
		mcp: server.NewMCPServer(
			"mcp-cli-gateway",
			"1.0.0",
			server.WithToolCapabilities(true),
			server.WithRoots(),
			server.WithInstructions(`Multi-provider gateway for parallel agent dispatch via CLI tools (Gemini, Codex, Claude).
Auto-selects the best available provider based on load and CLI availability.

PARALLELISM RULES:
- Evaluate EVERY task for parallelism before starting. If work can be split into independent tasks (different files, no output dependency, no shared state), it MUST be split.
- Parallel: edit different files, write impl + tests from spec, research topic A + B, spot-check N items.
- Sequential: edit same file, task B depends on A's output, update interface + consumers.

MODEL TIERS:
- lite: lightweight tasks, quick lookups, simple edits (auto-rebalances across available providers)
- fast: code generation, tests, refactoring, config edits, spot-checks, log analysis (auto-rebalances across available providers)
- deep: architecture review, complex reasoning, multi-file refactors, complex validation (auto-rebalances across available providers)

ORCHESTRATOR ROLE:
1. Evaluate parallelism graph before work starts
2. Dispatch independent tasks via gateway_dispatch or gateway_batch_dispatch
3. Work on your own tasks — never idle-wait
4. Both gateway_dispatch and gateway_batch_dispatch return response_text inline — no extra calls needed
5. Fix minor issues inline; on retry, improve the prompt first (max 2 retries, then do it yourself)
6. Never cancel a running job — wait for completion

PROMPT TIPS:
- Specify exact file paths to read and write
- Be explicit about namespace, conventions, and what NOT to modify
- For convention-sensitive work, tell the agent to read existing examples first rather than describing conventions in the prompt
- Agents write files directly — response summaries are stored in DB and returned with dispatch results

RESPONSE FIELDS:
- gateway_dispatch returns a DispatchResult with "output" containing the agent's response text
- gateway_batch_dispatch returns BatchResult[] with "response_text" per job
- gateway_result(id) retrieves a completed job's full details including response_text
- gateway_errors shows recent failures with error details

HEALTH: Call gateway_status(). ok = dispatch freely, slow = limit to 1, saturated = do it yourself.

REPORTING: Always report to user — on dispatch (count, tasks, tier), on complete (pass/fail, git diff), on skip (why).`),
		),
		gateway:     gw,
		logger:      logger,
		aliasToTier: a2t,
	}

	s.registerTools()
	return s
}

// StartStdio begins serving MCP over stdin/stdout.
func (s *Server) StartStdio(ctx context.Context) error {
	s.logger.Info("starting MCP server (stdio)")
	stdioServer := server.NewStdioServer(s.mcp)
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}

// toJSON marshals a value to indented JSON string.
func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(b)
}
