package gateway

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/testutil"
	"github.com/midweste/mcp-cli-gateway/internal/database"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/pacing"
	"github.com/midweste/mcp-cli-gateway/internal/provider"
)

func TestPollForSlot_ContextCancelled(t *testing.T) {
	exec := &mockExecutor{}
	gw, _ := newFullTestGateway(t, exec)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := gw.pollForSlot(ctx, "gemini-2.5-flash", 1)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestPollForSlot_SlotAvailable(t *testing.T) {
	exec := &mockExecutor{}
	cfg := config.Default()
	cfg.DBPath = ":memory:"
	cfg.ProjectRoot = "/tmp"
	cfg.QueuePollInterval = 10 * time.Millisecond // fast poll for tests
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

	mockProv := &mockProvider{name: "gemini", available: true}
	providers := provider.NewRegistry(aliasMap, mockProv)
	pacer := pacing.NewManager(store, cfg, registry)
	gw := NewGateway(store, pacer, exec, cfg, registry, providers, logger)

	// No running jobs — slot is immediately available
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = gw.pollForSlot(ctx, "gemini-2.5-flash", 1)
	if err != nil {
		t.Errorf("pollForSlot: %v — expected nil when slot available", err)
	}
}

func TestPollForSlot_WaitsForSlot(t *testing.T) {
	exec := &mockExecutor{}
	cfg := config.Default()
	cfg.DBPath = ":memory:"
	cfg.ProjectRoot = "/tmp"
	cfg.QueuePollInterval = 50 * time.Millisecond
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

	mockProv := &mockProvider{name: "gemini", available: true}
	providers := provider.NewRegistry(aliasMap, mockProv)
	pacer := pacing.NewManager(store, cfg, registry)
	gw := NewGateway(store, pacer, exec, cfg, registry, providers, logger)

	model, _ := registry.Resolve("gemini-fast")

	// Fill the slot
	ctx := context.Background()
	req := &domain.Request{
		Model: model, Status: domain.StatusRunning, PromptHash: "hash",
		PID: 0, Cwd: "/tmp", CreatedAt: float64(time.Now().Unix()),
	}
	id, _ := store.InsertRequest(ctx, req)

	// Start polling in background — will block since slot is full
	pollCtx, pollCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer pollCancel()

	done := make(chan error, 1)
	go func() {
		done <- gw.pollForSlot(pollCtx, model, 1)
	}()

	// Free the slot after a brief delay
	time.Sleep(100 * time.Millisecond)
	store.UpdateStatus(ctx, id, domain.StatusDone, map[string]any{
		"finished_at": domain.NowUnix(),
	})

	err = <-done
	if err != nil {
		t.Errorf("pollForSlot should have succeeded after slot freed: %v", err)
	}
}
