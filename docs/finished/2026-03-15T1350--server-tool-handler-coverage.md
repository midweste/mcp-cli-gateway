# Server Tool Handler Test Coverage

> Created: 2026-03-15 13:50 (local)
> Status: Debt

## Requirement

### Add integration tests for MCP tool handlers

- **What**: The `server` package has 6.6% test coverage. Only `argStr`, `boolPtr`, and `toJSON` utility helpers are tested. The 9 tool handler closures in `tools.go` (dispatch, batch_dispatch, status, jobs, pacing, stats, errors, cancel, retry, result) have zero test coverage.
- **Where**: [tools.go](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/server/tools.go), [tools_test.go](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/server/tools_test.go)
- **Why**: These are the MCP-facing API surface — the boundary where external callers interact with the gateway. Untested boundary code increases the risk of regressions when modifying tool schemas, argument parsing, or response formatting. The server is the only critical path without meaningful test coverage.
- **How**: Two approaches, pick one:
  1. **Refactor handlers into testable functions**: Move each tool handler closure body into a named function (e.g., `handleDispatch(ctx, args) (*mcp.CallToolResult, error)`) that can be tested independently with a mock `Gateway` interface.
  2. **End-to-end via mcp-go test helpers**: If `mcp-go/server` provides test utilities (test client, in-process call), use them to invoke tools and assert responses.
  Start with happy-path tests for read-only tools (`status`, `jobs`, `pacing`) which are safest and highest value.
- **Priority**: Medium
- **Effort**: High — requires either refactoring tool registration or understanding mcp-go test infrastructure
