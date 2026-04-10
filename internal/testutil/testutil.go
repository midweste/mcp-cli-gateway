package testutil

import (
	"log/slog"
	"os"
	"testing"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/database"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/provider"
)

// TestGeminiProvider returns a MockProvider that satisfies both provider.Provider
// and config.ProviderDescriptor with a gemini-like tier/model setup for testing.
func TestGeminiProvider() *MockProvider {
	return &MockProvider{
		ProviderName: "gemini",
		IsAvailable:  true,
		tierModels: map[string][]string{
			"lite": {"gemini-2.5-flash-lite"},
			"fast": {"gemini-2.5-flash"},
			"deep": {"gemini-2.5-pro", "gemini-3.1-pro-preview"},
		},
	}
}

// NewTestConfig returns a Config with defaults for testing, merged with a mock gemini provider.
func NewTestConfig() *config.Config {
	cfg := config.Default()
	cfg.DBPath = ":memory:"
	cfg.Merge([]config.ProviderDescriptor{TestGeminiProvider()})
	return cfg
}

// NewTestRegistry returns a ModelRegistry from test config.
func NewTestRegistry() *domain.ModelRegistry {
	cfg := NewTestConfig()
	return domain.NewModelRegistry(cfg.Models, cfg.AliasProviderMap([]config.ProviderDescriptor{TestGeminiProvider()}))
}

// NewTestProviders returns a ProviderRegistry with a mock gemini provider for testing.
func NewTestProviders() *provider.Registry {
	cfg := NewTestConfig()
	gemini := TestGeminiProvider()
	return provider.NewRegistry(cfg.AliasProviderMap([]config.ProviderDescriptor{gemini}), gemini)
}

// MockProvider implements provider.Provider and config.ProviderDescriptor for testing.
type MockProvider struct {
	ProviderName string
	IsAvailable  bool
	CmdArgs      []string
	OutputText   string
	OutputStats  map[string]any
	RateLimited  bool
	PrefixText   string
	tierModels   map[string][]string
}

func (m *MockProvider) Name() string   { return m.ProviderName }
func (m *MockProvider) Available() bool { return m.IsAvailable }

func (m *MockProvider) BuildCommand(model string, cwd string, sandbox bool) []string {
	if m.CmdArgs != nil {
		return m.CmdArgs
	}
	return []string{"echo", "mock"}
}

func (m *MockProvider) ParseOutput(stdout string) (string, map[string]any) {
	if m.OutputText != "" {
		stats := m.OutputStats
		if stats == nil {
			stats = make(map[string]any)
		}
		return m.OutputText, stats
	}
	return stdout, make(map[string]any)
}

func (m *MockProvider) DetectRateLimit(exitCode int, stdout, stderr string) bool {
	return m.RateLimited
}

func (m *MockProvider) SystemPrefix() string {
	if m.PrefixText != "" {
		return m.PrefixText
	}
	return "test-prefix: "
}

func (m *MockProvider) TierModels() map[string][]string {
	if m.tierModels != nil {
		return m.tierModels
	}
	return map[string][]string{
		"lite": {"test-lite"},
		"fast": {"test-fast"},
		"deep": {"test-deep"},
	}
}

func (m *MockProvider) ModelPacing(model string) config.PacingConfig {
	return config.PacingConfig{
		InitialGapMs:  1000,
		FloorMs:       500,
		MaxConcurrent: 1,
		MaxQueue:      50,
	}
}

// NewTestStore creates an in-memory database Store for testing.
func NewTestStore(t *testing.T) *database.Store {
	t.Helper()
	cfg := NewTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	store, err := database.NewStore(cfg, ":memory:", logger)
	if err != nil {
		t.Fatalf("NewTestStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	// Seed pacing
	registry := NewTestRegistry()
	if err := store.SeedPacing(t.Context(), registry, cfg); err != nil {
		t.Fatalf("SeedPacing: %v", err)
	}
	return store
}

// InsertRequest inserts a test request with common defaults.
func InsertRequest(t *testing.T, store *database.Store, model, status string, opts ...func(*domain.Request)) int64 {
	t.Helper()
	req := &domain.Request{
		Model:      model,
		Status:     status,
		PromptHash: "testhash",
		PID:        os.Getpid(),
		Cwd:        "/tmp",
		CreatedAt:  domain.NowUnix(),
	}
	for _, opt := range opts {
		opt(req)
	}
	id, err := store.InsertRequest(t.Context(), req)
	if err != nil {
		t.Fatalf("InsertRequest: %v", err)
	}
	return id
}

// WithLabel sets the label on a request.
func WithLabel(label string) func(*domain.Request) {
	return func(r *domain.Request) { r.Label = label }
}

// WithBatchID sets the batch ID on a request.
func WithBatchID(batchID string) func(*domain.Request) {
	return func(r *domain.Request) { r.BatchID = batchID }
}

// WithPID sets the PID on a request.
func WithPID(pid int) func(*domain.Request) {
	return func(r *domain.Request) { r.PID = pid }
}

// WithCreatedAt sets the created_at timestamp.
func WithCreatedAt(ts float64) func(*domain.Request) {
	return func(r *domain.Request) { r.CreatedAt = ts }
}

// WithError sets the error field.
func WithError(errMsg string) func(*domain.Request) {
	return func(r *domain.Request) { r.Error = errMsg }
}

// WithPromptText sets the prompt text field.
func WithPromptText(text string) func(*domain.Request) {
	return func(r *domain.Request) { r.PromptText = text }
}
