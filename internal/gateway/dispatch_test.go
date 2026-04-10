package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/testutil"
	"github.com/midweste/mcp-cli-gateway/internal/database"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/pacing"
	"github.com/midweste/mcp-cli-gateway/internal/provider"
)

// mockExecutor implements Executor for testing dispatch flow.
type mockExecutor struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	calls    int
}

func (m *mockExecutor) Run(_ context.Context, args []string, cwd string, stdin string) (string, string, int, error) {
	m.calls++
	return m.stdout, m.stderr, m.exitCode, m.err
}

// newFullTestGateway creates a Gateway with all dependencies for dispatch testing.
func newFullTestGateway(t *testing.T, exec Executor) (*Gateway, *database.Store) {
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

	mockProv := &mockProvider{name: "gemini", available: true}
	providers := provider.NewRegistry(aliasMap, mockProv)
	pacer := pacing.NewManager(store, cfg, registry)
	gw := NewGateway(store, pacer, exec, cfg, registry, providers, logger)
	return gw, store
}

func TestDispatch_Success(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "Hello world", "stats": {}}`,
		exitCode: 0,
	}
	gw, _ := newFullTestGateway(t, exec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "say hello",
		Label:  "test-job",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code=%d, want 0", result.ExitCode)
	}
	if result.RequestID == 0 {
		t.Error("request_id should be non-zero")
	}
	if exec.calls != 1 {
		t.Errorf("executor calls=%d, want 1", exec.calls)
	}
}

func TestDispatch_UnknownModel(t *testing.T) {
	exec := &mockExecutor{}
	gw, _ := newFullTestGateway(t, exec)

	_, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "nonexistent",
		Prompt: "test",
	})
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestDispatch_RateLimit_Retries(t *testing.T) {
	// First call returns rate limit, second succeeds
	customExec := &rateLimitThenSuccessExecutor{
		rateLimitCalls: 1,
	}

	gw, _ := newFullTestGateway(t, customExec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "test retry",
		Label:  "retry-test",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code=%d, want 0 (should recover after retry)", result.ExitCode)
	}
	if customExec.calls < 2 {
		t.Errorf("calls=%d, want >= 2 (at least 1 retry)", customExec.calls)
	}
}

// rateLimitThenSuccessExecutor returns rate-limit N times, then success.
type rateLimitThenSuccessExecutor struct {
	rateLimitCalls int
	calls          int
}

func (e *rateLimitThenSuccessExecutor) Run(_ context.Context, args []string, cwd string, stdin string) (string, string, int, error) {
	e.calls++
	if e.calls <= e.rateLimitCalls {
		return "", "429 Too Many Requests", 1, nil
	}
	return `{"response": "ok", "stats": {}}`, "", 0, nil
}

func TestDispatch_Failure(t *testing.T) {
	exec := &mockExecutor{
		stdout:   "",
		stderr:   "command failed",
		exitCode: 1,
	}
	gw, _ := newFullTestGateway(t, exec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "fail test",
		Label:  "fail-job",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for failure")
	}
}

func TestDispatch_ExecutorError(t *testing.T) {
	exec := &mockExecutor{
		err: fmt.Errorf("connection refused"),
	}
	gw, _ := newFullTestGateway(t, exec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "error test",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit for executor error")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestDispatch_Sandbox(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "sandbox result", "stats": {}}`,
		exitCode: 0,
	}
	gw, _ := newFullTestGateway(t, exec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:   "fast",
		Prompt:  "sandbox test",
		Sandbox: true,
		Cwd:     "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code=%d, want 0", result.ExitCode)
	}
}

