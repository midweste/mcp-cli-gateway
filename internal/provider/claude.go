package provider

import (
	"github.com/midweste/mcp-cli-gateway/internal/config"
	"encoding/json"
	"os/exec"
	"strings"
)

// ClaudeProvider dispatches jobs via the Anthropic Claude Code CLI.
type ClaudeProvider struct {
	systemPrefix string
}

// NewClaudeProvider creates a ClaudeProvider with the given system prompt prefix.
func NewClaudeProvider(systemPrefix string) *ClaudeProvider {
	return &ClaudeProvider{systemPrefix: systemPrefix}
}

func (c *ClaudeProvider) Name() string { return "claude" }

func (c *ClaudeProvider) Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (c *ClaudeProvider) BuildCommand(model string, cwd string, sandbox bool) []string {
	args := []string{
		"claude",
		"-p",
		"--model", model,
		"--output-format", "json",
	}

	if !sandbox {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Prompt is read from stdin in -p mode when no positional arg is given.
	return args
}

func (c *ClaudeProvider) ParseOutput(stdout string) (string, map[string]any) {
	stats := make(map[string]any)

	// Claude --output-format json returns a single JSON result object.
	var result claudeResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		// If not JSON, return raw stdout.
		return stdout, stats
	}

	if result.DurationMs > 0 {
		stats["api_latency_ms"] = result.DurationMs
	}
	if result.NumTurns > 0 {
		stats["tool_calls"] = result.NumTurns
	}
	if result.CostUSD > 0 {
		stats["cost_usd"] = result.CostUSD
	}
	if result.Usage.InputTokens > 0 {
		stats["tokens_in"] = result.Usage.InputTokens
	}
	if result.Usage.OutputTokens > 0 {
		stats["tokens_out"] = result.Usage.OutputTokens
	}
	if result.Usage.CacheRead > 0 {
		stats["tokens_cached"] = result.Usage.CacheRead
	}

	return result.Result, stats
}

// claudeResult represents the JSON output from Claude CLI in -p --output-format json mode.
type claudeResult struct {
	Type       string      `json:"type"`
	Subtype    string      `json:"subtype"`
	CostUSD    float64     `json:"cost_usd"`
	DurationMs int         `json:"duration_ms"`
	IsError    bool        `json:"is_error"`
	NumTurns   int         `json:"num_turns"`
	Result     string      `json:"result"`
	SessionID  string      `json:"session_id"`
	Usage      claudeUsage `json:"usage,omitempty"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	CacheRead    int `json:"cache_read_input_tokens,omitempty"`
}

func (c *ClaudeProvider) DetectRateLimit(exitCode int, stdout, stderr string) bool {
	if exitCode == 0 {
		return false
	}
	lowerStderr := strings.ToLower(stderr)
	for _, sig := range claudeRateLimitSignals {
		if strings.Contains(lowerStderr, sig) {
			return true
		}
	}
	return false
}

func (c *ClaudeProvider) SystemPrefix() string {
	return c.systemPrefix
}

func (c *ClaudeProvider) TierModels() map[string][]string {
	return map[string][]string{
		"lite": {"haiku"},
		"fast": {"sonnet"},
		"deep": {"opus"},
	}
}

func (c *ClaudeProvider) ModelPacing(model string) config.PacingConfig {
	defaults := config.PacingConfig{
		InitialGapMs:  1500,
		FloorMs:       1000,
		MaxConcurrent: 1,
		MaxQueue:      50,
	}
	switch model {
	case "opus":
		defaults.InitialGapMs = 2500
		defaults.FloorMs = 1500
	}
	return defaults
}

// Claude-specific rate limit detection patterns.
var claudeRateLimitSignals = []string{
	"429",
	"rate limit",
	"rate_limit",
	"overloaded",
	"too many requests",
	"capacity",
}
