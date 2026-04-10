package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// mockRootsProvider implements RootsProvider for testing.
type mockRootsProvider struct {
	roots []mcp.Root
	err   error
}

func (m *mockRootsProvider) RequestRoots(_ context.Context, _ mcp.ListRootsRequest) (*mcp.ListRootsResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &mcp.ListRootsResult{Roots: m.roots}, nil
}

func TestValidateCwd_UnderRoot(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{
			{URI: "file:///home/eric/projects/myapp"},
		},
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/projects/myapp/src")
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateCwd_ExactRoot(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{
			{URI: "file:///home/eric/projects/myapp"},
		},
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/projects/myapp")
	if err != nil {
		t.Errorf("expected nil for exact root match, got %v", err)
	}
}

func TestValidateCwd_OutsideRoot(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{
			{URI: "file:///home/eric/projects/myapp"},
		},
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/other-project")
	if err == nil {
		t.Error("expected error for cwd outside root, got nil")
	}
}

func TestValidateCwd_PrefixAttack(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{
			{URI: "file:///home/eric/projects/app"},
		},
	}

	// "/home/eric/projects/app-evil" should NOT match "/home/eric/projects/app"
	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/projects/app-evil")
	if err == nil {
		t.Error("expected error for prefix-attack path, got nil")
	}
}

func TestValidateCwd_MultipleRoots(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{
			{URI: "file:///home/eric/projects/frontend"},
			{URI: "file:///home/eric/projects/backend"},
		},
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/projects/backend/src")
	if err != nil {
		t.Errorf("expected nil for cwd under second root, got %v", err)
	}
}

func TestValidateCwd_RootsNotSupported(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		err: fmt.Errorf("session does not support roots"),
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/projects/myapp")
	if err == nil {
		t.Error("expected error when roots not supported, got nil")
	}
}

func TestValidateCwd_EmptyRoots(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{},
	}

	// Empty roots = unrestricted (client supports roots but declared none)
	err := validateCwdAgainstRoots(context.Background(), provider, "/home/eric/projects/myapp")
	if err != nil {
		t.Errorf("expected nil for empty roots (unrestricted), got %v", err)
	}
}

func TestValidateCwd_EmptyCwd(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		err: fmt.Errorf("should not be called"),
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "")
	if err != nil {
		t.Errorf("expected nil for empty cwd (skip validation), got %v", err)
	}
}

func TestRootURIToPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{"FileURI", "file:///home/eric/projects", "/home/eric/projects"},
		{"BarePath", "/home/eric/projects", "/home/eric/projects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := rootURIToPath(tt.uri)
			if err != nil {
				t.Fatalf("rootURIToPath(%q): %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("rootURIToPath(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}