func TestDispatch_QueueFull(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "ok", "stats": {}}`,
		exitCode: 0,
	}
	gw, store := newFullTestGateway(t, exec)

	// Fill the queue to capacity
	ctx := context.Background()
	model, _ := gw.registry.Resolve("gemini-fast")
	maxQueue := gw.cfg.MaxQueue["gemini-fast"]
	for range maxQueue {
		req := &domain.Request{
			Model: model, Status: "waiting", PromptHash: "hash",
			PID: 0, Cwd: "/tmp", CreatedAt: float64(time.Now().Unix()),
		}
		store.InsertRequest(ctx, req)
	}

	result, err := gw.Dispatch(ctx, DispatchRequest{
		Model:  "fast",
		Prompt: "queue full test",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 2 {
		t.Errorf("exit_code=%d, want 2 (queue full)", result.ExitCode)
	}
	if result.Error == "" {
		t.Error("expected queue full error message")
	}
}

// ════════ parseGeminiOutput tests ════════

func TestParseGeminiOutput(t *testing.T) {
	t.Parallel()

	geminiProv := provider.NewGeminiProvider("")

	tests := []struct {
		name      string
		input     string
		wantResp  string
		wantStats bool
	}{
		{
			name:     "ValidJSON",
			input:    `{"response": "Hello!", "stats": {"models": {"gemini-3-flash": {"tokens": {"input": 100, "candidates": 200}, "api": {"totalLatencyMs": 500}}}, "tools": {"totalCalls": 3}}}`,
			wantResp: "Hello!",
			wantStats: true,
		},
		{
			name:     "InvalidJSON",
			input:    "This is not JSON output",
			wantResp: "This is not JSON output",
			wantStats: false,
		},
		{
			name:     "EmptyJSON",
			input:    `{"response": "", "stats": {}}`,
			wantResp: "",
			wantStats: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, stats := geminiProv.ParseOutput(tt.input)
			if resp != tt.wantResp {
				t.Errorf("response=%q, want %q", resp, tt.wantResp)
			}
			if tt.wantStats && len(stats) == 0 {
				t.Error("expected non-empty stats")
			}
			if tt.wantStats {
				if _, ok := stats["tokens_in"]; !ok {
					t.Error("missing tokens_in in stats")
				}
				if _, ok := stats["tool_calls"]; !ok {
					t.Error("missing tool_calls in stats")
				}
			}
		})
	}
}

func TestFindBucketAlternative(t *testing.T) {
	exec := &mockExecutor{}
	gw, _ := newFullTestGateway(t, exec)

	// No running models → returns a bucket peer since nothing is marked unavailable
	// Use gemini-deep (deep bucket has think+deep, so there's a peer)
	alt := gw.findBucketAlternative(context.Background(), "gemini-deep")
	if alt == "" {
		t.Error("findBucketAlternative('gemini-deep') returned empty, want a bucket peer")
	}
	if alt == "gemini-deep" {
		t.Error("findBucketAlternative should not return the same alias")
	}
}

// mockProvider implements provider.Provider for gateway tests.
type mockProvider struct {
	name      string
	available bool
}

func (m *mockProvider) Name() string   { return m.name }
func (m *mockProvider) Available() bool { return m.available }
func (m *mockProvider) BuildCommand(model string, cwd string, sandbox bool) []string {
	return []string{"echo", "mock"}
}
func (m *mockProvider) ParseOutput(stdout string) (string, map[string]any) {
	// Delegate to Gemini parser for test compatibility
	return provider.NewGeminiProvider("").ParseOutput(stdout)
}
func (m *mockProvider) DetectRateLimit(exitCode int, stdout, stderr string) bool {
	return provider.NewGeminiProvider("").DetectRateLimit(exitCode, stdout, stderr)
}
func (m *mockProvider) SystemPrefix() string { return "test: " }
func (m *mockProvider) TierModels() map[string][]string {
	return map[string][]string{
		"lite": {"test-lite"},
		"fast": {"test-fast"},
		"deep": {"test-deep"},
	}
}
func (m *mockProvider) ModelPacing(model string) config.PacingConfig {
	return config.PacingConfig{
		InitialGapMs:  1000,
		FloorMs:       500,
		MaxConcurrent: 1,
		MaxQueue:      50,
	}
}

// ── Dispatch with tier name (public alias "fast" → "gemini-fast") ──

func TestDispatch_TierName(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "tier resolved", "stats": {}}`,
		exitCode: 0,
	}
	gw, _ := newFullTestGateway(t, exec)

	// "fast" is a public tier name — should resolve to "gemini-fast"
	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "tier test",
		Label:  "tier-job",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code=%d, want 0", result.ExitCode)
	}
}

// ── Dispatch with context cancellation ──

func TestDispatch_ContextCancelled(t *testing.T) {
	// Executor that blocks forever
	blockingExec := &blockingExecutor{done: make(chan struct{})}
	gw, _ := newFullTestGateway(t, blockingExec)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := gw.Dispatch(ctx, DispatchRequest{
		Model:  "fast",
		Prompt: "cancel test",
		Cwd:    "/tmp",
	})
	// Should either error or return a result — either is acceptable
	if err != nil {
		// Context cancelled during pacing wait or execution
		if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Logf("unexpected error type: %v", err)
		}
	}
	close(blockingExec.done)
}

type blockingExecutor struct {
	done chan struct{}
}

func (b *blockingExecutor) Run(ctx context.Context, args []string, cwd string, stdin string) (string, string, int, error) {
	select {
	case <-ctx.Done():
		return "", "", -1, ctx.Err()
	case <-b.done:
		return "", "", 0, nil
	}
}

// ── Dispatch with empty cwd (should use PROJECT_ROOT) ──

