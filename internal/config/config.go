package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all gateway tuning parameters.
// Scalar fields support GATEWAY_* environment variable overrides via LoadEnvOverrides().
// Use ALL_CAPS accessor methods to read env-overridable values — the naming convention
// signals that the value may have been set by an environment variable.
type Config struct {
	// ── Runtime-computed fields (populated by Merge, NOT env-overridable) ──

	// Tiers maps public tier names (e.g., "fast") to provider-prefixed aliases.
	Tiers map[string][]string

	// Models is the merged alias→model map with provider-prefixed keys.
	Models map[string]string

	// ModelBuckets groups models into substitution tiers for automatic load-balancing.
	ModelBuckets [][]string

	// Per-alias pacing maps (populated by Merge from providers).
	MaxConcurrent map[string]int
	MaxQueue      map[string]int
	InitialGapMs  map[string]int
	FloorMs       map[string]int

	// ── Env-overridable scalar fields ──
	// Access these via ALL_CAPS methods, which signal env-override awareness.

	QueuePollInterval time.Duration // GATEWAY_QUEUE_POLL_SECONDS
	CeilingMs         int          // GATEWAY_CEILING_MS
	JitterMs          [2]int       // not env-overridable (array)
	SpeedupFactor     float64      // GATEWAY_SPEEDUP_FACTOR
	SlowdownFactor    float64      // GATEWAY_SLOWDOWN_FACTOR
	BackoffInitialMs  int          // GATEWAY_BACKOFF_INITIAL_MS
	BackoffMaxMs      int          // GATEWAY_BACKOFF_MAX_MS
	StreakThreshold    int          // GATEWAY_STREAK_THRESHOLD
	StreakSpeedup      float64      // GATEWAY_STREAK_SPEEDUP
	MaxRetries        int          // GATEWAY_MAX_RETRIES
	TimeoutSeconds    int          // GATEWAY_TIMEOUT_SECONDS
	CleanupDays       int          // GATEWAY_CLEANUP_DAYS
	DBPath            string       // GATEWAY_DB_PATH
	ProjectRoot       string       // PROJECT_ROOT
	SystemPrefix      string       // GATEWAY_SYSTEM_PREFIX
	ProviderOrder     []string     // GATEWAY_PROVIDER_ORDER
}

// Default returns a Config with gateway-level tuning defaults.
// Call LoadEnvOverrides() after to apply GATEWAY_* env var overrides.
func Default() *Config {
	return &Config{
		// Runtime-computed — populated by Merge().
		Models:       map[string]string{},
		Tiers:        map[string][]string{},
		ModelBuckets: [][]string{},

		MaxConcurrent: map[string]int{},
		MaxQueue:      map[string]int{},
		InitialGapMs:  map[string]int{},
		FloorMs:       map[string]int{},

		// Defaults — each can be overridden by a GATEWAY_* env var.
		QueuePollInterval: 3 * time.Second,
		CeilingMs:         10000,
		JitterMs:          [2]int{0, 250},
		SpeedupFactor:     0.90,
		SlowdownFactor:    1.3,
		BackoffInitialMs:  1500,
		BackoffMaxMs:      8000,
		StreakThreshold:    3,
		StreakSpeedup:      0.85,
		MaxRetries:        3,
		TimeoutSeconds:    420,
		CleanupDays:       7,
		DBPath:            "data/mcp-cli-gateway.sqlite",
		SystemPrefix: "You are a code generation subagent dispatched by an orchestrating agent. " +
			"The orchestrator will review your work via `git diff` after you finish.\n\n" +
			"Tool usage:\n" +
			"- Read source code, existing tests, interfaces, and project conventions before writing.\n" +
			"- Write files directly using your file-writing tools. Create or modify files as needed.\n" +
			"- CRITICAL: Only write files within the current working directory. Do NOT write outside the codebase.\n\n" +
			"Output format:\n" +
			"- Write code directly to the target file paths.\n" +
			"- Include all necessary imports, namespace declarations, and use statements.\n" +
			"- Do NOT add explanations or commentary — just write the files.\n" +
			"---\n",
	}
}

