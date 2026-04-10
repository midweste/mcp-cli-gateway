package provider

import (
	"github.com/midweste/mcp-cli-gateway/internal/config"
	"encoding/json"
	"os/exec"
	"strings"
)

// GeminiProvider dispatches jobs via the Google Gemini CLI.
type GeminiProvider struct {
	systemPrefix string
}

// NewGeminiProvider creates a GeminiProvider with the given system prompt prefix.
func NewGeminiProvider(systemPrefix string) *GeminiProvider {
	return &GeminiProvider{systemPrefix: systemPrefix}
}

func (g *GeminiProvider) Name() string { return "gemini" }

func (g *GeminiProvider) Available() bool {
	_, err := exec.LookPath("gemini")
	return err == nil
}

func (g *GeminiProvider) BuildCommand(model string, cwd string, sandbox bool) []string {
	if sandbox {
		return []string{"gemini", "-m", model, "--sandbox", "false", "-o", "json"}
	}
	return []string{"gemini", "-m", model, "--yolo", "-o", "json"}
}

func (g *GeminiProvider) ParseOutput(stdout string) (string, map[string]any) {
	stats := make(map[string]any)

	var data struct {
		Response string `json:"response"`
		Stats    struct {
			Models map[string]struct {
				Tokens struct {
					Input      *int `json:"input"`
					Candidates *int `json:"candidates"`
					Cached     *int `json:"cached"`
					Thoughts   *int `json:"thoughts"`
				} `json:"tokens"`
				API struct {
					TotalLatencyMs *int `json:"totalLatencyMs"`
				} `json:"api"`
			} `json:"models"`
			Tools struct {
				TotalCalls *int `json:"totalCalls"`
			} `json:"tools"`
		} `json:"stats"`
	}

	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		return stdout, stats
	}

	for _, modelStats := range data.Stats.Models {
		if modelStats.Tokens.Input != nil {
			stats["tokens_in"] = *modelStats.Tokens.Input
		}
		if modelStats.Tokens.Candidates != nil {
			stats["tokens_out"] = *modelStats.Tokens.Candidates
		}
		if modelStats.Tokens.Cached != nil {
			stats["tokens_cached"] = *modelStats.Tokens.Cached
		}
		if modelStats.Tokens.Thoughts != nil {
			stats["tokens_thoughts"] = *modelStats.Tokens.Thoughts
		}
		if modelStats.API.TotalLatencyMs != nil {
			stats["api_latency_ms"] = *modelStats.API.TotalLatencyMs
		}
		break // Only first model
	}
	if data.Stats.Tools.TotalCalls != nil {
		stats["tool_calls"] = *data.Stats.Tools.TotalCalls
	}

	return data.Response, stats
}

func (g *GeminiProvider) DetectRateLimit(exitCode int, stdout, stderr string) bool {
	// Exit code 0 = success — never a rate limit.
	if exitCode == 0 {
		return false
	}
	if exitCode == geminiRateLimitExitCode {
		return true
	}
	// Only check stderr for rate-limit signals — stdout contains JSON with
	// numbers (token counts, latencies) that cause false positives (e.g., "429"
	// matching a token count of 429).
	lowerStderr := strings.ToLower(stderr)
	for _, sig := range geminiRateLimitSignals {
		if strings.Contains(lowerStderr, strings.ToLower(sig)) {
			return true
		}
	}
	return false
}

func (g *GeminiProvider) SystemPrefix() string {
	return g.systemPrefix
}

func (g *GeminiProvider) TierModels() map[string][]string {
	return map[string][]string{
		"lite": {"gemini-2.5-flash-lite"},
		"fast": {"gemini-2.5-flash"},
		"deep": {"gemini-2.5-pro", "gemini-3.1-pro-preview"},
	}
}

func (g *GeminiProvider) ModelPacing(model string) config.PacingConfig {
	defaults := config.PacingConfig{
		InitialGapMs:  2000,
		FloorMs:       1500,
		MaxConcurrent: 1,
		MaxQueue:      50,
	}
	switch model {
	case "gemini-2.5-flash-lite":
		defaults.InitialGapMs = 1500
		defaults.FloorMs = 1000
	case "gemini-2.5-pro", "gemini-3.1-pro-preview":
		defaults.InitialGapMs = 3000
		defaults.FloorMs = 2000
	}
	return defaults
}

// Gemini-specific rate limit detection constants.
var geminiRateLimitSignals = []string{
	"429",
	"RESOURCE_EXHAUSTED",
	"rate limit",
	"quota",
	"exhausted your capacity",
}

const geminiRateLimitExitCode = 130
