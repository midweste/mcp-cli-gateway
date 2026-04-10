package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGatewayDir(t *testing.T) {
	// resolveGatewayDir should return a non-empty absolute path
	dir := resolveGatewayDir()
	if dir == "" {
		t.Fatal("resolveGatewayDir() returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("resolveGatewayDir()=%q, want absolute path", dir)
	}
}

func TestGitRootFrom_ValidDir(t *testing.T) {
	// Running from within the gateway repo — should succeed
	root, err := gitRootFrom("")
	if err != nil {
		t.Fatalf("gitRootFrom(''): %v", err)
	}
	if root == "" {
		t.Error("gitRootFrom('') returned empty string")
	}
	if !filepath.IsAbs(root) {
		t.Errorf("gitRootFrom('')=%q, want absolute path", root)
	}
}

func TestGitRootFrom_WithDir(t *testing.T) {
	// Running from a known git directory
	cwd, _ := os.Getwd()
	root, err := gitRootFrom(cwd)
	if err != nil {
		t.Fatalf("gitRootFrom(%q): %v", cwd, err)
	}
	if root == "" {
		t.Error("gitRootFrom returned empty string for CWD")
	}
}

func TestGitRootFrom_NonGitDir(t *testing.T) {
	// /tmp is not a git repo
	_, err := gitRootFrom("/tmp")
	if err == nil {
		t.Error("expected error for non-git directory /tmp")
	}
}

func TestResolveProjectRoot_ExeGitFallback(t *testing.T) {
	// Without PROJECT_ROOT and from a non-git dir, should fall back to
	// git root from executable dir or CWD
	t.Setenv("PROJECT_ROOT", "")

	// Temporarily change to /tmp (non-git dir) to test the exe-based fallback
	origDir, _ := os.Getwd()
	if err := os.Chdir("/tmp"); err != nil {
		t.Skip("cannot chdir to /tmp")
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	root := ResolveProjectRoot()
	// Should still find something — either git root from exe dir or CWD (/tmp)
	if root == "" {
		t.Error("ResolveProjectRoot() returned empty string")
	}
}

func TestNewPaths_AllFieldsPopulated(t *testing.T) {
	t.Parallel()
	p := NewPaths("/test/root")

	if p.GatewayDir == "" {
		t.Error("GatewayDir should not be empty")
	}
	if p.DataDir == "" {
		t.Error("DataDir should not be empty")
	}
	if p.DBFile == "" {
		t.Error("DBFile should not be empty")
	}
	if p.EnvFile == "" {
		t.Error("EnvFile should not be empty")
	}
	if p.EnvExample == "" {
		t.Error("EnvExample should not be empty")
	}
	// EnvExample should be in GatewayDir
	if filepath.Dir(p.EnvExample) != p.GatewayDir {
		t.Errorf("EnvExample=%q not in GatewayDir=%q", p.EnvExample, p.GatewayDir)
	}
}
