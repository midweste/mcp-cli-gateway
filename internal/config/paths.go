// Package config — paths.go centralizes ALL filesystem paths for the gateway.
// Data paths derive from the executable location (global tool).
// ProjectRoot is resolved separately for dispatching agents.
package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Paths holds all derived filesystem paths for the gateway.
type Paths struct {
	ProjectRoot string // git root or CWD — where dispatched agents work
	GatewayDir  string // directory containing the gateway executable
	DataDir     string // {GatewayDir}/data
	DBFile      string // {DataDir}/mcp-cli-gateway.sqlite
	EnvFile     string // {GatewayDir}/.env
	EnvExample  string // {GatewayDir}/.env.example
}

// NewPaths derives data paths from the executable location and project root separately.
// Data is stored next to the binary (global tool), not in the project directory.
func NewPaths(projectRoot string) *Paths {
	gatewayDir := resolveGatewayDir()
	dataDir := filepath.Join(gatewayDir, "data")
	return &Paths{
		ProjectRoot: projectRoot,
		GatewayDir:  gatewayDir,
		DataDir:     dataDir,
		DBFile:      filepath.Join(dataDir, "mcp-cli-gateway.sqlite"),
		EnvFile:     filepath.Join(gatewayDir, ".env"),
		EnvExample:  filepath.Join(gatewayDir, ".env.example"),
	}
}

// EnsureDirs creates the data directory if it doesn't exist.
func (p *Paths) EnsureDirs() error {
	return os.MkdirAll(p.DataDir, 0o755)
}

// resolveGatewayDir returns the directory containing the gateway executable.
// Falls back to CWD if executable path cannot be determined.
func resolveGatewayDir() string {
	if exe, err := os.Executable(); err == nil {
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			return filepath.Dir(resolved)
		}
		return filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()
	return cwd
}

// ResolveProjectRoot determines the project root directory.
// Priority: PROJECT_ROOT env var → git root from CWD → git root from executable dir → CWD.
func ResolveProjectRoot() string {
	// 1. Explicit env var
	if root := os.Getenv("PROJECT_ROOT"); root != "" {
		return root
	}

	// 2. Git root from CWD (works when user runs from project dir)
	if root, err := gitRootFrom(""); err == nil {
		return root
	}

	// 3. Git root from executable directory (works when Antigravity launches
	//    the binary without setting CWD — the binary lives inside the project)
	if exe, err := os.Executable(); err == nil {
		if root, err := gitRootFrom(filepath.Dir(exe)); err == nil {
			return root
		}
	}

	// 4. CWD fallback
	cwd, _ := os.Getwd()
	return cwd
}

// gitRootFrom runs git rev-parse --show-toplevel from the given directory.
// If dir is empty, uses CWD.
func gitRootFrom(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
