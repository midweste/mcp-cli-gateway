// Package provider abstracts CLI agent backends (Gemini, Codex, Claude).
// Each provider knows how to build its CLI command, parse its output,
// detect its rate-limit signals, and describe its model catalog and pacing.
package provider

import "github.com/midweste/mcp-cli-gateway/internal/config"

// Provider abstracts a CLI agent backend.
// Implementations: GeminiProvider, CodexProvider, ClaudeProvider.
type Provider interface {
	// Name returns the provider identifier (e.g., "gemini", "codex", "claude").
	Name() string

	// Available returns true if the CLI binary is installed and callable.
	Available() bool

	// BuildCommand constructs the CLI args for non-interactive execution.
	// The prompt is delivered via stdin by the executor.
	BuildCommand(model string, cwd string, sandbox bool) []string

	// ParseOutput extracts response text and token stats from CLI stdout.
	// Stats keys match the database column names (tokens_in, tokens_out, etc.).
	ParseOutput(stdout string) (responseText string, stats map[string]any)

	// DetectRateLimit checks if the exit code + output indicate rate limiting.
	DetectRateLimit(exitCode int, stdout, stderr string) bool

	// SystemPrefix returns the system prompt prepended to every user prompt.
	SystemPrefix() string

	// TierModels returns the models this provider supports, grouped by tier.
	// Keys are tier names (e.g., "lite", "fast", "deep").
	// Values are ordered slices of model names — multiple models per tier
	// enables rate-limit dodging within a single provider.
	TierModels() map[string][]string

	// ModelPacing returns the pacing configuration for a specific model.
	// Providers return model-specific overrides where needed, falling back
	// to sensible provider-level defaults for unknown models.
	ModelPacing(model string) config.PacingConfig
}
