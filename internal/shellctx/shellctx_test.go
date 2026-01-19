package shellctx

import (
	"os"
	"runtime"
	"testing"
)

func TestGatherContext(t *testing.T) {
	ctx := GatherContext()

	if ctx == nil {
		t.Fatal("GatherContext() returned nil")
	}

	// WorkingDir should not be empty (or should be "unknown" if error)
	if ctx.WorkingDir == "" {
		t.Error("GatherContext().WorkingDir should not be empty")
	}

	// Shell should not be empty (or should be "unknown" if $SHELL not set)
	if ctx.Shell == "" {
		t.Error("GatherContext().Shell should not be empty")
	}

	// OS should match runtime.GOOS
	if ctx.OS != runtime.GOOS {
		t.Errorf("GatherContext().OS = %q, want %q", ctx.OS, runtime.GOOS)
	}
}

func TestGatherContextWithShell(t *testing.T) {
	// Save original SHELL env var
	origShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", origShell)

	tests := []struct {
		name      string
		shell     string
		wantShell string
	}{
		{
			name:      "zsh",
			shell:     "/bin/zsh",
			wantShell: "zsh",
		},
		{
			name:      "bash",
			shell:     "/bin/bash",
			wantShell: "bash",
		},
		{
			name:      "fish in usr/local",
			shell:     "/usr/local/bin/fish",
			wantShell: "fish",
		},
		{
			name:      "homebrew zsh",
			shell:     "/opt/homebrew/bin/zsh",
			wantShell: "zsh",
		},
		{
			name:      "empty SHELL",
			shell:     "",
			wantShell: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("SHELL", tt.shell)

			ctx := GatherContext()

			if ctx.Shell != tt.wantShell {
				t.Errorf("GatherContext().Shell = %q, want %q", ctx.Shell, tt.wantShell)
			}
		})
	}
}

func TestGatherContextWorkingDir(t *testing.T) {
	ctx := GatherContext()

	// Get expected working directory
	expected, err := os.Getwd()
	if err != nil {
		// If we can't get pwd, it should be "unknown"
		if ctx.WorkingDir != "unknown" {
			t.Errorf("GatherContext().WorkingDir = %q, want %q (when Getwd fails)", ctx.WorkingDir, "unknown")
		}
		return
	}

	if ctx.WorkingDir != expected {
		t.Errorf("GatherContext().WorkingDir = %q, want %q", ctx.WorkingDir, expected)
	}
}

func TestGatherContextOS(t *testing.T) {
	ctx := GatherContext()

	// OS should be a valid value
	validOS := map[string]bool{
		"darwin":  true,
		"linux":   true,
		"windows": true,
		"freebsd": true,
		"openbsd": true,
		"netbsd":  true,
	}

	if !validOS[ctx.OS] {
		// It's still valid if it's runtime.GOOS, even if not in our map
		if ctx.OS != runtime.GOOS {
			t.Errorf("GatherContext().OS = %q, want %q", ctx.OS, runtime.GOOS)
		}
	}
}

func TestGetShellFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "zsh",
			path:     "/bin/zsh",
			expected: "zsh",
		},
		{
			name:     "bash",
			path:     "/usr/bin/bash",
			expected: "bash",
		},
		{
			name:     "fish",
			path:     "/usr/local/bin/fish",
			expected: "fish",
		},
		{
			name:     "just shell name",
			path:     "zsh",
			expected: "zsh",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "unknown",
		},
		{
			name:     "trailing slash",
			path:     "/bin/zsh/",
			expected: "zsh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetShellFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("GetShellFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGatherContextNeverReturnsNil(t *testing.T) {
	// Even with weird environment, should never return nil
	origShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", origShell)

	os.Setenv("SHELL", "")

	ctx := GatherContext()
	if ctx == nil {
		t.Fatal("GatherContext() should never return nil")
	}

	// All fields should have values
	if ctx.WorkingDir == "" {
		t.Error("WorkingDir should not be empty even with unset SHELL")
	}
	if ctx.Shell == "" {
		t.Error("Shell should be 'unknown', not empty")
	}
	if ctx.OS == "" {
		t.Error("OS should not be empty")
	}
}

func TestShellContextFields(t *testing.T) {
	ctx := GatherContext()

	// Verify the struct is properly populated
	t.Logf("WorkingDir: %s", ctx.WorkingDir)
	t.Logf("Shell: %s", ctx.Shell)
	t.Logf("OS: %s", ctx.OS)

	// Basic sanity checks
	if ctx.WorkingDir == "unknown" {
		// This is fine if Getwd actually failed
		_, err := os.Getwd()
		if err == nil {
			t.Log("Note: WorkingDir is 'unknown' but Getwd succeeded")
		}
	}

	// OS should always be a valid runtime.GOOS value
	if ctx.OS != runtime.GOOS {
		t.Errorf("OS mismatch: got %q, want %q", ctx.OS, runtime.GOOS)
	}
}
