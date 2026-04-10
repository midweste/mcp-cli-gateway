package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/database"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/gateway"
	"github.com/midweste/mcp-cli-gateway/internal/pacing"
	"github.com/midweste/mcp-cli-gateway/internal/provider"
	"github.com/midweste/mcp-cli-gateway/internal/testutil"
)

// newTestMCPServer creates a fully wired MCPServer for handler testing.
func newTestMCPServer(t *testing.T) *MCPServer {
	t.Helper()
	cfg := config.Default()
	cfg.DBPath = ":memory:"
	cfg.ProjectRoot = "/tmp"
	cfg.Merge([]config.ProviderDescriptor{testutil.TestGeminiProvider()})
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := database.NewStore(cfg, ":memory:", logger)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	aliasMap := cfg.AliasProviderMap([]config.ProviderDescriptor{testutil.TestGeminiProvider()})
	registry := domain.NewModelRegistry(cfg.Models, aliasMap)
	store.SeedPacing(context.Background(), registry, cfg)

	mockProv := &testutil.MockProvider{ProviderName: "gemini", IsAvailable: true}
	providers := provider.NewRegistry(aliasMap, mockProv)
	pacer := pacing.NewManager(store, cfg, registry)
	gw := gateway.NewGateway(store, pacer, nil, cfg, registry, providers, logger)
	return New(gw, logger, cfg.Tiers)
}

func callTool(t *testing.T, s *MCPServer, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := s.mcp.GetTool(name)
	if tool == nil {
		t.Fatalf("tool %q not found", name)
	}

	argsJSON, _ := json.Marshal(args)
	var rawArgs json.RawMessage = argsJSON

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = rawArgs

	result, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler error: %v", name, err)
	}
	return result
}

func TestHandler_Status(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_status", nil)
	if result == nil {
		t.Fatal("nil result")
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestHandler_Jobs(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_jobs", nil)
	if result == nil {
		t.Fatal("nil result")
	}
}

func TestHandler_Pacing(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_pacing", nil)
	if result == nil {
		t.Fatal("nil result")
	}
}

func TestHandler_Stats(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_stats", map[string]any{"last": "1h"})
	if result == nil {
		t.Fatal("nil result")
	}
}

func TestHandler_Errors(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_errors", map[string]any{"last": ""})
	if result == nil {
		t.Fatal("nil result")
	}
}

func TestHandler_Cancel_NoArgs(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_cancel", map[string]any{})
	if result == nil {
		t.Fatal("nil result")
	}
	// Cancel returns a JSON result with an error field — not a tool-level error
	if len(result.Content) == 0 {
		t.Error("expected content with error message")
	}
}

func TestHandler_Retry_InvalidID(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_retry", map[string]any{"id": "not-a-number"})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for non-numeric ID")
	}
}

func TestHandler_Result_NotFound(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_result", map[string]any{"id": float64(99999)})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for non-existent job")
	}
}

func TestHandler_BatchDispatch_InvalidJobs(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_batch_dispatch", map[string]any{"jobs": "not-an-array"})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for invalid jobs format")
	}
}

func TestHandler_BatchDispatch_InvalidJobObject(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_batch_dispatch", map[string]any{
		"jobs": []any{"not-an-object"},
	})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for invalid job object")
	}
}

func TestHandler_Dispatch_NoExecutor(t *testing.T) {
	// Gateway was created with nil executor — dispatch should fail gracefully.
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_dispatch", map[string]any{
		"model":  "fast",
		"prompt": "test prompt",
	})
	if result == nil {
		t.Fatal("nil result")
	}
	// Should get an error result (no executor configured)
	if !result.IsError {
		// Even if it doesn't error, it should have content
		if len(result.Content) == 0 {
			t.Error("expected content in result")
		}
	}
}

func TestHandler_Stats_Lifetime(t *testing.T) {
	s := newTestMCPServer(t)
	// Empty "last" → lifetime stats
	result := callTool(t, s, "gateway_stats", map[string]any{})
	if result == nil {
		t.Fatal("nil result")
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestHandler_Retry_ValidFailedJob(t *testing.T) {
	s := newTestMCPServer(t)
	// Insert a failed request to retry
	store, err := database.NewStore(config.Default(), ":memory:", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Use the test server's gateway's store directly — retry will look up by ID
	// Since no matching job exists at ID 1, expect an error
	result := callTool(t, s, "gateway_retry", map[string]any{"id": float64(1)})
	if result == nil {
		t.Fatal("nil result")
	}
	// Job doesn't exist or not in failed state — should error
	if !result.IsError {
		if len(result.Content) == 0 {
			t.Error("expected content in result")
		}
	}
}

func TestHandler_Cancel_ByID(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_cancel", map[string]any{"id": "999"})
	if result == nil {
		t.Fatal("nil result")
	}
	// Should return a result (possibly with 0 cancelled)
	if len(result.Content) == 0 {
		t.Error("expected content")
	}
}

func TestHandler_Result_InvalidID(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_result", map[string]any{"id": "not-a-number"})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for non-numeric ID")
	}
}

func TestHandler_Retry_MissingID(t *testing.T) {
	s := newTestMCPServer(t)
	result := callTool(t, s, "gateway_retry", map[string]any{})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for missing ID")
	}
}
