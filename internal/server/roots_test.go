package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ---------------------------------------------------------------------------
// validateCwdAgainstRoots: root-matching behavior (roots present)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// validateCwdAgainstRoots: empty cwd behavior
// ---------------------------------------------------------------------------

func TestValidateCwd_EmptyCwd_RootsPresent_Skips(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{
			{URI: "file:///home/eric/projects/myapp"},
		},
	}

	// Empty cwd with valid roots → skip validation (gateway fills default later)
	err := validateCwdAgainstRoots(context.Background(), provider, "")
	if err != nil {
		t.Errorf("expected nil for empty cwd with roots present, got %v", err)
	}
}

func TestValidateCwd_EmptyCwd_RootsError_ReturnsActionable(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		err: fmt.Errorf("session does not support roots"),
	}

	// Empty cwd with roots unsupported → must return actionable error
	err := validateCwdAgainstRoots(context.Background(), provider, "")
	if err == nil {
		t.Fatal("expected error for empty cwd + roots unavailable, got nil")
	}
	if !strings.Contains(err.Error(), "MUST provide an explicit 'cwd'") {
		t.Errorf("error should be actionable, got: %v", err)
	}
}

func TestValidateCwd_EmptyCwd_RootsEmpty_ReturnsActionable(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{},
	}

	// Empty cwd with empty roots → must return actionable error
	err := validateCwdAgainstRoots(context.Background(), provider, "")
	if err == nil {
		t.Fatal("expected error for empty cwd + empty roots, got nil")
	}
	if !strings.Contains(err.Error(), "MUST provide an explicit 'cwd'") {
		t.Errorf("error should be actionable, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// validateCwdAgainstRoots: fallthrough to isSafeCwd
// ---------------------------------------------------------------------------

func TestValidateCwd_RootsNotSupported_FallsThrough_SafeDir(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		err: fmt.Errorf("session does not support roots"),
	}

	gwDir := findGitRoot(t)

	err := validateCwdAgainstRoots(context.Background(), provider, gwDir)
	if err != nil {
		t.Errorf("expected nil for roots-unsupported + safe project dir, got %v", err)
	}
}

func TestValidateCwd_RootsNotSupported_RejectsUnsafePath(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		err: fmt.Errorf("session does not support roots"),
	}

	err := validateCwdAgainstRoots(context.Background(), provider, "/tmp")
	if err == nil {
		t.Error("expected error for roots-unsupported + /tmp, got nil")
	}
}

func TestValidateCwd_EmptyRoots_FallsThrough_SafeDir(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{},
	}

	gwDir := findGitRoot(t)

	err := validateCwdAgainstRoots(context.Background(), provider, gwDir)
	if err != nil {
		t.Errorf("expected nil for empty roots + safe project dir, got %v", err)
	}
}

func TestValidateCwd_EmptyRoots_RejectsHomedir(t *testing.T) {
	t.Parallel()
	provider := &mockRootsProvider{
		roots: []mcp.Root{},
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	err = validateCwdAgainstRoots(context.Background(), provider, home)
	if err == nil {
		t.Error("expected error for empty roots + home dir, got nil")
	}
}

// ---------------------------------------------------------------------------
// isDeniedByDepth: structural path depth checks
// ---------------------------------------------------------------------------

func TestIsDeniedByDepth_Root(t *testing.T) {
	t.Parallel()
	denied, msg := isDeniedByDepth("/")
	if !denied {
		t.Error("expected / to be denied")
	}
	if !strings.Contains(msg, "filesystem root") {
		t.Errorf("expected root message, got: %s", msg)
	}
}

func TestIsDeniedByDepth_TopLevelDirs(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/tmp", "/home", "/var", "/etc", "/usr", "/opt", "/bin", "/sbin"} {
		denied, _ := isDeniedByDepth(path)
		if !denied {
			t.Errorf("expected %q to be denied as top-level dir", path)
		}
	}
}

func TestIsDeniedByDepth_HomeUser(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/home/eric", "/home/deploy", "/home/ubuntu"} {
		denied, msg := isDeniedByDepth(path)
		if !denied {
			t.Errorf("expected %q to be denied as user home dir", path)
		}
		if !strings.Contains(msg, "user home directory") {
			t.Errorf("expected home dir message for %q, got: %s", path, msg)
		}
	}
}

func TestIsDeniedByDepth_AllowsProjectDir(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/home/eric/projects", "/home/eric/websites/foo", "/opt/apps/myapp"} {
		denied, _ := isDeniedByDepth(path)
		if denied {
			t.Errorf("expected %q to be allowed", path)
		}
	}
}

