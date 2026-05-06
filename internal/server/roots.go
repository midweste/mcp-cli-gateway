package server

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// RootsProvider abstracts the ability to request roots from the MCP client.
// *server.MCPServer satisfies this interface.
type RootsProvider interface {
	RequestRoots(ctx context.Context, request mcp.ListRootsRequest) (*mcp.ListRootsResult, error)
}

// cwdRequiredMsg is returned when the MCP client does not provide roots
// and the caller did not supply an explicit cwd. It is intentionally verbose
// so that the calling agent can self-correct.
const cwdRequiredMsg = "MCP workspace roots are not available; you MUST provide an explicit 'cwd' " +
	"parameter pointing to a project directory that contains a .git folder. " +
	"Example: cwd=\"/home/user/projects/myapp\". " +
	"Dispatching without a project directory is not allowed."

// validateCwdAgainstRoots checks that cwd falls under one of the MCP client's
// declared workspace roots. When the client does not support roots or declares
// none, it falls through to isSafeCwd as an independent safety layer.
// Returns nil if valid, error if not.
// If cwd is empty AND the client provides roots, validation is skipped
// (the gateway fills a default). If cwd is empty AND roots are unavailable,
// an actionable error is returned so the agent can self-correct.
func validateCwdAgainstRoots(ctx context.Context, provider RootsProvider, cwd string) error {
	if cwd == "" {
		// Check whether roots are available before skipping validation.
		// If roots work, the gateway can safely fill a default cwd later.
		// If roots don't work, we need an explicit cwd from the caller.
		result, err := provider.RequestRoots(ctx, mcp.ListRootsRequest{})
		if err != nil || len(result.Roots) == 0 {
			return fmt.Errorf(cwdRequiredMsg)
		}
		return nil
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("resolve cwd %q: %w", cwd, err)
	}

	result, err := provider.RequestRoots(ctx, mcp.ListRootsRequest{})
	if err != nil {
		// Client doesn't support roots (e.g. Antigravity via mcp-proxy).
		// Fall through to isSafeCwd instead of blocking dispatch entirely.
		return isSafeCwd(absCwd)
	}

	if len(result.Roots) == 0 {
		// Client supports roots but declared none — validate via isSafeCwd
		// instead of treating as unrestricted.
		return isSafeCwd(absCwd)
	}

	for _, root := range result.Roots {
		rootPath, err := rootURIToPath(root.URI)
		if err != nil {
			continue // skip malformed URIs
		}
		absRoot, err := filepath.Abs(rootPath)
		if err != nil {
			continue
		}
		// Ensure prefix match is on a directory boundary
		if strings.HasPrefix(absCwd, absRoot+string(filepath.Separator)) || absCwd == absRoot {
			return nil
		}
	}

	// Build a helpful error listing allowed roots
	var rootPaths []string
	for _, root := range result.Roots {
		if p, err := rootURIToPath(root.URI); err == nil {
			rootPaths = append(rootPaths, p)
		}
	}
	return fmt.Errorf("cwd %q is outside all declared workspace roots %v", absCwd, rootPaths)
}

// isSafeCwd applies multi-layer safety validation to a working directory
// when MCP roots are unavailable. All checks must pass:
//  1. Resolve symlinks to get the real path
//  2. Structural depth: block all root-level paths and user home directories
//  3. Project marker: require .git reachable by walking up from cwd
func isSafeCwd(absCwd string) error {
	// Layer 0: Resolve symlinks to prevent traversal bypasses
	realCwd, err := filepath.EvalSymlinks(absCwd)
	if err != nil {
		return fmt.Errorf("cwd %q: cannot resolve path: %w", absCwd, err)
	}

	// Layer 1: Structural depth — block shallow system paths
	if denied, msg := isDeniedByDepth(realCwd); denied {
		return fmt.Errorf("%s. Provide a 'cwd' pointing to a project directory with a .git folder", msg)
	}

	// Layer 2: Project marker — require .git reachable from cwd
	if !hasProjectMarker(realCwd) {
		return fmt.Errorf("cwd %q has no .git directory within 6 levels up (not a project directory). "+
			"Provide a 'cwd' pointing to a directory inside a git repository", realCwd)
	}

	return nil
}

// isDeniedByDepth rejects paths that are too shallow to be project directories.
// Blocked:
//   - The filesystem root: /
//   - Any top-level directory: /tmp, /home, /var, /etc, /usr, /opt, etc.
//   - Any direct child of /home (user home dirs): /home/eric, /home/deploy, etc.
func isDeniedByDepth(absCwd string) (bool, string) {
	cleaned := filepath.Clean(absCwd)
	if cleaned == "/" {
		return true, "cwd \"/\" is the filesystem root — dispatch is not allowed here"
	}

	// Split path into segments: "/home/eric/projects" → ["home", "eric", "projects"]
	parts := strings.Split(strings.TrimPrefix(cleaned, "/"), "/")

	// Depth 1 = top-level dir (e.g., /tmp, /home, /var)
	if len(parts) <= 1 {
		return true, fmt.Sprintf("cwd %q is a top-level system directory — dispatch is not allowed here", cleaned)
	}

	// Depth 2 under /home = user home dir (e.g., /home/eric)
	if parts[0] == "home" && len(parts) == 2 {
		return true, fmt.Sprintf("cwd %q is a user home directory — dispatch requires a project subdirectory (e.g., %s/projects/myapp)", cleaned, cleaned)
	}

	return false, ""
}

// hasProjectMarker walks from dir up to 6 levels looking for a .git entry
// (directory or file — git worktrees use a .git file).
func hasProjectMarker(dir string) bool {
	const maxDepth = 6
	for i := range maxDepth {
		_ = i
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return false
}

// rootURIToPath converts a file:// URI to a filesystem path.
func rootURIToPath(uri string) (string, error) {
	if strings.HasPrefix(uri, "file://") {
		parsed, err := url.Parse(uri)
		if err != nil {
			return "", err
		}
		return parsed.Path, nil
	}
	// Treat bare paths as-is (shouldn't happen per MCP spec, but be defensive)
	return uri, nil
}

