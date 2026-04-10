package config

import (
	"reflect"
	"sort"
	"testing"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	sortStrings := func(s []string) {
		sort.Strings(s)
	}

	tests := []struct {
		name      string
		providers []ProviderDescriptor
		check     func(t *testing.T, cfg *Config)
	}{
		{
			name:      "1. Merge with single provider 'gemini'",
			providers: []ProviderDescriptor{newGeminiTestProvider()},
			check: func(t *testing.T, cfg *Config) {
				// Gemini has 4 models: flash-lite, flash, 2.5-pro, 3.1-pro-preview
				if len(cfg.Models) != 4 {
					t.Errorf("Models count = %d, want 4; Models=%v", len(cfg.Models), cfg.Models)
				}

				// Verify 3 tiers exist
				if len(cfg.Tiers) != 3 {
					t.Errorf("Tiers count = %d, want 3", len(cfg.Tiers))
				}

				// Deep tier should have 2 aliases (gemini-deep, gemini-deep-1)
				if len(cfg.Tiers["deep"]) != 2 {
					t.Errorf("Tiers[deep] count = %d, want 2; aliases=%v", len(cfg.Tiers["deep"]), cfg.Tiers["deep"])
				}

				// Verify 3 buckets (lite, fast, deep)
				if len(cfg.ModelBuckets) != 3 {
					t.Errorf("ModelBuckets count = %d, want 3", len(cfg.ModelBuckets))
				}
			},
		},
		{
			name:      "2. Merge with multiple providers",
			providers: []ProviderDescriptor{newGeminiTestProvider(), newClaudeTestProvider()},
			check: func(t *testing.T, cfg *Config) {
				// Gemini: 4 models + Claude: 3 models = 7 total
				if len(cfg.Models) != 7 {
					t.Errorf("Models count = %d, want 7; Models=%v", len(cfg.Models), cfg.Models)
				}

				// Fast tier should have gemini-fast and claude-fast
				fastAliases := cfg.Tiers["fast"]
				sortStrings(fastAliases)
				wantFast := []string{"claude-fast", "gemini-fast"}
				if !reflect.DeepEqual(fastAliases, wantFast) {
					t.Errorf("Tiers[fast] = %v, want %v", fastAliases, wantFast)
				}
			},
		},
		{
			name:      "3. Merge with no providers",
			providers: []ProviderDescriptor{},
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Models) != 0 {
					t.Errorf("Models not empty: %v", cfg.Models)
				}
				if len(cfg.Tiers) != 0 {
					t.Errorf("Tiers not empty: %v", cfg.Tiers)
				}
				if len(cfg.ModelBuckets) != 0 {
					t.Errorf("ModelBuckets not empty: %v", cfg.ModelBuckets)
				}
			},
		},
		{
			name:      "4. AliasProviderMap returns correct provider for each alias",
			providers: []ProviderDescriptor{newGeminiTestProvider(), newClaudeTestProvider()},
			check: func(t *testing.T, cfg *Config) {
				provs := []ProviderDescriptor{newGeminiTestProvider(), newClaudeTestProvider()}
				aliasMap := cfg.AliasProviderMap(provs)

				if aliasMap["gemini-fast"] != "gemini" {
					t.Errorf("AliasProviderMap[gemini-fast] = %q, want gemini", aliasMap["gemini-fast"])
				}
				if aliasMap["claude-deep"] != "claude" {
					t.Errorf("AliasProviderMap[claude-deep] = %q, want claude", aliasMap["claude-deep"])
				}
				// Multi-model alias
				if aliasMap["gemini-deep-1"] != "gemini" {
					t.Errorf("AliasProviderMap[gemini-deep-1] = %q, want gemini", aliasMap["gemini-deep-1"])
				}
			},
		},
		{
			name:      "5. Pacing from provider is applied to aliases",
			providers: []ProviderDescriptor{newGeminiTestProvider()},
			check: func(t *testing.T, cfg *Config) {
				// gemini-fast should get flash pacing
				if cfg.InitialGapMs["gemini-fast"] != 2000 {
					t.Errorf("InitialGapMs[gemini-fast] = %d, want 2000", cfg.InitialGapMs["gemini-fast"])
				}
				// gemini-deep should get pro pacing
				if cfg.InitialGapMs["gemini-deep"] != 3000 {
					t.Errorf("InitialGapMs[gemini-deep] = %d, want 3000", cfg.InitialGapMs["gemini-deep"])
				}
				// gemini-lite should get flash-lite pacing
				if cfg.InitialGapMs["gemini-lite"] != 1500 {
					t.Errorf("InitialGapMs[gemini-lite] = %d, want 1500", cfg.InitialGapMs["gemini-lite"])
				}
			},
		},
		{
			name:      "6. Claude pacing overrides work",
			providers: []ProviderDescriptor{newClaudeTestProvider()},
			check: func(t *testing.T, cfg *Config) {
				// claude-deep (opus) should get opus pacing
				if cfg.InitialGapMs["claude-deep"] != 2500 {
					t.Errorf("InitialGapMs[claude-deep] = %d, want 2500", cfg.InitialGapMs["claude-deep"])
				}
				// claude-fast (sonnet) should get default claude pacing
				if cfg.InitialGapMs["claude-fast"] != 1500 {
					t.Errorf("InitialGapMs[claude-fast] = %d, want 1500", cfg.InitialGapMs["claude-fast"])
				}
			},
		},
		{
			name:      "7. Multi-model deep bucket has all aliases",
			providers: []ProviderDescriptor{newGeminiTestProvider(), newClaudeTestProvider()},
			check: func(t *testing.T, cfg *Config) {
				// Find the deep bucket
				var deepBucket []string
				for _, bucket := range cfg.ModelBuckets {
					for _, alias := range bucket {
						if alias == "gemini-deep" || alias == "claude-deep" {
							deepBucket = bucket
							break
						}
					}
					if deepBucket != nil {
						break
					}
				}
				if deepBucket == nil {
					t.Fatal("deep bucket not found")
				}
				sortStrings(deepBucket)
				// Should have: claude-deep, gemini-deep, gemini-deep-1
				want := []string{"claude-deep", "gemini-deep", "gemini-deep-1"}
				if !reflect.DeepEqual(deepBucket, want) {
					t.Errorf("deep bucket = %v, want %v", deepBucket, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Default()
			cfg.Merge(tt.providers)
			tt.check(t, cfg)
		})
	}
}

