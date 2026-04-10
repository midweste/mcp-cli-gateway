package gateway

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/testutil"
)

func TestFindBucket(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Merge([]config.ProviderDescriptor{testutil.TestGeminiProvider()})

	tests := []struct {
		name    string
		alias   string
		wantNil bool
		wantLen int
	}{
		{name: "FlashBucket", alias: "gemini-fast", wantNil: false, wantLen: 1},
		{name: "DeepBucket", alias: "gemini-deep", wantNil: false, wantLen: 2},
		{name: "LiteBucket", alias: "gemini-lite", wantNil: false, wantLen: 1},
		{name: "Unknown", alias: "nonexistent", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bucket := FindBucketForModel(cfg, tt.alias)
			if tt.wantNil && bucket != nil {
				t.Errorf("expected nil bucket, got %v", bucket)
			}
			if !tt.wantNil && bucket == nil {
				t.Error("expected non-nil bucket, got nil")
			}
			if !tt.wantNil && len(bucket) != tt.wantLen {
				t.Errorf("bucket len=%d, want %d", len(bucket), tt.wantLen)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		slice []string
		item  string
		want  int
	}{
		{"Found", []string{"a", "b", "c"}, "b", 1},
		{"First", []string{"a", "b", "c"}, "a", 0},
		{"Last", []string{"a", "b", "c"}, "c", 2},
		{"NotFound", []string{"a", "b", "c"}, "d", -1},
		{"Empty", []string{}, "a", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := indexOf(tt.slice, tt.item)
			if got != tt.want {
				t.Errorf("indexOf(%v, %q) = %d, want %d", tt.slice, tt.item, got, tt.want)
			}
		})
	}
}

// newBatchGateway delegates to newFullTestGateway (dispatch_test.go) — same package.
var newBatchGateway = newFullTestGateway

func TestAssignModelsForBatch(t *testing.T) {
	exec := &mockExecutor{}
	gw, _ := newBatchGateway(t, exec)

	tests := []struct {
		name string
		jobs []DispatchRequest
		check func(t *testing.T, assignments []Assignment)
	}{
		{
			name: "SingleJob",
			jobs: []DispatchRequest{{Model: "fast", Prompt: "test"}},
			check: func(t *testing.T, assignments []Assignment) {
				if len(assignments) != 1 {
					t.Errorf("len=%d, want 1", len(assignments))
				}
				// "fast" tier resolves to "gemini-fast"
				if assignments[0].Alias != "gemini-fast" {
					t.Errorf("alias=%q, want 'gemini-fast'", assignments[0].Alias)
				}
			},
		},
		{
			name: "TwoSameBucket_Spreads",
			jobs: []DispatchRequest{
				{Model: "deep", Prompt: "a"},
				{Model: "deep", Prompt: "b"},
			},
			check: func(t *testing.T, assignments []Assignment) {
				if len(assignments) != 2 {
					t.Fatalf("len=%d, want 2", len(assignments))
				}
				// Deep bucket has 2 models (think+deep), second job should get a different alias
				if assignments[0].Alias == assignments[1].Alias {
					t.Errorf("expected different aliases for spreading, both got %q", assignments[0].Alias)
				}
			},
		},
		{
			name: "EmptyModelDefaults",
			jobs: []DispatchRequest{{Prompt: "default model"}},
			check: func(t *testing.T, assignments []Assignment) {
				if len(assignments) != 1 {
					t.Errorf("len=%d, want 1", len(assignments))
				}
				// Default "fast" tier resolves to "gemini-fast"
				if assignments[0].Alias != "gemini-fast" {
					t.Errorf("alias=%q, want 'gemini-fast' (default)", assignments[0].Alias)
				}
			},
		},
		{
			name: "CrossBucket",
			jobs: []DispatchRequest{
				{Model: "fast", Prompt: "flash tier"},
				{Model: "deep", Prompt: "pro tier"},
			},
			check: func(t *testing.T, assignments []Assignment) {
				if len(assignments) != 2 {
					t.Fatalf("len=%d, want 2", len(assignments))
				}
				// Should resolve tiers to provider-prefixed aliases.
				// Deep tier has multiple models (gemini-deep, gemini-deep-1) and
				// resolveTier picks randomly among equal-load candidates, so accept any.
				aliases := map[string]bool{assignments[0].Alias: true, assignments[1].Alias: true}
				hasFast := aliases["gemini-fast"]
				hasDeep := aliases["gemini-deep"] || aliases["gemini-deep-1"]
				if !hasFast || !hasDeep {
					t.Errorf("expected one fast-tier + one deep-tier alias, got %v", aliases)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignments := gw.AssignModelsForBatch(context.Background(), tt.jobs)
			tt.check(t, assignments)
		})
	}
}

func TestRunBatch(t *testing.T) {
	exec := &mockExecutor{
		stdout:   `{"response": "batch ok", "stats": {}}`,
		exitCode: 0,
	}
	gw, _ := newBatchGateway(t, exec)

	jobs := []DispatchRequest{
		{Model: "fast", Prompt: "batch job 1", Label: "job1", Cwd: "/tmp"},
		{Model: "deep", Prompt: "batch job 2", Label: "job2", Cwd: "/tmp"},
	}

	results, err := gw.RunBatch(context.Background(), jobs)
	if err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len=%d, want 2", len(results))
	}

	for i, r := range results {
		if r.ExitCode != 0 {
			t.Errorf("results[%d].ExitCode=%d, want 0", i, r.ExitCode)
		}
		if r.Status != "ok" {
			t.Errorf("results[%d].Status=%q, want 'ok'", i, r.Status)
		}
	}
}

func TestRunBatch_MixedResults(t *testing.T) {
	// Alternating success/failure
	exec := &alternatingExecutor{}
	gw, _ := newBatchGateway(t, exec)

	jobs := []DispatchRequest{
		{Model: "fast", Prompt: "success job", Label: "good", Cwd: "/tmp"},
		{Model: "deep", Prompt: "fail job", Label: "bad", Cwd: "/tmp"},
	}

	results, err := gw.RunBatch(context.Background(), jobs)
	if err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len=%d, want 2", len(results))
	}
}

// alternatingExecutor returns success on odd calls, failure on even.
// Uses atomic counter because RunBatch dispatches goroutines per model.
type alternatingExecutor struct {
	calls atomic.Int32
}

func (e *alternatingExecutor) Run(_ context.Context, args []string, cwd string, stdin string) (string, string, int, error) {
	n := e.calls.Add(1)
	if n%2 == 1 {
		return `{"response": "ok", "stats": {}}`, "", 0, nil
	}
	return "", "error", 1, nil
}
