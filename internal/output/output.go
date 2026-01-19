// Package output handles command output routing (ZLE, clipboard, print).
package output

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Common errors returned by output functions.
var (
	// ErrNoClipboard is returned when no clipboard tool is available.
	ErrNoClipboard = errors.New("no clipboard tool available")

	// ErrUnsupportedOS is returned for unsupported operating systems.
	ErrUnsupportedOS = errors.New("unsupported operating system")

	// ErrInvalidMode is returned when an invalid output mode string is provided.
	ErrInvalidMode = errors.New("invalid output mode")
)

// Mode represents the output mode for commands.
type Mode int

const (
	// ModeZLE outputs raw command to stdout (for shell wrapper).
	// CRITICAL: No trailing newline - shell wrapper captures stdout directly.
	ModeZLE Mode = iota
	// ModeClipboard copies to clipboard with message to stderr.
	ModeClipboard
	// ModePrint prints command to stdout with newline.
	ModePrint
	// ModeAuto tries clipboard, falls back to print.
	ModeAuto
)

// String returns the string representation of the mode.
func (m Mode) String() string {
	switch m {
	case ModeZLE:
		return "zle"
	case ModeClipboard:
		return "clipboard"
	case ModePrint:
		return "print"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ParseMode parses a string into an output Mode.
// Returns ModeAuto for empty string. Returns ErrInvalidMode for unknown strings.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "zle":
		return ModeZLE, nil
	case "clipboard":
		return ModeClipboard, nil
	case "print":
		return ModePrint, nil
	case "auto", "":
		return ModeAuto, nil
	default:
		return ModeAuto, ErrInvalidMode
	}
}

// stdout and stderr can be overridden for testing
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// SetOutputWriters allows tests to capture output by replacing stdout/stderr.
// Pass nil to restore default behavior.
func SetOutputWriters(out, err io.Writer) {
	if out != nil {
		stdout = out
	} else {
		stdout = os.Stdout
	}
	if err != nil {
		stderr = err
	} else {
		stderr = os.Stderr
	}
}

// Output routes the command to the appropriate output based on mode and safety.
//
// Mode behaviors:
//   - ModeZLE: Raw command to stdout, NO trailing newline (for shell wrapper capture)
//   - ModeClipboard: Copy to clipboard, print confirmation to stderr
//   - ModePrint: Print command to stdout with newline
//   - ModeAuto: Try clipboard; if unavailable, fall back to print
//
// Dangerous command handling:
//   - If isDangerous is true AND mode is ModeZLE: Still output to stdout
//     (shell wrapper will print instead of injecting based on exit code)
//   - For other modes when isDangerous is true: Print warning to stderr
func Output(cmd string, mode Mode, isDangerous bool) error {
	// Handle dangerous command warnings for non-ZLE modes
	if isDangerous && mode != ModeZLE {
		printDangerWarning()
	}

	switch mode {
	case ModeZLE:
		// Raw command to stdout, NO trailing newline
		// Shell wrapper captures this and uses exit code to determine behavior
		_, err := fmt.Fprint(stdout, cmd)
		return err

	case ModeClipboard:
		return outputClipboard(cmd)

	case ModePrint:
		return outputPrint(cmd)

	case ModeAuto:
		return outputAuto(cmd)

	default:
		// Fallback to print for unknown modes
		return outputPrint(cmd)
	}
}

// outputClipboard copies the command to clipboard and prints confirmation to stderr.
func outputClipboard(cmd string) error {
	err := copyToClipboardWithOverride(cmd)
	if err != nil {
		// If clipboard fails, return the error
		// Caller can decide whether to fall back to print
		return err
	}
	fmt.Fprintln(stderr, "Command copied to clipboard.")
	return nil
}

// outputPrint prints the command to stdout with a trailing newline.
func outputPrint(cmd string) error {
	_, err := fmt.Fprintln(stdout, cmd)
	return err
}

// outputAuto tries clipboard first, falls back to print if unavailable.
// This provides graceful degradation without error spam.
func outputAuto(cmd string) error {
	// Check if clipboard is available first
	if !HasClipboard() {
		// No clipboard available, fall back to print silently
		return outputPrint(cmd)
	}

	// Try clipboard
	err := copyToClipboardWithOverride(cmd)
	if err != nil {
		// Clipboard failed, fall back to print
		// Don't spam errors - just gracefully degrade
		return outputPrint(cmd)
	}

	fmt.Fprintln(stderr, "Command copied to clipboard.")
	return nil
}

// printDangerWarning prints a warning to stderr about dangerous commands.
func printDangerWarning() {
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "WARNING: This command has been flagged as potentially dangerous.")
	fmt.Fprintln(stderr, "Review carefully before executing.")
	fmt.Fprintln(stderr, "")
}
