package gateway

import (
	"context"
	"fmt"
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

func newTestGateway(t *testing.T) (*Gateway, *database.Store) {
	t.Helper()
	return newFullTestGateway(t, nil)
}

func insertTestReq(t *testing.T, store *database.Store, model, status string) int64 {
	t.Helper()
	ctx := context.Background()
	req := &domain.Request{
		Model: model, Status: status, PromptHash: "hash",
		PID: 0, Cwd: "/tmp", CreatedAt: float64(time.Now().Unix()),
	}
	id, err := store.InsertRequest(ctx, req)
	if err != nil {
		t.Fatalf("InsertRequest: %v", err)
	}
	return id
}

func insertTestReqWithOpts(t *testing.T, store *database.Store, model, status, label, batchID string, finishedAt *float64) int64 {
	t.Helper()
	ctx := context.Background()
	req := &domain.Request{
		Model: model, Status: status, PromptHash: "hash", Label: label,
		PID: 0, Cwd: "/tmp", CreatedAt: float64(time.Now().Unix()),
		BatchID: batchID,
	}
	id, err := store.InsertRequest(ctx, req)
	if err != nil {
		t.Fatalf("InsertRequest: %v", err)
	}
	if finishedAt != nil {
		store.UpdateStatus(ctx, id, status, map[string]any{"finished_at": *finishedAt})
	}
	return id
}

func TestStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(t *testing.T, gw *Gateway, store *database.Store)
		check  func(t *testing.T, status map[string]domain.ModelStatus)
	}{
		{
			name:  "Empty",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) {},
			check: func(t *testing.T, status map[string]domain.ModelStatus) {
				for alias, ms := range status {
					if ms.Running != 0 {
						t.Errorf("%s: running=%d, want 0", alias, ms.Running)
					}
					if ms.Health != "ok" {
						t.Errorf("%s: health=%q, want 'ok'", alias, ms.Health)
					}
				}
			},
		},
		{
			name: "Running",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) {
				model, _ := gw.registry.Resolve("gemini-fast")
				insertTestReq(t, store, model, "running")
			},
			check: func(t *testing.T, status map[string]domain.ModelStatus) {
				fast := status["gemini-fast"]
				if fast.Running != 1 {
					t.Errorf("fast.Running=%d, want 1", fast.Running)
				}
				if fast.Health != "busy" {
					t.Errorf("fast.Health=%q, want 'busy'", fast.Health)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw, store := newTestGateway(t)
			tt.setup(t, gw, store)

			status, err := gw.Status(context.Background())
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			tt.check(t, status)
		})
	}
}

func TestJobs(t *testing.T) {
	gw, store := newTestGateway(t)
	model, _ := gw.registry.Resolve("gemini-fast")

	// Insert active and done
	insertTestReq(t, store, model, "running")
	now := float64(time.Now().Unix())
	insertTestReqWithOpts(t, store, model, "done", "done-job", "", &now)

	jobs, err := gw.Jobs(context.Background())
	if err != nil {
		t.Fatalf("Jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(jobs)=%d, want 1 (only active)", len(jobs))
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T, gw *Gateway, store *database.Store) string
		cancel  func(gw *Gateway, param string) (*domain.CancelResult, error)
		wantCnt int
	}{
		{
			name: "ByID",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) string {
				model, _ := gw.registry.Resolve("gemini-fast")
				id := insertTestReq(t, store, model, "waiting")
				return fmt.Sprintf("%d", id)
			},
			cancel:  func(gw *Gateway, param string) (*domain.CancelResult, error) {
				return gw.Cancel(context.Background(), param, "", "")
			},
			wantCnt: 1,
		},
		{
			name: "ByBatchID",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) string {
				model, _ := gw.registry.Resolve("gemini-fast")
				insertTestReqWithOpts(t, store, model, "waiting", "a", "batch-99", nil)
				insertTestReqWithOpts(t, store, model, "running", "b", "batch-99", nil)
				return "batch-99"
			},
			cancel:  func(gw *Gateway, param string) (*domain.CancelResult, error) {
				return gw.Cancel(context.Background(), "", "", param)
			},
			wantCnt: 2,
		},
		{
			name: "ByModel",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) string {
				model, _ := gw.registry.Resolve("gemini-deep")
				insertTestReq(t, store, model, "waiting")
				insertTestReq(t, store, model, "retrying")
				return "gemini-deep"
			},
			cancel:  func(gw *Gateway, param string) (*domain.CancelResult, error) {
				return gw.Cancel(context.Background(), "", param, "")
			},
			wantCnt: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw, store := newTestGateway(t)
			param := tt.setup(t, gw, store)

			result, err := tt.cancel(gw, param)
			if err != nil {
				t.Fatalf("Cancel: %v", err)
			}
			if result.Count != tt.wantCnt {
				t.Errorf("cancelled=%d, want %d", result.Count, tt.wantCnt)
			}
		})
	}
}