func TestProviderOrder(t *testing.T) {
	t.Parallel()

	provs := []ProviderDescriptor{newGeminiTestProvider(), newClaudeTestProvider()}

	tests := []struct {
		name      string
		order     []string
		tier      string
		wantFirst string // first alias in the tier should start with this provider
	}{
		{
			name:      "gemini first",
			order:     []string{"gemini", "claude"},
			tier:      "fast",
			wantFirst: "gemini-fast",
		},
		{
			name:      "claude first",
			order:     []string{"claude", "gemini"},
			tier:      "fast",
			wantFirst: "claude-fast",
		},
		{
			name:      "default alphabetical when no order",
			order:     nil,
			tier:      "fast",
			wantFirst: "claude-fast", // c < g alphabetically
		},
		{
			name:      "partial order — unlisted providers sort to end",
			order:     []string{"gemini"},
			tier:      "deep",
			wantFirst: "gemini-deep",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Default()
			cfg.ProviderOrder = tt.order
			cfg.Merge(provs)

			aliases := cfg.Tiers[tt.tier]
			if len(aliases) == 0 {
				t.Fatalf("no aliases for tier %q", tt.tier)
			}
			if aliases[0] != tt.wantFirst {
				t.Errorf("Tiers[%q][0] = %q, want %q; full: %v",
					tt.tier, aliases[0], tt.wantFirst, aliases)
			}
		})
	}
}
