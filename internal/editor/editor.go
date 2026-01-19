// Package editor handles temporary file creation and $EDITOR invocation.
package editor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Editor handles opening the user's preferred editor for query input.
type Editor struct {
	// EditorCmd overrides the default editor command ($VISUAL, $EDITOR, or vi).
	EditorCmd string
}

// NewEditor creates a new Editor with an optional command override.
// If editorCmd is empty, the default lookup chain will be used.
func NewEditor(editorCmd string) *Editor {
	return &Editor{EditorCmd: editorCmd}
}

// GetInput opens the editor and returns user input.
// Returns empty string if file is empty or only comments.
// Returns error if editor fails to launch.
func (e *Editor) GetInput(ctx context.Context) (string, error) {
	// Create secure temp file with 0600 permissions
	tmpFile, err := os.CreateTemp("", "qcmd-*.txt")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup happens regardless of outcome
	defer func() {
		os.Remove(tmpPath)
	}()

	// Write template to file
	if _, err := tmpFile.WriteString(InputTemplate); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("writing template: %w", err)
	}

	// Explicitly set permissions to 0600 (CreateTemp may not guarantee this)
	if err := tmpFile.Chmod(0600); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("setting file permissions: %w", err)
	}

	// Close file before opening in editor
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	// Get editor command
	cmdParts := getEditorCommand(e.EditorCmd)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("no editor command found")
	}

	// Append temp file path to command
	cmdParts = append(cmdParts, tmpPath)

	// Create command with context
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run editor and wait for it to exit
	if err := cmd.Run(); err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return "", fmt.Errorf("editor cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("running editor: %w", err)
	}

	// Read file contents
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("reading temp file: %w", err)
	}

	// Process and return input
	return ProcessInput(string(content)), nil
}

// ProcessInput cleans up raw editor input.
// It removes comment lines (starting with #) and trims blank lines.
func ProcessInput(raw string) string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(raw))

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comment lines
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Keep original line (preserving indentation if any)
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return ""
	}

	// Join non-empty, non-comment lines
	result := strings.Join(lines, "\n")

	// Trim any trailing whitespace
	return strings.TrimSpace(result)
}

// getEditorCommand returns the editor command split into executable and arguments.
// It follows the precedence: override -> $VISUAL -> $EDITOR -> vi
func getEditorCommand(override string) []string {
	var editorStr string

	if override != "" {
		editorStr = override
	} else if visual := os.Getenv("VISUAL"); visual != "" {
		editorStr = visual
	} else if editor := os.Getenv("EDITOR"); editor != "" {
		editorStr = editor
	} else {
		editorStr = "vi"
	}

	// Split on spaces to handle editors with arguments like "code --wait"
	// This handles simple cases; complex quoting is not supported
	parts := strings.Fields(editorStr)
	if len(parts) == 0 {
		return []string{"vi"}
	}

	return parts
}

// GetEditorPath returns the resolved path of the editor for display purposes.
// Returns the editor name even if the full path cannot be resolved.
func GetEditorPath(override string) string {
	parts := getEditorCommand(override)
	if len(parts) == 0 {
		return "vi"
	}

	// Try to resolve the full path
	if path, err := exec.LookPath(parts[0]); err == nil {
		return path
	}

	return parts[0]
}

// InputTemplate is the template shown to users in the editor.
const InputTemplate = `# Describe the shell command you need
# Lines starting with # are ignored
# Save and quit when done (:wq in vim)

`

// TempFilePrefix is the prefix used for temp files.
const TempFilePrefix = "qcmd-"

// TempFilePattern is the full pattern for temp file naming.
func TempFilePattern() string {
	return filepath.Join(os.TempDir(), TempFilePrefix+"*.txt")
}