func TestStats(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, gw *Gateway, store *database.Store)
		last  string
		check func(t *testing.T, result *domain.StatsResult)
	}{
		{
			name:  "Empty",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) {},
			last:  "",
			check: func(t *testing.T, result *domain.StatsResult) {
				if result.Period != "lifetime" {
					t.Errorf("period=%v, want 'lifetime'", result.Period)
				}
			},
		},
		{
			name: "WithData",
			setup: func(t *testing.T, gw *Gateway, store *database.Store) {
				model, _ := gw.registry.Resolve("gemini-fast")
				ctx := context.Background()
				now := float64(time.Now().Unix())

				// Insert 3 done jobs with timing data
				for i := range 3 {
					id := insertTestReq(t, store, model, "done")
					startedAt := now - float64(10+i)
					store.UpdateStatus(ctx, id, "done", map[string]any{
						"started_at":  startedAt,
						"finished_at": now - float64(i),
						"exit_code":   0,
					})
				}
				// Insert 1 failed job
				failID := insertTestReq(t, store, model, "failed")
				store.UpdateStatus(ctx, failID, "failed", map[string]any{
					"started_at":  now - 5.0,
					"finished_at": now,
					"exit_code":   1,
					"error":       "test fail",
				})
			},
			last: "1h",
			check: func(t *testing.T, result *domain.StatsResult) {
				ms, ok := result.Models["gemini-fast"]
				if !ok {
					t.Fatal("missing 'fast' in stats")
				}
				if ms.TotalJobs != 4 {
					t.Errorf("total_jobs=%d, want 4", ms.TotalJobs)
				}
				if ms.Succeeded != 3 {
					t.Errorf("succeeded=%d, want 3", ms.Succeeded)
				}
				if ms.Failed != 1 {
					t.Errorf("failed=%d, want 1", ms.Failed)
				}
				if ms.AvgExecutionS == nil {
					t.Error("avg_execution_s should not be nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw, store := newTestGateway(t)
			tt.setup(t, gw, store)

			result, err := gw.Stats(context.Background(), tt.last)
			if err != nil {
				t.Fatalf("Stats: %v", err)
			}
			tt.check(t, result)
		})
	}
}

func TestErrors(t *testing.T) {
	gw, store := newTestGateway(t)
	model, _ := gw.registry.Resolve("gemini-fast")

	ctx := context.Background()
	now := float64(time.Now().Unix())
	id := insertTestReq(t, store, model, "failed")
	store.UpdateStatus(ctx, id, "failed", map[string]any{
		"finished_at": now, "error": "test failure", "exit_code": 1,
		"started_at": now - 5.0,
	})

	result, err := gw.Errors(ctx, "")
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	if result.Count != 1 {
		t.Errorf("error count=%v, want 1", result.Count)
	}
}

func TestErrors_WithTimeWindow(t *testing.T) {
	gw, store := newTestGateway(t)
	model, _ := gw.registry.Resolve("gemini-fast")

	ctx := context.Background()
	now := float64(time.Now().Unix())
	id := insertTestReq(t, store, model, "failed")
	store.UpdateStatus(ctx, id, "failed", map[string]any{
		"finished_at": now, "error": "recent fail", "exit_code": 1,
	})

	result, err := gw.Errors(ctx, "1h")
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	if result.Count != 1 {
		t.Errorf("count=%v, want 1", result.Count)
	}
}

