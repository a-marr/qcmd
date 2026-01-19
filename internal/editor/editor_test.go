package editor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple input",
			input:    "list all files",
			expected: "list all files",
		},
		{
			name:     "input with comment",
			input:    "# this is a comment\nlist all files",
			expected: "list all files",
		},
		{
			name:     "multiple comments",
			input:    "# comment 1\n# comment 2\nlist all files\n# trailing comment",
			expected: "list all files",
		},
		{
			name:     "comment with leading whitespace",
			input:    "  # indented comment\nlist all files",
			expected: "list all files",
		},
		{
			name:     "input with blank lines",
			input:    "\n\nlist all files\n\n",
			expected: "list all files",
		},
		{
			name:     "template only",
			input:    InputTemplate,
			expected: "",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \n\t\n   ",
			expected: "",
		},
		{
			name:     "only comments",
			input:    "# comment 1\n# comment 2\n# comment 3",
			expected: "",
		},
		{
			name:     "multiline input",
			input:    "# header\nfind . -name '*.go'\n# middle comment\ngrep -r 'TODO'",
			expected: "find . -name '*.go'\ngrep -r 'TODO'",
		},
		{
			name:     "preserves indentation",
			input:    "if [ -f file ]; then\n    echo 'exists'\nfi",
			expected: "if [ -f file ]; then\n    echo 'exists'\nfi",
		},
		{
			name:     "hash in middle of line is kept",
			input:    "echo 'hello # world'",
			expected: "echo 'hello # world'",
		},
		{
			name:     "trailing whitespace trimmed",
			input:    "list files   \n",
			expected: "list files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessInput(tt.input)
			if result != tt.expected {
				t.Errorf("ProcessInput(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetEditorCommand(t *testing.T) {
	// Save original env vars
	origVisual := os.Getenv("VISUAL")
	origEditor := os.Getenv("EDITOR")
	defer func() {
		os.Setenv("VISUAL", origVisual)
		os.Setenv("EDITOR", origEditor)
	}()

	tests := []struct {
		name     string
		override string
		visual   string
		editor   string
		expected []string
	}{
		{
			name:     "override takes precedence",
			override: "nano",
			visual:   "vim",
			editor:   "emacs",
			expected: []string{"nano"},
		},
		{
			name:     "VISUAL over EDITOR",
			override: "",
			visual:   "vim",
			editor:   "emacs",
			expected: []string{"vim"},
		},
		{
			name:     "EDITOR when no VISUAL",
			override: "",
			visual:   "",
			editor:   "emacs",
			expected: []string{"emacs"},
		},
		{
			name:     "fallback to vi",
			override: "",
			visual:   "",
			editor:   "",
			expected: []string{"vi"},
		},
		{
			name:     "editor with arguments",
			override: "code --wait",
			visual:   "",
			editor:   "",
			expected: []string{"code", "--wait"},
		},
		{
			name:     "editor with multiple arguments",
			override: "nvim -u NONE -c 'set noswapfile'",
			visual:   "",
			editor:   "",
			expected: []string{"nvim", "-u", "NONE", "-c", "'set", "noswapfile'"},
		},
		{
			name:     "VISUAL with arguments",
			override: "",
			visual:   "code --wait --new-window",
			editor:   "",
			expected: []string{"code", "--wait", "--new-window"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("VISUAL", tt.visual)
			os.Setenv("EDITOR", tt.editor)

			result := getEditorCommand(tt.override)

			if len(result) != len(tt.expected) {
				t.Errorf("getEditorCommand(%q) = %v, want %v", tt.override, result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("getEditorCommand(%q)[%d] = %q, want %q", tt.override, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestNewEditor(t *testing.T) {
	e := NewEditor("custom-editor")
	if e.EditorCmd != "custom-editor" {
		t.Errorf("NewEditor(%q).EditorCmd = %q, want %q", "custom-editor", e.EditorCmd, "custom-editor")
	}

	e2 := NewEditor("")
	if e2.EditorCmd != "" {
		t.Errorf("NewEditor(%q).EditorCmd = %q, want %q", "", e2.EditorCmd, "")
	}
}

func TestTempFileCreationAndCleanup(t *testing.T) {
	// We can't easily test the full GetInput flow without a real editor,
	// but we can test temp file handling by using a fake editor that just exits.

	// Create a test script that acts as a no-op editor
	tmpDir := t.TempDir()
	fakeEditor := filepath.Join(tmpDir, "fake-editor.sh")
	err := os.WriteFile(fakeEditor, []byte("#!/bin/sh\nexit 0\n"), 0755)
	if err != nil {
		t.Fatalf("creating fake editor: %v", err)
	}

	e := NewEditor(fakeEditor)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := e.GetInput(ctx)
	if err != nil {
		t.Fatalf("GetInput failed: %v", err)
	}

	// The fake editor doesn't modify the file, so we should get empty string
	// (template only contains comments)
	if result != "" {
		t.Errorf("GetInput() = %q, want empty string", result)
	}
}

func TestGetInputWithContent(t *testing.T) {
	// Create a fake editor that writes content to the file
	tmpDir := t.TempDir()
	fakeEditor := filepath.Join(tmpDir, "fake-editor.sh")

	// This editor appends content to the file
	script := `#!/bin/sh
echo "list all go files" >> "$1"
`
	err := os.WriteFile(fakeEditor, []byte(script), 0755)
	if err != nil {
		t.Fatalf("creating fake editor: %v", err)
	}

	e := NewEditor(fakeEditor)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := e.GetInput(ctx)
	if err != nil {
		t.Fatalf("GetInput failed: %v", err)
	}

	expected := "list all go files"
	if result != expected {
		t.Errorf("GetInput() = %q, want %q", result, expected)
	}
}

func TestGetInputEditorFailure(t *testing.T) {
	// Create a fake editor that exits with an error
	tmpDir := t.TempDir()
	fakeEditor := filepath.Join(tmpDir, "failing-editor.sh")

	err := os.WriteFile(fakeEditor, []byte("#!/bin/sh\nexit 1\n"), 0755)
	if err != nil {
		t.Fatalf("creating fake editor: %v", err)
	}

	e := NewEditor(fakeEditor)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = e.GetInput(ctx)
	if err == nil {
		t.Error("GetInput() should return error when editor fails")
	}
}

func TestGetInputContextCancellation(t *testing.T) {
	// Create a fake editor that sleeps
	tmpDir := t.TempDir()
	fakeEditor := filepath.Join(tmpDir, "slow-editor.sh")

	err := os.WriteFile(fakeEditor, []byte("#!/bin/sh\nsleep 60\n"), 0755)
	if err != nil {
		t.Fatalf("creating fake editor: %v", err)
	}

	e := NewEditor(fakeEditor)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = e.GetInput(ctx)
	if err == nil {
		t.Error("GetInput() should return error when context is cancelled")
	}
}

func TestTempFilePermissions(t *testing.T) {
	// Create a fake editor that checks file permissions
	tmpDir := t.TempDir()
	resultFile := filepath.Join(tmpDir, "result.txt")
	fakeEditor := filepath.Join(tmpDir, "perm-check-editor.sh")

	// This editor checks if the file has 0600 permissions and writes result
	script := `#!/bin/sh
PERMS=$(stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1" 2>/dev/null)
echo "$PERMS" > ` + resultFile + `
exit 0
`
	err := os.WriteFile(fakeEditor, []byte(script), 0755)
	if err != nil {
		t.Fatalf("creating fake editor: %v", err)
	}

	e := NewEditor(fakeEditor)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = e.GetInput(ctx)
	if err != nil {
		t.Fatalf("GetInput failed: %v", err)
	}

	// Read the permissions result
	permsBytes, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	perms := string(permsBytes)
	// Trim whitespace
	perms = perms[:len(perms)-1] // remove trailing newline

	if perms != "600" {
		t.Errorf("temp file permissions = %q, want %q", perms, "600")
	}
}

func TestGetEditorPath(t *testing.T) {
	// Test that GetEditorPath returns something reasonable
	path := GetEditorPath("")
	if path == "" {
		t.Error("GetEditorPath(\"\") should not return empty string")
	}

	// Test with override
	path = GetEditorPath("custom-editor")
	if path != "custom-editor" {
		// It won't be found in PATH, so should return the name as-is
		t.Logf("GetEditorPath(\"custom-editor\") = %q (expected since not in PATH)", path)
	}
}

func TestInputTemplateConstant(t *testing.T) {
	// Verify the template contains expected elements
	if !contains(InputTemplate, "#") {
		t.Error("InputTemplate should contain comment lines starting with #")
	}

	// Verify template ends with blank line for user input
	if !endsWith(InputTemplate, "\n") {
		t.Error("InputTemplate should end with newline")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
