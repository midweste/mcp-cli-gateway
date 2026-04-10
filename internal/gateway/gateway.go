package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/pacing"
	"github.com/midweste/mcp-cli-gateway/internal/provider"
)

// Executor abstracts subprocess execution for testability.
type Executor interface {
	Run(ctx context.Context, args []string, cwd string, stdin string) (stdout, stderr string, exitCode int, err error)
}

// Store abstracts database operations used by Gateway.
// *database.Store satisfies this interface implicitly.
type Store interface {
	// Request lifecycle
	InsertRequest(ctx context.Context, req *domain.Request) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status string, fields map[string]any) error
	GetRequest(ctx context.Context, id int64) (*domain.Request, error)

	// Counting & listing
	CountRunning(ctx context.Context, model string) (int, error)
	CountPending(ctx context.Context, model string) (int, error)
	StatusCounts(ctx context.Context, model string) (map[string]int, error)
	ListActive(ctx context.Context) ([]domain.Request, error)
	ListActiveByBatchID(ctx context.Context, batchID string) ([]domain.Request, error)
	ListActiveByModel(ctx context.Context, model string) ([]domain.Request, error)
	ListCompleted(ctx context.Context, model string, since time.Time) ([]domain.Request, error)
	StatsAggregate(ctx context.Context, model string, since time.Time) (*domain.StatsAggregate, error)
	ListTimestamps(ctx context.Context, model string, since time.Time) ([]domain.TimestampPair, error)
	ListFailed(ctx context.Context, since time.Time, limit int) ([]domain.Request, error)
	RunningModels(ctx context.Context) ([]string, error)

	// Pacing
	GetPacing(ctx context.Context, model string) (*domain.PacingState, error)
	UpdatePacing(ctx context.Context, model string, fields map[string]any) error
}

// Gateway is the central orchestrator — owns dispatch, batch, and commands.
// Dependencies are injected via constructor (DIP).
type Gateway struct {
	store     Store
	pacer     pacing.Pacer
	executor  Executor
	cfg       *config.Config
	registry  *domain.ModelRegistry
	providers *provider.Registry
	logger    *slog.Logger
}

// NewGateway creates a Gateway with all dependencies injected.
func NewGateway(
	store Store,
	pacer pacing.Pacer,
	executor Executor,
	cfg *config.Config,
	registry *domain.ModelRegistry,
	providers *provider.Registry,
	logger *slog.Logger,
) *Gateway {
	return &Gateway{
		store:     store,
		pacer:     pacer,
		executor:  executor,
		cfg:       cfg,
		registry:  registry,
		providers: providers,
		logger:    logger,
	}
}
