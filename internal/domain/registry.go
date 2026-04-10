package domain

import (
	"fmt"
	"sort"
)

// ModelRegistry provides DRY model alias ↔ full name lookups with provider tracking.
// Aliases are provider-prefixed (e.g., "gemini-fast" → "gemini-2.5-flash").
type ModelRegistry struct {
	aliasToModel    map[string]string
	modelToAlias    map[string]string
	aliasToProvider map[string]string
}

// NewModelRegistry creates a registry from an alias→model map and alias→provider map.
func NewModelRegistry(models map[string]string, providers map[string]string) *ModelRegistry {
	r := &ModelRegistry{
		aliasToModel:    make(map[string]string, len(models)),
		modelToAlias:    make(map[string]string, len(models)),
		aliasToProvider: make(map[string]string, len(providers)),
	}
	for alias, model := range models {
		r.aliasToModel[alias] = model
		r.modelToAlias[model] = alias
	}
	for alias, prov := range providers {
		r.aliasToProvider[alias] = prov
	}
	return r
}

// Resolve converts a provider-prefixed alias to the full model string.
// Returns an error if the alias is unknown.
func (r *ModelRegistry) Resolve(alias string) (string, error) {
	model, ok := r.aliasToModel[alias]
	if !ok {
		return "", fmt.Errorf("unknown model alias %q, valid: %v", alias, r.Aliases())
	}
	return model, nil
}

// MustResolve converts a provider-prefixed alias to the full model string, panicking on error.
// Use only during initialization where failure is unrecoverable.
func (r *ModelRegistry) MustResolve(alias string) string {
	model, err := r.Resolve(alias)
	if err != nil {
		panic(err)
	}
	return model
}

// AliasFor returns the provider-prefixed alias for a full model name,
// or the model name itself if unknown.
func (r *ModelRegistry) AliasFor(fullName string) string {
	if alias, ok := r.modelToAlias[fullName]; ok {
		return alias
	}
	return fullName
}

// ProviderFor returns the provider name that owns a provider-prefixed alias.
// Returns "" if the alias is unknown.
func (r *ModelRegistry) ProviderFor(alias string) string {
	return r.aliasToProvider[alias]
}

// ForEach iterates over all alias→model pairs.
func (r *ModelRegistry) ForEach(fn func(alias, model string)) {
	for alias, model := range r.aliasToModel {
		fn(alias, model)
	}
}

// Aliases returns all known aliases.
func (r *ModelRegistry) Aliases() []string {
	aliases := make([]string, 0, len(r.aliasToModel))
	for alias := range r.aliasToModel {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

// HasAlias returns true if the alias exists.
func (r *ModelRegistry) HasAlias(alias string) bool {
	_, ok := r.aliasToModel[alias]
	return ok
}
