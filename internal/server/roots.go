package server

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// RootsProvider abstracts the ability to request roots from the MCP client.
// *server.MCPServer satisfies this interface.
type RootsProvider interface {
	RequestRoots(ctx context.Context, request mcp.ListRootsRequest) (*mcp.ListRootsResult, error)
}

// validateCwdAgainstRoots checks that cwd falls under one of the MCP client's
// declared workspace roots. Returns nil if valid, error if not.
// If cwd is empty, validation is skipped (the gateway fills a default).
func validateCwdAgainstRoots(ctx context.Context, provider RootsProvider, cwd string) error {
	if cwd == "" {
		return nil
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("resolve cwd %q: %w", cwd, err)
	}

	result, err := provider.RequestRoots(ctx, mcp.ListRootsRequest{})
	if err != nil {
		return fmt.Errorf("client does not support roots (required for dispatch): %w", err)
	}

	if len(result.Roots) == 0 {
		// Client supports roots but declared none — treat as unrestricted.
		// This happens when the workspace has no explicit folder roots
		// (e.g. the .claude workspace opened via a .code-workspace file).
		return nil
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
