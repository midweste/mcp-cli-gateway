package provider

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/midweste/mcp-cli-gateway/internal/config"
)

// Helper to compare string slices.
// The provided compareSlices has a bug and doesn't sort.
// This new helper will sort the slices before comparison to handle cases where order doesn't matter.
func compareStringSlices(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true // Both are nil or empty, consider them equal
	}
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

// MockProvider is a test implementation of the Provider interface.
type MockProvider struct {
	name          string
	available     bool
	systemPrefix  string
	buildCommand  func(model, cwd string, sandbox bool) []string
	parseOutput   func(stdout string) (string, map[string]any)
	detectRateLimit func(exitCode int, stdout, stderr string) bool
}

func (m *MockProvider) Name() string { return m.name }
func (m *MockProvider) Available() bool { return m.available }
func (m *MockProvider) BuildCommand(model, cwd string, sandbox bool) []string {
	if m.buildCommand != nil {
		return m.buildCommand(model, cwd, sandbox)
	}
	return nil
}
func (m *MockProvider) ParseOutput(stdout string) (string, map[string]any) {
	if m.parseOutput != nil {
		return m.parseOutput(stdout)
	}
	return stdout, nil
}
func (m *MockProvider) DetectRateLimit(exitCode int, stdout, stderr string) bool {
	if m.detectRateLimit != nil {
		return m.detectRateLimit(exitCode, stdout, stderr)
	}
	return false
}
func (m *MockProvider) SystemPrefix() string { return m.systemPrefix }
func (m *MockProvider) TierModels() map[string][]string {
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

func TestGeminiProvider_BuildCommand(t *testing.T) {
	t.Parallel()
	g := NewGeminiProvider("gemini-prefix")

	tests := []struct {
		name    string
		model   string
		cwd     string
		sandbox bool
		want    []string
	}{
		{
			name:    "sandbox true",
			model:   "gemini-pro",
			cwd:     "/tmp",
			sandbox: true,
			want:    []string{"gemini", "-m", "gemini-pro", "--sandbox", "false", "-o", "json"},
		},
		{
			name:    "sandbox false",
			model:   "gemini-flash",
			cwd:     "/tmp",
			sandbox: false,
			want:    []string{"gemini", "-m", "gemini-flash", "--yolo", "-o", "json"},
		},
		{
			name:    "no cwd",
			model:   "gemini-pro",
			cwd:     "",
			sandbox: true,
			want:    []string{"gemini", "-m", "gemini-pro", "--sandbox", "false", "-o", "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.BuildCommand(tt.model, tt.cwd, tt.sandbox)
			if !compareBuildCmd(got, tt.want) {
				t.Errorf("BuildCommand() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeminiProvider_ParseOutput(t *testing.T) {
	t.Parallel()
	g := NewGeminiProvider("gemini-prefix")

	tests := []struct {
		name          string
		stdout        string
		wantText      string
		wantStats     map[string]any
	}{
		{
			name:     "valid json with all fields",
			stdout:   `{"response":"hello from gemini","stats":{"models":{"gemini-pro":{"tokens":{"input":10,"candidates":20,"cached":5,"thoughts":3},"api":{"totalLatencyMs":100}}},"tools":{"totalCalls":1}}}`,
			wantText: "hello from gemini",
			wantStats: map[string]any{
				"tokens_in":        10,
				"tokens_out":       20,
				"tokens_cached":    5,
				"tokens_thoughts":  3,
				"api_latency_ms":   100,
				"tool_calls":       1,
			},
		},
		{
			name:     "invalid json",
			stdout:   `this is not json`,
			wantText: `this is not json`,
			wantStats: nil, // json.Unmarshal returns error, stats remains empty
		},
		{
			name:     "empty stdout",
			stdout:   "",
			wantText: "",
			wantStats: nil,
		},
		{
			name:     "json with missing fields",
			stdout:   `{"response":"partial response","stats":{"models":{"gemini-pro":{}}}}`,
			wantText: "partial response",
			wantStats: nil, // Only existing fields are set. Here, none of the token/latency fields are present.
		},
		{
			name:     "json with zero values",
			stdout:   `{"response":"zero values","stats":{"models":{"gemini-pro":{"tokens":{"input":0,"candidates":0,"cached":0,"thoughts":0},"api":{"totalLatencyMs":0}}},"tools":{"totalCalls":0}}}`,
			wantText: "zero values",
			wantStats: map[string]any{
				"tokens_in":        0,
				"tokens_out":       0,
				"tokens_cached":    0,
				"tokens_thoughts":  0,
				"api_latency_ms":   0,
				"tool_calls":       0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotStats := g.ParseOutput(tt.stdout)
			if gotText != tt.wantText {
				t.Errorf("ParseOutput() gotText = %q, want %q", gotText, tt.wantText)
			}
			if !compareMaps(gotStats, tt.wantStats) {
				t.Errorf("ParseOutput() gotStats = %v, want %v", gotStats, tt.wantStats)
			}
		})
	}
}

func TestGeminiProvider_DetectRateLimit(t *testing.T) {
	t.Parallel()
	g := NewGeminiProvider("gemini-prefix")

	tests := []struct {
		name     string
		exitCode int
		stdout   string
		stderr   string
		want     bool
	}{
		{
			name:     "exit code 0 (success)",
			exitCode: 0,
			stdout:   "",
			stderr:   "",
			want:     false,
		},
		{
			name:     "gemini specific exit code",
			exitCode: geminiRateLimitExitCode,
			stdout:   "",
			stderr:   "",
			want:     true,
		},
		{
			name:     "stderr contains rate limit signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "Error: 429 Too Many Requests",
			want:     true,
		},
		{
			name:     "stderr contains other signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "RESOURCE_EXHAUSTED",
			want:     true,
		},
		{
			name:     "stderr contains quota signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "exhausted your capacity",
			want:     true,
		},
		{
			name:     "stdout contains rate limit signal (false positive)",
			exitCode: 1,
			stdout:   `{"response":"429 is a number"}`,
			stderr:   "",
			want:     false,
		},
		{
			name:     "no rate limit signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "some other error",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.DetectRateLimit(tt.exitCode, tt.stdout, tt.stderr)
			if got != tt.want {
				t.Errorf("DetectRateLimit() got = %v, want %v for exitCode %d, stderr %q", got, tt.want, tt.exitCode, tt.stderr)
			}
		})
	}
}

func TestGeminiProvider_SystemPrefix(t *testing.T) {
	t.Parallel()
	prefix := "test-gemini-prefix"
	g := NewGeminiProvider(prefix)
	if got := g.SystemPrefix(); got != prefix {
		t.Errorf("SystemPrefix() got = %q, want %q", got, prefix)
	}
}

func TestCodexProvider_BuildCommand(t *testing.T) {
	t.Parallel()
	c := NewCodexProvider("codex-prefix")

	tests := []struct {
		name    string
		model   string
		cwd     string
		sandbox bool
		want    []string
	}{
		{
			name:    "sandbox true, with cwd",
			model:   "codex-m",
			cwd:     "/home/user/project",
			sandbox: true,
			want:    []string{"codex", "exec", "--sandbox", "workspace-write", "-m", "codex-m", "--json", "-C", "/home/user/project", "-"},
		},
		{
			name:    "sandbox false, no cwd",
			model:   "codex-f",
			cwd:     "",
			sandbox: false,
			want:    []string{"codex", "exec", "--full-auto", "-m", "codex-f", "--json", "-"},
		},
		{
			name:    "sandbox true, no cwd",
			model:   "codex-m",
			cwd:     "",
			sandbox: true,
			want:    []string{"codex", "exec", "--sandbox", "workspace-write", "-m", "codex-m", "--json", "-"},
		},
		{
			name:    "sandbox false, with cwd",
			model:   "codex-f",
			cwd:     "/home/user/project",
			sandbox: false,
			want:    []string{"codex", "exec", "--full-auto", "-m", "codex-f", "--json", "-C", "/home/user/project", "-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.BuildCommand(tt.model, tt.cwd, tt.sandbox)
			if !compareBuildCmd(got, tt.want) {
				t.Errorf("BuildCommand() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodexProvider_ParseOutput(t *testing.T) {
	t.Parallel()
	c := NewCodexProvider("codex-prefix")

	tests := []struct {
		name          string
		stdout        string
		wantText      string
		wantStats     map[string]any
	}{
		{
			name: "valid jsonl with message and usage",
			stdout: `{"type":"event","data":{"step":"start"}}
{"type":"message","role":"assistant","content":[{"type":"output_text","text":"response part 1"}]}
{"type":"event","usage":{"input_tokens":10,"output_tokens":20}}
{"type":"message","role":"assistant","content":[{"type":"output_text","text":"final response"}]}`,
			wantText: "final response",
			wantStats: map[string]any{
				"tokens_in":  10,
				"tokens_out": 20,
			},
		},
		{
			name:     "empty stdout",
			stdout:   "",
			wantText: "",
			wantStats: nil,
		},
		{
			name:     "single json object fallback",
			stdout:   `{"result":"fallback response"}`,
			wantText: "fallback response",
			wantStats: nil,
		},
		{
			name:     "invalid jsonl",
			stdout:   `not jsonl`,
			wantText: `not jsonl`,
			wantStats: nil,
		},
		{
			name: "jsonl with multiple messages, no usage",
			stdout: `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first part"}]}
{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second part"}]}`,
			wantText: "second part",
			wantStats: nil,
		},
		{
			name: "jsonl with usage in first event",
			stdout: `{"type":"event","usage":{"input_tokens":100,"output_tokens":200}}
{"type":"message","role":"assistant","content":[{"type":"output_text","text":"response"}]}`,
			wantText: "response",
			wantStats: map[string]any{
				"tokens_in":  100,
				"tokens_out": 200,
			},
		},
		{
			name: "jsonl with no response text",
			stdout: `{"type":"event","usage":{"input_tokens":10,"output_tokens":20}}
{"type":"event","data":{"step":"end"}}`,
			wantText: `{"type":"event","usage":{"input_tokens":10,"output_tokens":20}}
{"type":"event","data":{"step":"end"}}`, // Expect raw stdout as response text
			wantStats: map[string]any{
				"tokens_in":  10,
				"tokens_out": 20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotStats := c.ParseOutput(tt.stdout)
			if gotText != tt.wantText {
				t.Errorf("ParseOutput() gotText = %q, want %q", gotText, tt.wantText)
			}
			if !compareMaps(gotStats, tt.wantStats) {
				t.Errorf("ParseOutput() gotStats = %v, want %v", gotStats, tt.wantStats)
			}
		})
	}
}

func TestCodexProvider_DetectRateLimit(t *testing.T) {
	t.Parallel()
	c := NewCodexProvider("codex-prefix")

	tests := []struct {
		name     string
		exitCode int
		stdout   string
		stderr   string
		want     bool
	}{
		{
			name:     "exit code 0 (success)",
			exitCode: 0,
			stdout:   "",
			stderr:   "",
			want:     false,
		},
		{
			name:     "stderr contains rate limit signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "Error: 429 Too Many Requests",
			want:     true,
		},
		{
			name:     "stderr contains other signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "rate limit exceeded",
			want:     true,
		},
		{
			name:     "stdout contains rate limit signal (false positive)",
			exitCode: 1,
			stdout:   `{"response":"429 is a number"}`,
			stderr:   "",
			want:     false,
		},
		{
			name:     "no rate limit signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "some other error",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.DetectRateLimit(tt.exitCode, tt.stdout, tt.stderr)
			if got != tt.want {
				t.Errorf("DetectRateLimit() got = %v, want %v for exitCode %d, stderr %q", got, tt.want, tt.exitCode, tt.stderr)
			}
		})
	}
}

func TestCodexProvider_SystemPrefix(t *testing.T) {
	t.Parallel()
	prefix := "test-codex-prefix"
	c := NewCodexProvider(prefix)
	if got := c.SystemPrefix(); got != prefix {
		t.Errorf("SystemPrefix() got = %q, want %q", got, prefix)
	}
}

func TestClaudeProvider_BuildCommand(t *testing.T) {
	t.Parallel()
	c := NewClaudeProvider("claude-prefix")

	tests := []struct {
		name    string
		model   string
		cwd     string
		sandbox bool
		want    []string
	}{
		{
			name:    "sandbox true",
			model:   "claude-3-opus",
			cwd:     "/tmp",
			sandbox: true,
			want:    []string{"claude", "-p", "--model", "claude-3-opus", "--output-format", "json"},
		},
		{
			name:    "sandbox false",
			model:   "claude-3-sonnet",
			cwd:     "/tmp",
			sandbox: false,
			want:    []string{"claude", "-p", "--model", "claude-3-sonnet", "--output-format", "json", "--dangerously-skip-permissions"},
		},
		{
			name:    "no cwd",
			model:   "claude-3-opus",
			cwd:     "",
			sandbox: true,
			want:    []string{"claude", "-p", "--model", "claude-3-opus", "--output-format", "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.BuildCommand(tt.model, tt.cwd, tt.sandbox)
			if !compareBuildCmd(got, tt.want) {
				t.Errorf("BuildCommand() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaudeProvider_ParseOutput(t *testing.T) {
	t.Parallel()
	c := NewClaudeProvider("claude-prefix")

	tests := []struct {
		name          string
		stdout        string
		wantText      string
		wantStats     map[string]any
	}{
		{
			name:     "valid json with all fields",
			stdout:   `{"type":"message","subtype":"text","cost_usd":0.001,"duration_ms":123,"is_error":false,"num_turns":1,"result":"hello from claude","session_id":"abc","usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":5}}`,
			wantText: "hello from claude",
			wantStats: map[string]any{
				"api_latency_ms": 123,
				"tool_calls":     1,
				"cost_usd":       0.001,
				"tokens_in":      10,
				"tokens_out":     20,
				"tokens_cached":  5,
			},
		},
		{
			name:     "invalid json",
			stdout:   `this is not json`,
			wantText: `this is not json`,
			wantStats: nil,
		},
		{
			name:     "empty stdout",
			stdout:   "",
			wantText: "",
			wantStats: nil,
		},
		{
			name:     "json with missing fields",
			stdout:   `{"type":"message","result":"partial response"}`,
			wantText: "partial response",
			wantStats: nil,
		},
		{
			name:     "json with zero values",
			stdout:   `{"type":"message","cost_usd":0,"duration_ms":0,"num_turns":0,"result":"zero values","usage":{"input_tokens":0,"output_tokens":0,"cache_read":0}}`,
			wantText: "zero values",
			wantStats: nil, // All values are 0, so no stats should be added to the map.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotStats := c.ParseOutput(tt.stdout)
			if gotText != tt.wantText {
				t.Errorf("ParseOutput() gotText = %q, want %q", gotText, tt.wantText)
			}
			if !compareMaps(gotStats, tt.wantStats) {
				t.Errorf("ParseOutput() gotStats = %v, want %v", gotStats, tt.wantStats)
			}
		})
	}
}

func TestClaudeProvider_DetectRateLimit(t *testing.T) {
	t.Parallel()
	c := NewClaudeProvider("claude-prefix")

	tests := []struct {
		name     string
		exitCode int
		stdout   string
		stderr   string
		want     bool
	}{
		{
			name:     "exit code 0 (success)",
			exitCode: 0,
			stdout:   "",
			stderr:   "",
			want:     false,
		},
		{
			name:     "stderr contains rate limit signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "Error: 429 Too Many Requests",
			want:     true,
		},
		{
			name:     "stderr contains other signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "rate limit exceeded",
			want:     true,
		},
		{
			name:     "stdout contains rate limit signal (false positive)",
			exitCode: 1,
			stdout:   `{"result":"429 is a number"}`,
			stderr:   "",
			want:     false,
		},
		{
			name:     "no rate limit signal",
			exitCode: 1,
			stdout:   "",
			stderr:   "some other error",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.DetectRateLimit(tt.exitCode, tt.stdout, tt.stderr)
			if got != tt.want {
				t.Errorf("DetectRateLimit() got = %v, want %v for exitCode %d, stderr %q", got, tt.want, tt.exitCode, tt.stderr)
			}
		})
	}
}

func TestClaudeProvider_SystemPrefix(t *testing.T) {
	t.Parallel()
	prefix := "test-claude-prefix"
	c := NewClaudeProvider(prefix)
	if got := c.SystemPrefix(); got != prefix {
		t.Errorf("SystemPrefix() got = %q, want %q", got, prefix)
	}
}

func TestRegistry_NewRegistry(t *testing.T) {
	t.Parallel()
	aliasMap := map[string]string{
		"gemini-fast": "gemini",
		"codex-dev":   "codex",
	}

	tests := []struct {
		name          string
		providers     []Provider
		wantAvailable []string
		wantCount     int
	}{
		{
			name: "all available",
			providers: []Provider{
				&MockProvider{name: "gemini", available: true},
				&MockProvider{name: "codex", available: true},
				&MockProvider{name: "claude", available: true},
			},
			wantAvailable: []string{"claude", "codex", "gemini"}, // Sorted alphabetically
			wantCount:     3,
		},
		{
			name: "some available",
			providers: []Provider{
				&MockProvider{name: "gemini", available: true},
				&MockProvider{name: "codex", available: false},
				&MockProvider{name: "claude", available: true},
			},
			wantAvailable: []string{"claude", "gemini"}, // Sorted alphabetically
			wantCount:     2,
		},
		{
			name: "none available",
			providers: []Provider{
				&MockProvider{name: "gemini", available: false},
				&MockProvider{name: "codex", available: false},
			},
			wantAvailable: nil,
			wantCount:     0,
		},
		{
			name:          "no providers given",
			providers:     []Provider{},
			wantAvailable: nil,
			wantCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(aliasMap, tt.providers...)

			gotAvailable := reg.Available()
			if !compareStringSlices(gotAvailable, tt.wantAvailable) {
				t.Errorf("NewRegistry() Available() got = %v, want %v", gotAvailable, tt.wantAvailable)
			}
			if gotCount := reg.Count(); gotCount != tt.wantCount {
				t.Errorf("NewRegistry() Count() got = %v, want %v", gotCount, tt.wantCount)
			}
		})
	}
}

func TestRegistry_Get(t *testing.T) {
	t.Parallel()
	mockGemini := &MockProvider{name: "gemini", available: true}
	reg := NewRegistry(nil, mockGemini)

	tests := []struct {
		name      string
		provider  string
		wantFound bool
		wantName  string
	}{
		{
			name:      "existing provider",
			provider:  "gemini",
			wantFound: true,
			wantName:  "gemini",
		},
		{
			name:      "non-existent provider",
			provider:  "nonexistent",
			wantFound: false,
			wantName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := reg.Get(tt.provider)
			if ok != tt.wantFound {
				t.Errorf("Get() got ok = %v, want %v", ok, tt.wantFound)
			}
			if ok && p.Name() != tt.wantName {
				t.Errorf("Get() got name = %q, want %q", p.Name(), tt.wantName)
			}
		})
	}
}

func TestRegistry_ForAlias(t *testing.T) {
	t.Parallel()
	mockGemini := &MockProvider{name: "gemini", available: true}
	mockCodexUnavailable := &MockProvider{name: "codex", available: false}

	aliasMap := map[string]string{
		"gemini-fast": "gemini",
		"codex-dev":   "codex", // Points to an unavailable provider
		"unknown-alias": "unknown",
	}
	reg := NewRegistry(aliasMap, mockGemini, mockCodexUnavailable)

	tests := []struct {
		name      string
		alias     string
		wantFound bool
		wantName  string
	}{
		{
			name:      "existing alias for available provider",
			alias:     "gemini-fast",
			wantFound: true,
			wantName:  "gemini",
		},
		{
			name:      "alias for unavailable provider",
			alias:     "codex-dev",
			wantFound: false,
			wantName:  "",
		},
		{
			name:      "non-existent alias",
			alias:     "nonexistent-alias",
			wantFound: false,
			wantName:  "",
		},
		{
			name:      "alias for unknown provider",
			alias:     "unknown-alias",
			wantFound: false, // Provider "unknown" is not in the registry
			wantName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := reg.ForAlias(tt.alias)
			if ok != tt.wantFound {
				t.Errorf("ForAlias() got ok = %v, want %v", ok, tt.wantFound)
			}
			if ok && p.Name() != tt.wantName {
				t.Errorf("ForAlias() got name = %q, want %q", p.Name(), tt.wantName)
			}
		})
	}
}

func TestRegistry_Available(t *testing.T) {
	t.Parallel()
	mockGemini := &MockProvider{name: "gemini", available: true}
	mockCodex := &MockProvider{name: "codex", available: true}
	mockClaudeUnavailable := &MockProvider{name: "claude", available: false}

	reg := NewRegistry(nil, mockGemini, mockCodex, mockClaudeUnavailable)

	want := []string{"codex", "gemini"} // Names are stored in a map, order is not guaranteed. Sort for comparison.
	got := reg.Available()
	if !compareStringSlices(got, want) {
		t.Errorf("Available() got = %v, want %v", got, want)
	}
}

func TestRegistry_Count(t *testing.T) {
	t.Parallel()
	mockGemini := &MockProvider{name: "gemini", available: true}
	mockCodex := &MockProvider{name: "codex", available: true}
	mockClaudeUnavailable := &MockProvider{name: "claude", available: false}

	reg := NewRegistry(nil, mockGemini, mockCodex, mockClaudeUnavailable)
	if got := reg.Count(); got != 2 {
		t.Errorf("Count() got = %v, want %v", got, 2)
	}

	regEmpty := NewRegistry(nil)
	if got := regEmpty.Count(); got != 0 {
		t.Errorf("Count() got = %v, want %v for empty registry", got, 0)
	}
}

func TestRegistry_MustHaveProviders(t *testing.T) {
	t.Parallel()
	t.Run("panics when no providers are available", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MustHaveProviders() did not panic when no providers were available")
			} else {
				t.Logf("Expected panic: %v", r)
			}
		}()
		reg := NewRegistry(nil)
		reg.MustHaveProviders()
	})
	t.Run("does not panic when providers are available", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustHaveProviders() panicked unexpectedly when providers were available: %v", r)
			}
		}()
		mockGemini := &MockProvider{name: "gemini", available: true}
		reg := NewRegistry(nil, mockGemini)
		reg.MustHaveProviders()
	})
}

// compareBuildCmd compares BuildCommand output allowing args[0] to be a full path.
// Tests specify bare names ("gemini"); LookPathWithShell may resolve to full paths.
func compareBuildCmd(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if i == 0 {
			// Compare basename only for the binary path.
			if filepath.Base(got[0]) != filepath.Base(want[0]) {
				return false
			}
			continue
		}
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Helper to compare string slices, ignoring order for NewRegistry's Available() since map iteration order is not guaranteed.
func compareSlices[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	// For small slices, this simple approach is fine. For larger, use a map for frequency count.
	// For strings, sorting might be better if order doesn't matter and elements are unique.
	// Given the context of provider names, uniqueness and small size are likely.
	// A simple check where elements of 'a' exist in 'b' and vice versa.
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	aMap := make(map[T]int)
	bMap := make(map[T]int)

	for _, x := range a {
		aMap[x]++
	}
	for _, x := range b {
		bMap[x]++
	}

	for k, v := range aMap {
		if bMap[k] != v {
			return false
		}
	}
	for k, v := range bMap {
		if aMap[k] != v {
			return false
		}
	}
	return true
}

// Helper to compare maps
func compareMaps(a, b map[string]any) bool {
	// Treat nil map and empty map as equal
	if (a == nil || len(a) == 0) && (b == nil || len(b) == 0) {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !interfaceEquals(v, bv) {
			return false
		}
	}
	return true
}

// Helper to compare interface{} values, especially for numbers which might be int or float64.
func interfaceEquals(a, b any) bool {
	if a == b {
		return true
	}

	// Handle numeric types which might be unmarshaled as float64 by default.
	if fa, ok := a.(float64); ok {
		if fb, ok := b.(int); ok {
			return fa == float64(fb)
		}
		if fb, ok := b.(float64); ok {
			return fa == fb
		}
	}
	if ia, ok := a.(int); ok {
		if fb, ok := b.(float64); ok {
			return float64(ia) == fb
		}
		if ib, ok := b.(int); ok {
			return ia == ib
		}
	}
	// Compare string, bool, etc. directly.
	return false
}