func TestIsDeniedByDepth_AllowsNonHomeTwoSegment(t *testing.T) {
	t.Parallel()
	// /opt/myapp is 2 segments but NOT under /home — should be allowed
	denied, _ := isDeniedByDepth("/opt/myapp")
	if denied {
		t.Error("expected /opt/myapp to be allowed (not under /home)")
	}
}

// ---------------------------------------------------------------------------
// isSafeCwd: integration tests
// ---------------------------------------------------------------------------

func TestIsSafeCwd_RejectsRoot(t *testing.T) {
	t.Parallel()
	err := isSafeCwd("/")
	if err == nil {
		t.Error("expected error for /, got nil")
	}
}

func TestIsSafeCwd_RejectsHomedir(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	err = isSafeCwd(home)
	if err == nil {
		t.Errorf("expected error for home dir %q, got nil", home)
	}
}

func TestIsSafeCwd_RejectsTmp(t *testing.T) {
	t.Parallel()
	err := isSafeCwd("/tmp")
	if err == nil {
		t.Error("expected error for /tmp, got nil")
	}
}

func TestIsSafeCwd_AllowsProjectDir(t *testing.T) {
	t.Parallel()
	gwDir := findGitRoot(t)

	err := isSafeCwd(gwDir)
	if err != nil {
		t.Errorf("expected nil for git project dir %q, got %v", gwDir, err)
	}
}

func TestIsSafeCwd_AllowsProjectSubdir(t *testing.T) {
	t.Parallel()
	gwDir := findGitRoot(t)
	subdir := filepath.Join(gwDir, "internal", "server")

	err := isSafeCwd(subdir)
	if err != nil {
		t.Errorf("expected nil for project subdir %q, got %v", subdir, err)
	}
}

func TestIsSafeCwd_RejectsNonGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	err := isSafeCwd(tmpDir)
	if err == nil {
		t.Errorf("expected error for dir with no .git %q, got nil", tmpDir)
	}
}

func TestIsSafeCwd_AllowsInfraDir_WithGit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("create .git: %v", err)
	}

	err := isSafeCwd(tmpDir)
	if err != nil {
		t.Errorf("expected nil for infra dir with .git, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// isSafeCwd: symlink resolution
// ---------------------------------------------------------------------------

func TestIsSafeCwd_ResolvesSymlinks(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	link := filepath.Join(tmpDir, "safe-looking-link")

	if err := os.Symlink("/tmp", link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	err := isSafeCwd(link)
	if err == nil {
		t.Error("expected error for symlink pointing to /tmp, got nil")
	}
}

func TestIsSafeCwd_RejectsUnresolvablePath(t *testing.T) {
	t.Parallel()
	err := isSafeCwd("/nonexistent/path/that/cannot/resolve")
	if err == nil {
		t.Error("expected error for unresolvable path, got nil")
	}
}

// ---------------------------------------------------------------------------
// isSafeCwd: error messages are actionable
// ---------------------------------------------------------------------------

func TestIsSafeCwd_ErrorMessages_ContainGuidance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cwd     string
		wantSub string
	}{
		{"root", "/", "Provide a 'cwd'"},
		{"top-level", "/tmp", "Provide a 'cwd'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := isSafeCwd(tt.cwd)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error should contain guidance %q, got: %v", tt.wantSub, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// hasProjectMarker
// ---------------------------------------------------------------------------

func TestHasProjectMarker_GitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if !hasProjectMarker(tmpDir) {
		t.Error("expected true for dir with .git directory")
	}
}

func TestHasProjectMarker_GitFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitFile := filepath.Join(tmpDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/repo/.git/worktrees/wt1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !hasProjectMarker(tmpDir) {
		t.Error("expected true for dir with .git file (worktree)")
	}
}

func TestHasProjectMarker_ParentDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create 5 levels deep (within the 6-level limit)
	subdir := filepath.Join(tmpDir, "a", "b", "c", "d", "e")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	if !hasProjectMarker(subdir) {
		t.Error("expected true for subdir within 6 levels of git root")
	}
}

func TestHasProjectMarker_ExceedsMaxDepth(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create 7 levels deep (exceeds the 6-level limit)
	subdir := filepath.Join(tmpDir, "a", "b", "c", "d", "e", "f", "g")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	if hasProjectMarker(subdir) {
		t.Error("expected false for subdir beyond 6 levels from git root")
	}
}

func TestHasProjectMarker_NoGit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	if hasProjectMarker(tmpDir) {
		t.Error("expected false for dir with no .git")
	}
}

// ---------------------------------------------------------------------------
// rootURIToPath
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func findGitRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find .git root from test working directory")
		}
		dir = parent
	}
}
