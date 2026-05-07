package provider

import (
	"os"
	"os/exec"
	"strings"
)

// ensureShellPATH augments the process PATH with the user's login shell PATH.
// MCP servers often inherit a stripped-down PATH that lacks nvm, volta, pyenv,
// etc. This resolves the user's full PATH once at startup and merges it.
func init() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, "-l", "-c", "echo $PATH")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return
	}

	shellPATH := strings.TrimSpace(string(out))
	if shellPATH == "" {
		return
	}

	// Merge: keep existing process PATH entries, append any new ones from shell.
	currentPATH := os.Getenv("PATH")
	existing := make(map[string]bool)
	for _, dir := range strings.Split(currentPATH, ":") {
		existing[dir] = true
	}

	var newDirs []string
	for _, dir := range strings.Split(shellPATH, ":") {
		if !existing[dir] {
			newDirs = append(newDirs, dir)
		}
	}

	if len(newDirs) > 0 {
		merged := currentPATH + ":" + strings.Join(newDirs, ":")
		os.Setenv("PATH", merged)
	}
}
