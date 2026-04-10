package domain

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	t.Parallel()

	models := map[string]string{
		"fast":  "gemini-3-flash-preview",
		"think": "gemini-2.5-pro",
		"deep":  "gemini-3.1-pro-preview",
	}
	reg := NewModelRegistry(models, map[string]string{})

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ResolveValidAlias",
			fn: func(t *testing.T) {
				model, err := reg.Resolve("fast")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if model != "gemini-3-flash-preview" {
					t.Errorf("got %q, want %q", model, "gemini-3-flash-preview")
				}
			},
		},
		{
			name: "ResolveAllAliases",
			fn: func(t *testing.T) {
				for alias, expected := range models {
					model, err := reg.Resolve(alias)
					if err != nil {
						t.Errorf("Resolve(%q) error: %v", alias, err)
					}
					if model != expected {
						t.Errorf("Resolve(%q) = %q, want %q", alias, model, expected)
					}
				}
			},
		},
		{
			name: "ResolveUnknownAlias",
			fn: func(t *testing.T) {
				_, err := reg.Resolve("nonexistent")
				if err == nil {
					t.Error("expected error for unknown alias, got nil")
				}
			},
		},
		{
			name: "AliasForRoundTrip",
			fn: func(t *testing.T) {
				for alias, model := range models {
					got := reg.AliasFor(model)
					if got != alias {
						t.Errorf("AliasFor(%q) = %q, want %q", model, got, alias)
					}
					resolved, _ := reg.Resolve(got)
					if resolved != model {
						t.Errorf("round-trip failed: Resolve(AliasFor(%q)) = %q", model, resolved)
					}
				}
			},
		},
		{
			name: "HasAlias",
			fn: func(t *testing.T) {
				if !reg.HasAlias("fast") {
					t.Error("HasAlias('fast') = false, want true")
				}
				if reg.HasAlias("nonexistent") {
					t.Error("HasAlias('nonexistent') = true, want false")
				}
			},
		},
		{
			name: "MustResolve",
			fn: func(t *testing.T) {
				got := reg.MustResolve("fast")
				if got != "gemini-3-flash-preview" {
					t.Errorf("MustResolve('fast') = %q, want 'gemini-3-flash-preview'", got)
				}
			},
		},
		{
			name: "MustResolve_Panics",
			fn: func(t *testing.T) {
				defer func() {
					if r := recover(); r == nil {
						t.Error("MustResolve should panic for unknown alias")
					}
				}()
				reg.MustResolve("nonexistent")
			},
		},
		{
			name: "ForEach",
			fn: func(t *testing.T) {
				count := 0
				reg.ForEach(func(alias, model string) {
					count++
					if alias == "" || model == "" {
						t.Error("ForEach yielded empty alias or model")
					}
				})
				if count != len(models) {
					t.Errorf("ForEach visited %d, want %d", count, len(models))
				}
			},
		},
		{
			name: "AliasForUnknown",
			fn: func(t *testing.T) {
				got := reg.AliasFor("unknown-model-name")
				if got != "unknown-model-name" {
					t.Errorf("AliasFor unknown=%q, want same string", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.fn(t)
		})
	}
}
