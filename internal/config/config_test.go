package config

import (
	"testing"
)

// testProvider is a minimal ProviderDescriptor for config tests.
type testProvider struct {
	name       string
	tierModels map[string][]string
	pacing     map[string]PacingConfig
}

func (tp *testProvider) Name() string                       { return tp.name }
func (tp *testProvider) TierModels() map[string][]string    { return tp.tierModels }
func (tp *testProvider) ModelPacing(model string) PacingConfig {
	if p, ok := tp.pacing[model]; ok {
		return p
	}
	return PacingConfig{InitialGapMs: 1000, FloorMs: 500, MaxConcurrent: 1, MaxQueue: 50}
}

func newGeminiTestProvider() *testProvider {
	return &testProvider{
		name: "gemini",
		tierModels: map[string][]string{
			"lite": {"gemini-2.5-flash-lite"},
			"fast": {"gemini-2.5-flash"},
			"deep": {"gemini-2.5-pro", "gemini-3.1-pro-preview"},
		},
		pacing: map[string]PacingConfig{
			"gemini-2.5-flash-lite":  {InitialGapMs: 1500, FloorMs: 1000, MaxConcurrent: 1, MaxQueue: 50},
			"gemini-2.5-flash":       {InitialGapMs: 2000, FloorMs: 1500, MaxConcurrent: 1, MaxQueue: 50},
			"gemini-2.5-pro":         {InitialGapMs: 3000, FloorMs: 2000, MaxConcurrent: 1, MaxQueue: 50},
			"gemini-3.1-pro-preview": {InitialGapMs: 3000, FloorMs: 2000, MaxConcurrent: 1, MaxQueue: 50},
		},
	}
}

