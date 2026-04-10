package gateway

import (
	"testing"
	"time"

	"github.com/midweste/mcp-cli-gateway/internal/provider"
)

func TestPromptHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		check  func(t *testing.T, hash string)
	}{
		{
			name:  "Consistent",
			input: "hello world",
			check: func(t *testing.T, hash string) {
				second := PromptHash("hello world")
				if hash != second {
					t.Errorf("hash not consistent: %q != %q", hash, second)
				}
				if len(hash) != 12 {
					t.Errorf("hash length=%d, want 12", len(hash))
				}
			},
		},
		{
			name:  "Unique",
			input: "input A",
			check: func(t *testing.T, hash string) {
				other := PromptHash("input B")
				if hash == other {
					t.Errorf("different inputs produced same hash: %q", hash)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hash := PromptHash(tt.input)
			tt.check(t, hash)
		})
	}
}

func TestDetectRateLimit(t *testing.T) {
	t.Parallel()
	geminiProv := provider.NewGeminiProvider("")

	tests := []struct {
		name     string
		exitCode int
		stdout   string
		stderr   string
		want     bool
	}{
		{name: "ByExitCode/130", exitCode: 130, stdout: "ok", stderr: "", want: true},
		{name: "ByStderr/RESOURCE_EXHAUSTED", exitCode: 1, stdout: "", stderr: "RESOURCE_EXHAUSTED", want: true},
		{name: "ByStderr/429", exitCode: 1, stdout: "", stderr: "error 429 too many", want: true},
		{name: "ByStderr/quota", exitCode: 1, stdout: "", stderr: "quota exceeded", want: true},
		{name: "ExitZero_NeverRateLimited", exitCode: 0, stdout: "all good", stderr: "", want: false},
		{name: "ExitZero_SignalInStdout_NotRateLimited", exitCode: 0, stdout: "RESOURCE_EXHAUSTED", stderr: "", want: false},
		{name: "ExitZero_429InStdout_NotRateLimited", exitCode: 0, stdout: `{"tokens": {"input": 429}}`, stderr: "", want: false},
		{name: "OtherExitCode_NoSignal", exitCode: 1, stdout: "error", stderr: "", want: false},
		{name: "SignalInStdoutOnly_NotRateLimited", exitCode: 1, stdout: "quota exceeded", stderr: "normal error", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := geminiProv.DetectRateLimit(tt.exitCode, tt.stdout, tt.stderr)
			if got != tt.want {
				t.Errorf("DetectRateLimit=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		want time.Duration
	}{
		{name: "Hours", input: "1h", want: 1 * time.Hour},
		{name: "Days", input: "2d", want: 48 * time.Hour},
		{name: "Minutes", input: "30m", want: 30 * time.Minute},
		{name: "Empty", input: "", want: 0},
		{name: "SingleDigit", input: "5", want: 5 * time.Hour},
		{name: "DecimalHours", input: "1.5h", want: time.Duration(1.5 * float64(time.Hour))},
		{name: "InvalidInput", input: "xyz", want: 0},
		{name: "JustSuffix", input: "h", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseDuration(tt.input)
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