// LoadEnvOverrides reads all GATEWAY_* env vars and overrides defaults.
// Call after Default() and before Merge(). CLI flag overrides should be applied after this.
func (c *Config) LoadEnvOverrides() {
	envStr("GATEWAY_DB_PATH", &c.DBPath)
	envStr("GATEWAY_SYSTEM_PREFIX", &c.SystemPrefix)
	envStr("PROJECT_ROOT", &c.ProjectRoot)
	envInt("GATEWAY_TIMEOUT_SECONDS", &c.TimeoutSeconds)
	envInt("GATEWAY_MAX_RETRIES", &c.MaxRetries)
	envInt("GATEWAY_CLEANUP_DAYS", &c.CleanupDays)
	envInt("GATEWAY_CEILING_MS", &c.CeilingMs)
	envInt("GATEWAY_BACKOFF_INITIAL_MS", &c.BackoffInitialMs)
	envInt("GATEWAY_BACKOFF_MAX_MS", &c.BackoffMaxMs)
	envInt("GATEWAY_STREAK_THRESHOLD", &c.StreakThreshold)
	envFloat("GATEWAY_SPEEDUP_FACTOR", &c.SpeedupFactor)
	envFloat("GATEWAY_SLOWDOWN_FACTOR", &c.SlowdownFactor)
	envFloat("GATEWAY_STREAK_SPEEDUP", &c.StreakSpeedup)

	envDuration("GATEWAY_QUEUE_POLL_SECONDS", &c.QueuePollInterval)
	envStringList("GATEWAY_PROVIDER_ORDER", &c.ProviderOrder)
}

// ── ALL_CAPS accessors — signal that the value may come from an env var ──

func (c *Config) DB_PATH() string                       { return c.DBPath }
func (c *Config) PROJECT_ROOT() string                   { return c.ProjectRoot }
func (c *Config) SYSTEM_PREFIX() string                  { return c.SystemPrefix }
func (c *Config) TIMEOUT_SECONDS() int                   { return c.TimeoutSeconds }
func (c *Config) MAX_RETRIES() int                       { return c.MaxRetries }
func (c *Config) CLEANUP_DAYS() int                      { return c.CleanupDays }
func (c *Config) CEILING_MS() int                        { return c.CeilingMs }
func (c *Config) BACKOFF_INITIAL_MS() int                { return c.BackoffInitialMs }
func (c *Config) BACKOFF_MAX_MS() int                    { return c.BackoffMaxMs }
func (c *Config) STREAK_THRESHOLD() int                  { return c.StreakThreshold }
func (c *Config) SPEEDUP_FACTOR() float64                { return c.SpeedupFactor }
func (c *Config) SLOWDOWN_FACTOR() float64               { return c.SlowdownFactor }
func (c *Config) STREAK_SPEEDUP() float64                { return c.StreakSpeedup }
func (c *Config) QUEUE_POLL_INTERVAL() time.Duration     { return c.QueuePollInterval }
func (c *Config) PROVIDER_ORDER() []string               { return c.ProviderOrder }

// FloorForAlias returns the floor_ms for a provider-prefixed alias, defaulting to 800.
func (c *Config) FloorForAlias(alias string) int {
	if v, ok := c.FloorMs[alias]; ok {
		return v
	}
	return 800
}

// InitialGapForAlias returns the initial_gap_ms for a provider-prefixed alias, defaulting to 1200.
func (c *Config) InitialGapForAlias(alias string) int {
	if v, ok := c.InitialGapMs[alias]; ok {
		return v
	}
	return 1200
}

// DefaultTierAlias returns the default provider-prefixed alias for a public tier.
func (c *Config) DefaultTierAlias(tier string) string {
	aliases := c.Tiers[tier]
	if len(aliases) > 0 {
		return aliases[0]
	}
	return ""
}

// ── Env parsing helpers ──

func envStr(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func envInt(key string, dst *int) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func envFloat(key string, dst *float64) {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*dst = f
		}
	}
}

func envDuration(key string, dst *time.Duration) {
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			*dst = time.Duration(secs) * time.Second
		}
	}
}

func envStringList(key string, dst *[]string) {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		*dst = parts
	}
}
