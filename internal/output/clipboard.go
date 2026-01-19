// Package output handles command output routing (ZLE, clipboard, print).
package output

import (
	"os/exec"
	"runtime"
	"strings"
)

// CopyToClipboard copies text to the system clipboard.
// It automatically detects the appropriate clipboard tool based on the OS:
// - macOS: pbcopy
// - Linux: wl-copy (Wayland), xclip, or xsel
//
// Returns ErrNoClipboard if no clipboard tool is available on Linux.
// Returns ErrUnsupportedOS for unsupported operating systems.
func CopyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try clipboard tools in order of preference:
		// 1. wl-copy (Wayland) - modern Linux desktop
		// 2. xclip - common X11 clipboard tool
		// 3. xsel - alternative X11 clipboard tool
		if hasCommand("wl-copy") {
			cmd = exec.Command("wl-copy")
		} else if hasCommand("xclip") {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if hasCommand("xsel") {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return ErrNoClipboard
		}
	default:
		return ErrUnsupportedOS
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// HasClipboard returns true if a clipboard tool is available on the current system.
// This can be used to determine if clipboard operations will succeed before attempting them.
func HasClipboard() bool {
	switch runtime.GOOS {
	case "darwin":
		// macOS always has pbcopy
		return hasCommand("pbcopy")
	case "linux":
		// Check for any of the supported Linux clipboard tools
		return hasCommand("wl-copy") || hasCommand("xclip") || hasCommand("xsel")
	default:
		return false
	}
}

// hasCommand checks if a command exists in the system PATH.
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// clipboardTool is a package-level variable that allows tests to override
// the clipboard tool detection. When nil, the default detection is used.
var clipboardTool func(text string) error

// SetClipboardFunc allows tests to inject a custom clipboard function.
// Pass nil to restore default behavior.
func SetClipboardFunc(fn func(text string) error) {
	clipboardTool = fn
}

// copyToClipboardWithOverride uses the injected clipboard function if available,
// otherwise falls back to the real implementation.
func copyToClipboardWithOverride(text string) error {
	if clipboardTool != nil {
		return clipboardTool(text)
	}
	return CopyToClipboard(text)
}
