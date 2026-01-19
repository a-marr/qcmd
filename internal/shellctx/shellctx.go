// Package shellctx gathers shell context information (pwd, shell, OS).
package shellctx

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/user/qcmd/internal/backend"
)

// GatherContext collects information about the current shell environment.
// Returns a ShellContext with the working directory, shell type, and OS.
// Never returns nil - if values cannot be determined, sensible defaults are used.
func GatherContext() *backend.ShellContext {
	return &backend.ShellContext{
		WorkingDir: getWorkingDir(),
		Shell:      getShell(),
		OS:         runtime.GOOS,
	}
}

// getWorkingDir returns the current working directory.
// Returns "unknown" if it cannot be determined.
func getWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return wd
}

// getShell returns the shell type (e.g., "zsh", "bash").
// It checks the $SHELL environment variable and extracts the basename.
// Returns "unknown" if $SHELL is not set.
func getShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown"
	}

	// Extract the basename (e.g., "/bin/zsh" -> "zsh")
	return filepath.Base(shell)
}

// GetShellFromPath extracts the shell name from a full path.
// Exported for testing purposes.
func GetShellFromPath(shellPath string) string {
	if shellPath == "" {
		return "unknown"
	}
	return filepath.Base(shellPath)
}
