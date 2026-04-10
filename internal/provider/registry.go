package provider

import "fmt"

// Registry maps provider names to Provider instances.
// Only providers whose CLI is available are registered.
type Registry struct {
	providers     map[string]Provider
	aliasProvider map[string]string // provider-prefixed alias → provider name
}

// NewRegistry creates a Registry, filtering to only available providers.
// aliasMap maps provider-prefixed aliases (e.g., "gemini-fast") to the provider name.
func NewRegistry(aliasMap map[string]string, providers ...Provider) *Registry {
	r := &Registry{
		providers:     make(map[string]Provider),
		aliasProvider: aliasMap,
	}
	for _, p := range providers {
		if p.Available() {
			r.providers[p.Name()] = p
		}
	}
	return r
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// ForAlias returns the provider that owns a given provider-prefixed alias.
func (r *Registry) ForAlias(alias string) (Provider, bool) {
	provName, ok := r.aliasProvider[alias]
	if !ok {
		return nil, false
	}
	return r.Get(provName)
}

// Available returns the names of all available providers.
func (r *Registry) Available() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Count returns the number of available providers.
func (r *Registry) Count() int {
	return len(r.providers)
}

// MustHaveProviders panics if no providers are available.
func (r *Registry) MustHaveProviders() {
	if len(r.providers) == 0 {
		panic(fmt.Sprintf("no CLI providers available — install at least one of: gemini, codex, claude"))
	}
}
