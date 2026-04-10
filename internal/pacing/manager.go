package pacing

import (
	"context"
	"fmt"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
)

const (
	// backoffDecrementMs is subtracted from backoff on each success.
	backoffDecrementMs = 500
)

// Pacer defines the adaptive rate-limit interface.
type Pacer interface {
	OnSuccess(ctx context.Context, model string) error
	OnRateLimit(ctx context.Context, model string) error
}

// PacingStore is the subset of database operations needed by the pacing manager.
type PacingStore interface {
	GetPacing(ctx context.Context, model string) (*domain.PacingState, error)
	UpdatePacing(ctx context.Context, model string, fields map[string]any) error
}

// Manager implements Pacer with adaptive speedup/slowdown.
type Manager struct {
	store    PacingStore
	cfg      *config.Config
	registry *domain.ModelRegistry
}

// NewManager creates a new pacing manager.
func NewManager(store PacingStore, cfg *config.Config, registry *domain.ModelRegistry) *Manager {
	return &Manager{store: store, cfg: cfg, registry: registry}
}

// OnSuccess speeds up the gap after a successful request.
func (m *Manager) OnSuccess(ctx context.Context, model string) error {
	state, err := m.store.GetPacing(ctx, model)
	if err != nil {
		return fmt.Errorf("get pacing for %s: %w", model, err)
	}
	if state == nil {
		return nil
	}

	alias := m.registry.AliasFor(model)
	floor := m.cfg.FloorForAlias(alias)
	consecutive := state.ConsecutiveOK + 1

	gap := state.MinGapMs
	if consecutive >= m.cfg.STREAK_THRESHOLD() {
		gap = max(floor, int(float64(gap)*m.cfg.STREAK_SPEEDUP()))
	} else {
		gap = max(floor, int(float64(gap)*m.cfg.SPEEDUP_FACTOR()))
	}

	backoff := max(0, state.BackoffMs-backoffDecrementMs)

	return m.store.UpdatePacing(ctx, model, map[string]any{
		"min_gap_ms":     gap,
		"backoff_ms":     backoff,
		"consecutive_ok": consecutive,
		"total_ok":       state.TotalOK + 1,
	})
}

// OnRateLimit slows down the gap after a rate-limit.
func (m *Manager) OnRateLimit(ctx context.Context, model string) error {
	state, err := m.store.GetPacing(ctx, model)
	if err != nil {
		return fmt.Errorf("get pacing for %s: %w", model, err)
	}
	if state == nil {
		return nil
	}

	gap := min(int(float64(state.MinGapMs)*m.cfg.SLOWDOWN_FACTOR()), m.cfg.CEILING_MS())
	backoff := min(
		max(state.BackoffMs*2, m.cfg.BACKOFF_INITIAL_MS()),
		m.cfg.BACKOFF_MAX_MS(),
	)

	return m.store.UpdatePacing(ctx, model, map[string]any{
		"min_gap_ms":         gap,
		"backoff_ms":         backoff,
		"consecutive_ok":     0,
		"total_rate_limited": state.TotalRateLimited + 1,
	})
}
