package server

import (
	"strings"
	"testing"
)

func TestArgStr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		key  string
		want string
	}{
		{"Present", map[string]any{"model": "fast"}, "model", "fast"},
		{"Missing", map[string]any{"model": "fast"}, "label", ""},
		{"WrongType", map[string]any{"model": 42}, "model", ""},
		{"EmptyMap", map[string]any{}, "anything", ""},
		{"NilValue", map[string]any{"model": nil}, "model", ""},
		{"EmptyString", map[string]any{"model": ""}, "model", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := argStr(tt.args, tt.key)
			if got != tt.want {
				t.Errorf("argStr(%v, %q) = %q, want %q", tt.args, tt.key, got, tt.want)
			}
		})
	}
}

func TestBoolPtr(t *testing.T) {
	t.Parallel()

	trueVal := boolPtr(true)
	if trueVal == nil || *trueVal != true {
		t.Errorf("boolPtr(true) = %v, want *true", trueVal)
	}

	falseVal := boolPtr(false)
	if falseVal == nil || *falseVal != false {
		t.Errorf("boolPtr(false) = %v, want *false", falseVal)
	}
}

func TestToJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantSub string // substring to check for
	}{
		{"Struct", struct{ Name string }{"test"}, `"Name": "test"`},
		{"Map", map[string]int{"count": 42}, `"count": 42`},
		{"Nil", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toJSON(tt.input)
			if got == "" {
				t.Error("toJSON returned empty string")
			}
			if len(got) > 0 && !strings.Contains(got, tt.wantSub) {
				t.Errorf("toJSON(%v) = %q, want substring %q", tt.input, got, tt.wantSub)
			}
		})
	}
}