func TestPacing(t *testing.T) {
	gw, _ := newTestGateway(t)

	pacing, err := gw.Pacing(context.Background())
	if err != nil {
		t.Fatalf("Pacing: %v", err)
	}

	if len(pacing) != 4 {
		t.Errorf("len(pacing)=%d, want 4", len(pacing))
	}
}

func TestCancel_NoArgs(t *testing.T) {
	gw, _ := newTestGateway(t)

	result, err := gw.Cancel(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error message when no args given")
	}
}

func TestCancel_InvalidID(t *testing.T) {
	gw, _ := newTestGateway(t)

	result, err := gw.Cancel(context.Background(), "not-a-number", "", "")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for invalid ID")
	}
}

func TestCancel_UnknownModel(t *testing.T) {
	gw, _ := newTestGateway(t)

	result, err := gw.Cancel(context.Background(), "", "nonexistent", "")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for unknown model")
	}
}

func TestRetry_NotFound(t *testing.T) {
	gw, _ := newTestGateway(t)

	_, err := gw.Retry(context.Background(), 99999)
	if err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestRetry_NoPrompt(t *testing.T) {
	gw, store := newTestGateway(t)
	model, _ := gw.registry.Resolve("gemini-fast")

	// Insert failed request without prompt_text
	id := insertTestReq(t, store, model, "failed")

	_, err := gw.Retry(context.Background(), id)
	if err == nil {
		t.Error("expected error for job with no stored prompt")
	}
}

func TestRetry_Success(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "retry ok", "stats": {}}`,
		exitCode: 0,
	}
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

	mockProv := &mockProvider{name: "gemini", available: true}
	providers := provider.NewRegistry(aliasMap, mockProv)
	pacer := pacing.NewManager(store, cfg, registry)
	gw := NewGateway(store, pacer, exec, cfg, registry, providers, logger)

	// Insert failed request with prompt_text
	fastModel, _ := registry.Resolve("gemini-fast")
	ctx := context.Background()
	req := &domain.Request{
		Model: fastModel, Status: "failed", PromptHash: "hash",
		PID: 0, Cwd: "/tmp", CreatedAt: float64(time.Now().Unix()),
		PromptText: "retry this prompt", Label: "orig",
	}
	id, _ := store.InsertRequest(ctx, req)

	result, err := gw.Retry(ctx, id)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code=%d, want 0", result.ExitCode)
	}
}

func TestResult(t *testing.T) {
	t.Parallel()
	gw, store := newTestGateway(t)
	ctx := context.Background()

	// Insert a request
	req := &domain.Request{
		Model: "gemini-2.5-flash", Status: "done", PromptHash: "hash",
		PID: 0, Cwd: "/tmp", CreatedAt: float64(time.Now().Unix()),
		ResponseText: "hello world",
	}
	id, _ := store.InsertRequest(ctx, req)

	// Test retrieving it
	result, err := gw.Result(ctx, id)
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if result == nil {
		t.Fatal("Result returned nil")
	}
	if result.ResponseText != "hello world" {
		t.Errorf("ResponseText=%q, want 'hello world'", result.ResponseText)
	}

	// Test non-existent job returns error
	_, err = gw.Result(ctx, 99999)
	if err == nil {
		t.Error("Expected error for non-existent job")
	}
}

func TestPacingCommand(t *testing.T) {
	t.Parallel()
	gw, _ := newTestGateway(t)
	ctx := context.Background()

	result, err := gw.Pacing(ctx)
	if err != nil {
		t.Fatalf("Pacing: %v", err)
	}
	// Should have entries for all models
	if len(result) == 0 {
		t.Error("Pacing returned empty map")
	}
	// Each entry should have non-negative values
	for alias, info := range result {
		if info.MinGapMs < 0 {
			t.Errorf("Pacing[%s].MinGapMs=%d, want >= 0", alias, info.MinGapMs)
		}
	}
}