func TestDispatch_EmptyCwd(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "ok", "stats": {}}`,
		exitCode: 0,
	}
	gw, _ := newFullTestGateway(t, exec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "no cwd test",
		// Cwd intentionally empty
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code=%d, want 0", result.ExitCode)
	}
}

// ── Dispatch exhausts retries ──

func TestDispatch_ExhaustsRetries(t *testing.T) {
	// Always returns rate limit 
	exec := &rateLimitThenSuccessExecutor{
		rateLimitCalls: 999, // never succeeds
	}
	gw, _ := newFullTestGateway(t, exec)

	result, err := gw.Dispatch(context.Background(), DispatchRequest{
		Model:  "fast",
		Prompt: "will exhaust retries",
		Cwd:    "/tmp",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit_code=%d, want 1 (exhausted retries)", result.ExitCode)
	}
	if result.Error == "" {
		t.Error("expected error message about exhausted retries")
	}
}

// ── pickBucketAlternative edge cases ──

func TestPickBucketAlternative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bucket      []string
		requested   string
		unavailable map[string]bool
		want        string
	}{
		{
			name:        "PreferSmarter",
			bucket:      []string{"lite", "fast", "deep"},
			requested:   "fast",
			unavailable: map[string]bool{},
			want:        "deep", // higher index preferred
		},
		{
			name:        "FallbackToLesser",
			bucket:      []string{"lite", "fast", "deep"},
			requested:   "fast",
			unavailable: map[string]bool{"deep": true},
			want:        "lite",
		},
		{
			name:        "AllUnavailable",
			bucket:      []string{"lite", "fast", "deep"},
			requested:   "fast",
			unavailable: map[string]bool{"lite": true, "deep": true},
			want:        "",
		},
		{
			name:        "RequestedNotInBucket",
			bucket:      []string{"lite", "fast", "deep"},
			requested:   "unknown",
			unavailable: map[string]bool{},
			want:        "",
		},
		{
			name:        "SingleItemBucket",
			bucket:      []string{"fast"},
			requested:   "fast",
			unavailable: map[string]bool{},
			want:        "", // no alternatives
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pickBucketAlternative(tt.bucket, tt.requested, tt.unavailable)
			if got != tt.want {
				t.Errorf("pickBucketAlternative(%v, %q, %v) = %q, want %q",
					tt.bucket, tt.requested, tt.unavailable, got, tt.want)
			}
		})
	}
}

// ── ParseDuration edge cases ──

func TestParseDuration_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  time.Duration
	}{
		{"", 0},
		{"1h", time.Hour},
		{"2d", 48 * time.Hour},
		{"30m", 30 * time.Minute},
		{"0.5h", 30 * time.Minute},
		{"3", 3 * time.Hour},       // bare number → hours
		{"abc", 0},                  // invalid
		{"x", 0},                    // single char non-numeric
		{"1x", time.Hour},          // unknown suffix → try full as hours
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Input_%s", tt.input), func(t *testing.T) {
			t.Parallel()
			got := ParseDuration(tt.input)
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ── Jobs with populated data ──

func TestJobs_WithWaitingAndRetrying(t *testing.T) {
	gw, store := newFullTestGateway(t, &mockExecutor{})
	model, _ := gw.registry.Resolve("gemini-fast")
	ctx := context.Background()

	insertTestReq(t, store, model, "running")
	insertTestReq(t, store, model, "waiting")
	insertTestReq(t, store, model, "retrying")

	jobs, err := gw.Jobs(ctx)
	if err != nil {
		t.Fatalf("Jobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("len(jobs)=%d, want 3", len(jobs))
	}

	// Verify all active statuses are represented
	statuses := make(map[string]int)
	for _, j := range jobs {
		statuses[j.Status]++
	}
	if statuses["running"] != 1 {
		t.Errorf("running jobs=%d, want 1", statuses["running"])
	}
}

// ── findBucketAlternative with all aliases busy ──

func TestFindBucketAlternative_AllBusy(t *testing.T) {
	exec := &mockExecutor{}
	gw, store := newFullTestGateway(t, exec)
	ctx := context.Background()

	// Fill all deep-tier models with running requests
	bucket := FindBucketForModel(gw.cfg, "gemini-deep")
	for _, alias := range bucket {
		model, _ := gw.registry.Resolve(alias)
		insertTestReq(t, store, model, "running")
	}

	// Now all bucket models are busy — findBucketAlternative should return ""
	alt := gw.findBucketAlternative(ctx, "gemini-deep")
	if alt != "" {
		t.Errorf("findBucketAlternative returned %q, want empty (all busy)", alt)
	}
}
