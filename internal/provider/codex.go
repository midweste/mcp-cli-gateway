package provider

import (
	"github.com/midweste/mcp-cli-gateway/internal/config"
	"encoding/json"
	"os/exec"
	"strings"
)

// CodexProvider dispatches jobs via the OpenAI Codex CLI.
type CodexProvider struct {
	systemPrefix string
}

// NewCodexProvider creates a CodexProvider with the given system prompt prefix.
func NewCodexProvider(systemPrefix string) *CodexProvider {
	return &CodexProvider{systemPrefix: systemPrefix}
}

func (c *CodexProvider) Name() string { return "codex" }

func (c *CodexProvider) Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (c *CodexProvider) BuildCommand(model string, cwd string, sandbox bool) []string {
	args := []string{"codex", "exec"}

	if sandbox {
		args = append(args, "--sandbox", "workspace-write")
	} else {
		args = append(args, "--full-auto")
	}

	args = append(args, "-m", model, "--json")

	if cwd != "" {
		args = append(args, "-C", cwd)
	}

	// "-" forces reading prompt from stdin, avoiding ARG_MAX limits.
	args = append(args, "-")

	return args
}

func (c *CodexProvider) ParseOutput(stdout string) (string, map[string]any) {
	stats := make(map[string]any)

	// Codex --json outputs JSONL (one JSON object per line).
	// We scan for the last message event containing response text.
	var responseText string

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event codexEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Extract response text from message events.
		if event.Type == "message" && event.Role == "assistant" {
			for _, part := range event.Content {
				if part.Type == "output_text" && part.Text != "" {
					responseText = part.Text
				}
			}
		}

		// Extract usage stats if present.
		if event.Usage.InputTokens > 0 {
			stats["tokens_in"] = event.Usage.InputTokens
		}
		if event.Usage.OutputTokens > 0 {
			stats["tokens_out"] = event.Usage.OutputTokens
		}
	}

	if responseText == "" {
		// Fallback: try parsing as a single JSON object.
		var single struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal([]byte(stdout), &single); err == nil && single.Result != "" {
			return single.Result, stats
		}
		return stdout, stats
	}

	return responseText, stats
}

// codexEvent represents a JSONL event from the Codex CLI.
type codexEvent struct {
	Type    string      `json:"type"`
	Role    string      `json:"role,omitempty"`
	Content []codexPart `json:"content,omitempty"`
	Usage   codexUsage  `json:"usage,omitempty"`
}

type codexPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

func (c *CodexProvider) DetectRateLimit(exitCode int, stdout, stderr string) bool {
	if exitCode == 0 {
		return false
	}
	lowerStderr := strings.ToLower(stderr)
	for _, sig := range codexRateLimitSignals {
		if strings.Contains(lowerStderr, sig) {
			return true
		}
	}
	return false
}

func (c *CodexProvider) SystemPrefix() string {
	return c.systemPrefix
}

func (c *CodexProvider) TierModels() map[string][]string {
	return map[string][]string{
		"lite": {"gpt-5.1-codex-mini"},
		"fast": {"gpt-5.3-codex"},
		"deep": {"gpt-5.4"},
	}
}

func (c *CodexProvider) ModelPacing(model string) config.PacingConfig {
	defaults := config.PacingConfig{
		InitialGapMs:  1000,
		FloorMs:       500,
		MaxConcurrent: 1,
		MaxQueue:      50,
	}
	switch model {
	case "gpt-5.1-codex-mini":
		defaults.InitialGapMs = 800
		defaults.FloorMs = 400
	case "gpt-5.4":
		defaults.InitialGapMs = 2000
		defaults.FloorMs = 1000
	}
	return defaults
}

// Codex-specific rate limit detection patterns.
var codexRateLimitSignals = []string{
	"429",
	"rate limit",
	"rate_limit",
	"too many requests",
	"capacity",
}
