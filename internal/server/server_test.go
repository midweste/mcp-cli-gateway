package server

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/database"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/gateway"
	"github.com/midweste/mcp-cli-gateway/internal/pacing"
	"github.com/midweste/mcp-cli-gateway/internal/provider"
	"github.com/midweste/mcp-cli-gateway/internal/testutil"
)

func newTestGateway(t *testing.T) (*gateway.Gateway, map[string][]string) {
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
	if err := store.SeedPacing(context.Background(), registry, cfg); err != nil {
		t.Fatalf("SeedPacing: %v", err)
	}

	mockProv := &testutil.MockProvider{ProviderName: "gemini", IsAvailable: true}
	providers := provider.NewRegistry(aliasMap, mockProv)
	pacer := pacing.NewManager(store, cfg, registry)
	return gateway.NewGateway(store, pacer, nil, cfg, registry, providers, logger), cfg.Tiers
}

func TestNew(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw, tiers := newTestGateway(t)

	s := New(gw, logger, tiers)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.mcp == nil {
		t.Error("mcp server should be initialized")
	}
	if s.gateway == nil {
		t.Error("gateway should be set")
	}
	if s.logger == nil {
		t.Error("logger should be set")
	}
}

func TestNew_RegistersTools(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	gw, tiers := newTestGateway(t)

	s := New(gw, logger, tiers)

	// Verify tools are registered by listing them
	tools := s.mcp.ListTools()

	expectedTools := []string{
		"gateway_dispatch",
		"gateway_batch_dispatch",
		"gateway_status",
		"gateway_jobs",
		"gateway_pacing",
		"gateway_stats",
		"gateway_errors",
		"gateway_cancel",
		"gateway_retry",
		"gateway_result",
	}

	for _, name := range expectedTools {
		if _, ok := tools[name]; !ok {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}
