package pacing

import (
	"context"
	"testing"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/testutil"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
)

type mockPacingStore struct {
	states map[string]*domain.PacingState
}

func newMockPacingStore(model string, gapMs int) *mockPacingStore {
	return &mockPacingStore{
		states: map[string]*domain.PacingState{
			model: {Model: model, MinGapMs: gapMs},
		},
	}
}

func (m *mockPacingStore) GetPacing(_ context.Context, model string) (*domain.PacingState, error) {
	s, ok := m.states[model]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockPacingStore) UpdatePacing(_ context.Context, model string, fields map[string]any) error {
	s, ok := m.states[model]
	if !ok {
		s = &domain.PacingState{Model: model}
		m.states[model] = s
	}
	for k, v := range fields {
		switch k {
		case "min_gap_ms":
			s.MinGapMs = v.(int)
		case "backoff_ms":
			s.BackoffMs = v.(int)
		case "consecutive_ok":
			s.ConsecutiveOK = v.(int)
		case "total_ok":
			s.TotalOK = v.(int)
		case "total_rate_limited":
			s.TotalRateLimited = v.(int)
		}
	}
	return nil
}

func TestOnSuccess(t *testing.T) {
	t.Parallel()
	model := "gemini-2.5-flash"
	cfg := config.Default()
	cfg.Merge([]config.ProviderDescriptor{testutil.TestGeminiProvider()})
	registry := domain.NewModelRegistry(cfg.Models, cfg.AliasProviderMap([]config.ProviderDescriptor{testutil.TestGeminiProvider()}))

	tests := []struct {
		name        string
		initialGap  int
		calls       int
		expectLower bool
	}{
		{name: "SpeedsUp", initialGap: 5000, calls: 1, expectLower: true},
		{name: "StreakBonus", initialGap: 5000, calls: cfg.StreakThreshold + 1, expectLower: true},
		{name: "RespectsFloor", initialGap: 1500, calls: 50, expectLower: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPacingStore(model, tt.initialGap)
			mgr := NewManager(store, cfg, registry)
			ctx := context.Background()

			for range tt.calls {
				if err := mgr.OnSuccess(ctx, model); err != nil {
					t.Fatalf("OnSuccess: %v", err)
				}
			}

			state := store.states[model]
			floor := cfg.FloorForAlias("gemini-fast")

			if tt.name == "RespectsFloor" {
				if state.MinGapMs < floor {
					t.Errorf("gap=%d below floor=%d", state.MinGapMs, floor)
				}
			} else if tt.expectLower && state.MinGapMs >= tt.initialGap {
				t.Errorf("gap=%d, expected < initial %d", state.MinGapMs, tt.initialGap)
			}
		})
	}
}

func TestOnSuccess_BackoffDrains(t *testing.T) {
	t.Parallel()

	model := "gemini-2.5-flash"
	cfg := config.Default()
	cfg.Merge([]config.ProviderDescriptor{testutil.TestGeminiProvider()})
	registry := domain.NewModelRegistry(cfg.Models, cfg.AliasProviderMap([]config.ProviderDescriptor{testutil.TestGeminiProvider()}))

	store := newMockPacingStore(model, 5000)
	store.states[model].BackoffMs = 3000
	mgr := NewManager(store, cfg, registry)

	ctx := context.Background()
	if err := mgr.OnSuccess(ctx, model); err != nil {
		t.Fatalf("OnSuccess: %v", err)
	}

	if store.states[model].BackoffMs != 2500 {
		t.Errorf("backoff=%d, want 2500 (3000-500)", store.states[model].BackoffMs)
	}
}

func TestOnRateLimit(t *testing.T) {
	t.Parallel()
	model := "gemini-2.5-flash"
	cfg := config.Default()
	cfg.Merge([]config.ProviderDescriptor{testutil.TestGeminiProvider()})
	registry := domain.NewModelRegistry(cfg.Models, cfg.AliasProviderMap([]config.ProviderDescriptor{testutil.TestGeminiProvider()}))

	tests := []struct {
		name         string
		initialGap   int
		calls        int
		expectHigher bool
	}{
		{name: "SlowsDown", initialGap: 2000, calls: 1, expectHigher: true},
		{name: "RespectsCeiling", initialGap: 9000, calls: 10, expectHigher: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPacingStore(model, tt.initialGap)
			mgr := NewManager(store, cfg, registry)
			ctx := context.Background()

			for range tt.calls {
				if err := mgr.OnRateLimit(ctx, model); err != nil {
					t.Fatalf("OnRateLimit: %v", err)
				}
			}

			state := store.states[model]

			if tt.name == "RespectsCeiling" {
				if state.MinGapMs > cfg.CeilingMs {
					t.Errorf("gap=%d above ceiling=%d", state.MinGapMs, cfg.CeilingMs)
				}
			} else if tt.expectHigher && state.MinGapMs <= tt.initialGap {
				t.Errorf("gap=%d, expected > initial %d", state.MinGapMs, tt.initialGap)
			}

			if state.ConsecutiveOK != 0 {
				t.Errorf("consecutive_ok=%d, want 0 after rate limit", state.ConsecutiveOK)
			}
		})
	}
}