func newClaudeTestProvider() *testProvider {
	return &testProvider{
		name: "claude",
		tierModels: map[string][]string{
			"lite": {"haiku"},
			"fast": {"sonnet"},
			"deep": {"opus"},
		},
		pacing: map[string]PacingConfig{
			"haiku":  {InitialGapMs: 1500, FloorMs: 1000, MaxConcurrent: 1, MaxQueue: 50},
			"sonnet": {InitialGapMs: 1500, FloorMs: 1000, MaxConcurrent: 1, MaxQueue: 50},
			"opus":   {InitialGapMs: 2500, FloorMs: 1500, MaxConcurrent: 1, MaxQueue: 50},
		},
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check func(t *testing.T, cfg *Config)
	}{
		{
			name: "MergedModelsPopulated",
			check: func(t *testing.T, cfg *Config) {
				provs := []ProviderDescriptor{newGeminiTestProvider()}
				cfg.Merge(provs)
				// gemini has 4 models across 3 tiers (deep has 2)
				if len(cfg.Models) != 4 {
					t.Errorf("len(Models)=%d, want 4; Models=%v", len(cfg.Models), cfg.Models)
				}
			},
		},
		{
			name: "PacingParamsNonZero",
			check: func(t *testing.T, cfg *Config) {
				provs := []ProviderDescriptor{newGeminiTestProvider()}
				cfg.Merge(provs)
				for alias, gapMs := range cfg.InitialGapMs {
					if gapMs <= 0 {
						t.Errorf("InitialGapMs[%q]=%d, want > 0", alias, gapMs)
					}
				}
			},
		},
		{
			name: "MaxConcurrentAndQueue",
			check: func(t *testing.T, cfg *Config) {
				provs := []ProviderDescriptor{newGeminiTestProvider()}
				cfg.Merge(provs)
				for alias, mc := range cfg.MaxConcurrent {
					if mc <= 0 {
						t.Errorf("MaxConcurrent[%q]=%d, want > 0", alias, mc)
					}
				}
				for alias, mq := range cfg.MaxQueue {
					if mq <= 0 {
						t.Errorf("MaxQueue[%q]=%d, want > 0", alias, mq)
					}
				}
			},
		},
		{
			name: "ModelBuckets",
			check: func(t *testing.T, cfg *Config) {
				provs := []ProviderDescriptor{newGeminiTestProvider()}
				cfg.Merge(provs)
				if len(cfg.ModelBuckets) != 3 {
					t.Errorf("len(ModelBuckets)=%d, want 3", len(cfg.ModelBuckets))
				}
			},
		},
		{
			name: "SystemPrefix",
			check: func(t *testing.T, cfg *Config) {
				if cfg.SystemPrefix == "" {
					t.Error("SystemPrefix empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.check(t, cfg)
		})
	}
}

func TestInitialGapForAlias(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Merge([]ProviderDescriptor{newGeminiTestProvider()})

	tests := []struct {
		alias string
		want  int
	}{
		{"gemini-fast", 2000},
		{"nonexistent", 1200}, // default
		{"gemini-deep", 3000},
	}
	for _, tt := range tests {
		got := cfg.InitialGapForAlias(tt.alias)
		if got != tt.want {
			t.Errorf("InitialGapForAlias(%q) = %d, want %d", tt.alias, got, tt.want)
		}
	}
}

func TestFloorForAlias(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Merge([]ProviderDescriptor{newGeminiTestProvider()})

	tests := []struct {
		alias string
		want  int
	}{
		{"gemini-fast", 1500},
		{"nonexistent", 800}, // default
		{"gemini-deep", 2000},
	}
	for _, tt := range tests {
		got := cfg.FloorForAlias(tt.alias)
		if got != tt.want {
			t.Errorf("FloorForAlias(%q) = %d, want %d", tt.alias, got, tt.want)
		}
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	// These tests set real env vars so they can't be parallel.
	setEnv := func(t *testing.T, key, val string) {
		t.Helper()
		t.Setenv(key, val)
	}

	t.Run("int overrides", func(t *testing.T) {
		setEnv(t, "GATEWAY_TIMEOUT_SECONDS", "999")
		setEnv(t, "GATEWAY_MAX_RETRIES", "7")
		setEnv(t, "GATEWAY_CLEANUP_DAYS", "30")
		setEnv(t, "GATEWAY_CEILING_MS", "20000")
		setEnv(t, "GATEWAY_BACKOFF_INITIAL_MS", "3000")
		setEnv(t, "GATEWAY_BACKOFF_MAX_MS", "16000")
		setEnv(t, "GATEWAY_STREAK_THRESHOLD", "5")

		cfg := Default()
		cfg.LoadEnvOverrides()

		if cfg.TIMEOUT_SECONDS() != 999 {
			t.Errorf("TIMEOUT_SECONDS() = %d, want 999", cfg.TIMEOUT_SECONDS())
		}
		if cfg.MAX_RETRIES() != 7 {
			t.Errorf("MAX_RETRIES() = %d, want 7", cfg.MAX_RETRIES())
		}
		if cfg.CLEANUP_DAYS() != 30 {
			t.Errorf("CLEANUP_DAYS() = %d, want 30", cfg.CLEANUP_DAYS())
		}
		if cfg.CEILING_MS() != 20000 {
			t.Errorf("CEILING_MS() = %d, want 20000", cfg.CEILING_MS())
		}
		if cfg.BACKOFF_INITIAL_MS() != 3000 {
			t.Errorf("BACKOFF_INITIAL_MS() = %d, want 3000", cfg.BACKOFF_INITIAL_MS())
		}
		if cfg.BACKOFF_MAX_MS() != 16000 {
			t.Errorf("BACKOFF_MAX_MS() = %d, want 16000", cfg.BACKOFF_MAX_MS())
		}
		if cfg.STREAK_THRESHOLD() != 5 {
			t.Errorf("STREAK_THRESHOLD() = %d, want 5", cfg.STREAK_THRESHOLD())
		}
	})

	t.Run("float overrides", func(t *testing.T) {
		setEnv(t, "GATEWAY_SPEEDUP_FACTOR", "0.75")
		setEnv(t, "GATEWAY_SLOWDOWN_FACTOR", "2.0")
		setEnv(t, "GATEWAY_STREAK_SPEEDUP", "0.60")

		cfg := Default()
		cfg.LoadEnvOverrides()

		if cfg.SPEEDUP_FACTOR() != 0.75 {
			t.Errorf("SPEEDUP_FACTOR() = %f, want 0.75", cfg.SPEEDUP_FACTOR())
		}
		if cfg.SLOWDOWN_FACTOR() != 2.0 {
			t.Errorf("SLOWDOWN_FACTOR() = %f, want 2.0", cfg.SLOWDOWN_FACTOR())
		}
		if cfg.STREAK_SPEEDUP() != 0.60 {
			t.Errorf("STREAK_SPEEDUP() = %f, want 0.60", cfg.STREAK_SPEEDUP())
		}
	})

	t.Run("string overrides", func(t *testing.T) {
		setEnv(t, "GATEWAY_DB_PATH", "/tmp/test.db")
		setEnv(t, "PROJECT_ROOT", "/tmp/project")

		cfg := Default()
		cfg.LoadEnvOverrides()

		if cfg.DB_PATH() != "/tmp/test.db" {
			t.Errorf("DB_PATH() = %q, want /tmp/test.db", cfg.DB_PATH())
		}
		if cfg.PROJECT_ROOT() != "/tmp/project" {
			t.Errorf("PROJECT_ROOT() = %q, want /tmp/project", cfg.PROJECT_ROOT())
		}
	})

	t.Run("provider order", func(t *testing.T) {
		setEnv(t, "GATEWAY_PROVIDER_ORDER", "claude, gemini, codex")

		cfg := Default()
		cfg.LoadEnvOverrides()

		order := cfg.PROVIDER_ORDER()
		if len(order) != 3 || order[0] != "claude" || order[1] != "gemini" || order[2] != "codex" {
			t.Errorf("PROVIDER_ORDER() = %v, want [claude gemini codex]", order)
		}
	})

	t.Run("defaults preserved when env unset", func(t *testing.T) {
		cfg := Default()
		cfg.LoadEnvOverrides()

		if cfg.TIMEOUT_SECONDS() != 420 {
			t.Errorf("TIMEOUT_SECONDS() = %d, want 420 (default)", cfg.TIMEOUT_SECONDS())
		}
		if cfg.MAX_RETRIES() != 3 {
			t.Errorf("MAX_RETRIES() = %d, want 3 (default)", cfg.MAX_RETRIES())
		}
	})
}
