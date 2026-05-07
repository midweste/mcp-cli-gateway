package config

import (
	"fmt"
	"sort"
	"strings"
)

// ProviderDescriptor is the minimal interface that Merge() needs to build
// the runtime config from available providers. Satisfied by provider.Provider.
// Defined here to avoid a circular import between config and provider.
type ProviderDescriptor interface {
	Name() string
	TierModels() map[string][]string
	ModelPacing(model string) PacingConfig
}

// PacingConfig holds pacing parameters for a specific model.
type PacingConfig struct {
	InitialGapMs  int
	FloorMs       int
	MaxConcurrent int
	MaxQueue      int
}

// TierOrder defines the canonical ordering of tiers.
var TierOrder = []string{"lite", "fast", "deep"}

// Merge populates the flat Models/Tiers/ModelBuckets/per-alias config from
// the given providers. Each provider self-describes its models and pacing.
func (c *Config) Merge(providers []ProviderDescriptor) {
	c.Models = make(map[string]string)
	c.Tiers = make(map[string][]string)
	c.InitialGapMs = make(map[string]int)
	c.FloorMs = make(map[string]int)
	c.MaxConcurrent = make(map[string]int)
	c.MaxQueue = make(map[string]int)

	for _, prov := range providers {
		provName := prov.Name()

		for tier, models := range prov.TierModels() {
			for idx, model := range models {
				alias := aliasName(provName, tier, idx)
				c.Models[alias] = model

				pacing := prov.ModelPacing(model)
				c.InitialGapMs[alias] = pacing.InitialGapMs
				c.FloorMs[alias] = pacing.FloorMs
				c.MaxConcurrent[alias] = pacing.MaxConcurrent
				c.MaxQueue[alias] = pacing.MaxQueue
			}
		}
	}

	// Build Tiers — group all provider-prefixed aliases by their tier name.
	tierSet := make(map[string][]string)
	for _, prov := range providers {
		provName := prov.Name()
		for tier, models := range prov.TierModels() {
			for idx := range models {
				alias := aliasName(provName, tier, idx)
				tierSet[tier] = append(tierSet[tier], alias)
			}
		}
	}
	c.Tiers = sortTiersByProviderOrder(tierSet, c.ProviderOrder)

	// Build ModelBuckets — one bucket per tier (1:1 mapping), in canonical order.
	c.ModelBuckets = nil
	for _, tier := range TierOrder {
		if aliases, ok := c.Tiers[tier]; ok && len(aliases) > 0 {
			bucket := make([]string, len(aliases))
			copy(bucket, aliases)
			c.ModelBuckets = append(c.ModelBuckets, bucket)
		}
	}
}

// AliasProviderMap returns a map of provider-prefixed alias → provider name.
func (c *Config) AliasProviderMap(providers []ProviderDescriptor) map[string]string {
	m := make(map[string]string, len(c.Models))
	for alias := range c.Models {
		for _, prov := range providers {
			prefix := prov.Name() + "-"
			if strings.HasPrefix(alias, prefix) {
				m[alias] = prov.Name()
				break
			}
		}
	}
	return m
}

// aliasName generates the provider-prefixed alias for a model at a given index within a tier.
// First model (idx=0): "provider-tier", subsequent: "provider-tier-1", "provider-tier-2", etc.
func aliasName(provName, tier string, idx int) string {
	if idx == 0 {
		return provName + "-" + tier
	}
	return fmt.Sprintf("%s-%s-%d", provName, tier, idx)
}

// sortTiersByProviderOrder sorts aliases within each tier according to the
// preferred provider order. Aliases for earlier providers in the order list
// appear first. Providers not in the order list sort to the end alphabetically.
func sortTiersByProviderOrder(tiers map[string][]string, order []string) map[string][]string {
	if len(order) == 0 {
		for tier, aliases := range tiers {
			sorted := make([]string, len(aliases))
			copy(sorted, aliases)
			sort.Strings(sorted)
			tiers[tier] = sorted
		}
		return tiers
	}

	priority := make(map[string]int, len(order))
	for i, name := range order {
		priority[strings.ToLower(name)] = i
	}

	for tier, aliases := range tiers {
		sorted := make([]string, len(aliases))
		copy(sorted, aliases)
		sort.SliceStable(sorted, func(i, j int) bool {
			pi := providerPriority(sorted[i], priority)
			pj := providerPriority(sorted[j], priority)
			if pi != pj {
				return pi < pj
			}
			return sorted[i] < sorted[j]
		})
		tiers[tier] = sorted
	}
	return tiers
}

// providerPriority extracts the provider name from an alias and returns its priority.
func providerPriority(alias string, priority map[string]int) int {
	idx := strings.Index(alias, "-")
	if idx <= 0 {
		return len(priority)
	}
	provName := alias[:idx]
	if p, ok := priority[strings.ToLower(provName)]; ok {
		return p
	}
	return len(priority)
}
