package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestParseMode tests the ParseMode function with valid and invalid inputs.
func TestParseMode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMode    Mode
		wantErr     bool
		wantErrType error
	}{
		// Valid modes
		{"zle mode", "zle", ModeZLE, false, nil},
		{"clipboard mode", "clipboard", ModeClipboard, false, nil},
		{"print mode", "print", ModePrint, false, nil},
		{"auto mode", "auto", ModeAuto, false, nil},
		{"empty string defaults to auto", "", ModeAuto, false, nil},

		// Invalid modes
		{"invalid mode xyz", "xyz", ModeAuto, true, ErrInvalidMode},
		{"invalid mode ZLE (case sensitive)", "ZLE", ModeAuto, true, ErrInvalidMode},
		{"invalid mode Clipboard (case sensitive)", "Clipboard", ModeAuto, true, ErrInvalidMode},
		{"invalid mode with spaces", " print ", ModeAuto, true, ErrInvalidMode},
		{"invalid numeric mode", "0", ModeAuto, true, ErrInvalidMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMode(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMode(%q) expected error, got nil", tt.input)
					return
				}
				if tt.wantErrType != nil && !errors.Is(err, tt.wantErrType) {
					t.Errorf("ParseMode(%q) error = %v, want %v", tt.input, err, tt.wantErrType)
				}
			} else {
				if err != nil {
					t.Errorf("ParseMode(%q) unexpected error: %v", tt.input, err)
					return
				}
			}

			if got != tt.wantMode {
				t.Errorf("ParseMode(%q) = %v, want %v", tt.input, got, tt.wantMode)
			}
		})
	}
}

// TestModeString tests the String method on Mode.
func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeZLE, "zle"},
		{ModeClipboard, "clipboard"},
		{ModePrint, "print"},
		{ModeAuto, "auto"},
		{Mode(99), "unknown"}, // Invalid mode
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// TestOutputZLE tests that ZLE mode outputs raw command without trailing newline.
func TestOutputZLE(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		isDangerous bool
		wantStdout  string
		wantStderr  string
	}{
		{
			name:        "simple command",
			cmd:         "ls -la",
			isDangerous: false,
			wantStdout:  "ls -la", // NO trailing newline
			wantStderr:  "",
		},
		{
			name:        "multi-line command",
			cmd:         "docker run \\\n  -v /data:/data \\\n  nginx",
			isDangerous: false,
			wantStdout:  "docker run \\\n  -v /data:/data \\\n  nginx",
			wantStderr:  "",
		},
		{
			name:        "dangerous command still outputs to stdout",
			cmd:         "rm -rf /",
			isDangerous: true,
			wantStdout:  "rm -rf /", // Still outputs - shell wrapper handles the warning
			wantStderr:  "",         // No warning in ZLE mode - exit code signals danger
		},
		{
			name:        "empty command",
			cmd:         "",
			isDangerous: false,
			wantStdout:  "",
			wantStderr:  "",
		},
		{
			name:        "command with special characters",
			cmd:         `echo "hello world" | grep 'hello'`,
			isDangerous: false,
			wantStdout:  `echo "hello world" | grep 'hello'`,
			wantStderr:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}
			SetOutputWriters(stdoutBuf, stderrBuf)
			defer SetOutputWriters(nil, nil)

			err := Output(tt.cmd, ModeZLE, tt.isDangerous)
			if err != nil {
				t.Errorf("Output() unexpected error: %v", err)
				return
			}

			gotStdout := stdoutBuf.String()
			gotStderr := stderrBuf.String()

			if gotStdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", gotStdout, tt.wantStdout)
			}
			if gotStderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", gotStderr, tt.wantStderr)
			}
		})
	}
}

// TestOutputPrint tests that print mode outputs command with trailing newline.
func TestOutputPrint(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		isDangerous bool
		wantStdout  string
		wantWarning bool
	}{
		{
			name:        "simple command",
			cmd:         "ls -la",
			isDangerous: false,
			wantStdout:  "ls -la\n", // WITH trailing newline
			wantWarning: false,
		},
		{
			name:        "dangerous command with warning",
			cmd:         "rm -rf /",
			isDangerous: true,
			wantStdout:  "rm -rf /\n",
			wantWarning: true,
		},
		{
			name:        "multi-line command",
			cmd:         "docker run \\\n  -v /data:/data",
			isDangerous: false,
			wantStdout:  "docker run \\\n  -v /data:/data\n",
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}
			SetOutputWriters(stdoutBuf, stderrBuf)
			defer SetOutputWriters(nil, nil)

			err := Output(tt.cmd, ModePrint, tt.isDangerous)
			if err != nil {
				t.Errorf("Output() unexpected error: %v", err)
				return
			}

			gotStdout := stdoutBuf.String()
			gotStderr := stderrBuf.String()

			if gotStdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", gotStdout, tt.wantStdout)
			}

			if tt.wantWarning {
				if !strings.Contains(gotStderr, "WARNING") {
					t.Errorf("stderr should contain warning, got: %q", gotStderr)
				}
				if !strings.Contains(gotStderr, "dangerous") {
					t.Errorf("stderr should mention 'dangerous', got: %q", gotStderr)
				}
			} else {
				if gotStderr != "" {
					t.Errorf("stderr should be empty, got: %q", gotStderr)
				}
			}
		})
	}
}

// TestOutputClipboard tests clipboard mode behavior.
func TestOutputClipboard(t *testing.T) {
	tests := []struct {
		name           string
		cmd            string
		isDangerous    bool
		clipboardErr   error
		wantErr        bool
		wantStdout     string
		wantStderrMsg  string
		wantWarning    bool
	}{
		{
			name:          "successful clipboard copy",
			cmd:           "ls -la",
			isDangerous:   false,
			clipboardErr:  nil,
			wantErr:       false,
			wantStdout:    "",
			wantStderrMsg: "Command copied to clipboard.",
			wantWarning:   false,
		},
		{
			name:          "clipboard copy with dangerous warning",
			cmd:           "rm -rf /",
			isDangerous:   true,
			clipboardErr:  nil,
			wantErr:       false,
			wantStdout:    "",
			wantStderrMsg: "Command copied to clipboard.",
			wantWarning:   true,
		},
		{
			name:          "clipboard unavailable",
			cmd:           "ls -la",
			isDangerous:   false,
			clipboardErr:  ErrNoClipboard,
			wantErr:       true,
			wantStdout:    "",
			wantStderrMsg: "",
			wantWarning:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}
			SetOutputWriters(stdoutBuf, stderrBuf)
			defer SetOutputWriters(nil, nil)

			// Mock clipboard function
			SetClipboardFunc(func(text string) error {
				if text != tt.cmd {
					t.Errorf("clipboard received %q, want %q", text, tt.cmd)
				}
				return tt.clipboardErr
			})
			defer SetClipboardFunc(nil)

			err := Output(tt.cmd, ModeClipboard, tt.isDangerous)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Output() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Output() unexpected error: %v", err)
				return
			}

			gotStdout := stdoutBuf.String()
			gotStderr := stderrBuf.String()

			if gotStdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", gotStdout, tt.wantStdout)
			}

			if tt.wantStderrMsg != "" && !strings.Contains(gotStderr, tt.wantStderrMsg) {
				t.Errorf("stderr should contain %q, got: %q", tt.wantStderrMsg, gotStderr)
			}

			if tt.wantWarning && !strings.Contains(gotStderr, "WARNING") {
				t.Errorf("stderr should contain warning, got: %q", gotStderr)
			}
		})
	}
}

// TestOutputAuto tests auto mode fallback behavior.
func TestOutputAuto(t *testing.T) {
	tests := []struct {
		name          string
		cmd           string
		hasClipboard  bool
		clipboardErr  error
		wantStdout    string
		wantStderrMsg string
	}{
		{
			name:          "clipboard available and works",
			cmd:           "ls -la",
			hasClipboard:  true,
			clipboardErr:  nil,
			wantStdout:    "",
			wantStderrMsg: "Command copied to clipboard.",
		},
		{
			name:          "clipboard available but fails - fallback to print",
			cmd:           "ls -la",
			hasClipboard:  true,
			clipboardErr:  errors.New("clipboard error"),
			wantStdout:    "ls -la\n",
			wantStderrMsg: "",
		},
		{
			name:          "no clipboard available - fallback to print",
			cmd:           "ls -la",
			hasClipboard:  false,
			clipboardErr:  nil,
			wantStdout:    "ls -la\n",
			wantStderrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}
			SetOutputWriters(stdoutBuf, stderrBuf)
			defer SetOutputWriters(nil, nil)

			// We need to mock HasClipboard behavior
			// Since we can't easily mock HasClipboard, we mock the clipboard function
			// and rely on the implementation details
			if tt.hasClipboard {
				SetClipboardFunc(func(text string) error {
					return tt.clipboardErr
				})
			} else {
				// When clipboard is not available, copyToClipboardWithOverride will
				// never be called because HasClipboard() returns false
				// For this test, we need to ensure the test environment
				// doesn't have clipboard tools, or we accept this limitation
				// For now, skip this specific test scenario
				SetClipboardFunc(func(text string) error {
					return ErrNoClipboard
				})
			}
			defer SetClipboardFunc(nil)

			err := Output(tt.cmd, ModeAuto, false)
			if err != nil {
				t.Errorf("Output() unexpected error: %v", err)
				return
			}

			gotStdout := stdoutBuf.String()
			gotStderr := stderrBuf.String()

			if gotStdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", gotStdout, tt.wantStdout)
			}

			if tt.wantStderrMsg != "" {
				if !strings.Contains(gotStderr, tt.wantStderrMsg) {
					t.Errorf("stderr should contain %q, got: %q", tt.wantStderrMsg, gotStderr)
				}
			}
		})
	}
}

// TestOutputZLENoTrailingNewline specifically verifies the critical requirement
// that ZLE mode does not add a trailing newline.
func TestOutputZLENoTrailingNewline(t *testing.T) {
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	SetOutputWriters(stdoutBuf, stderrBuf)
	defer SetOutputWriters(nil, nil)

	cmd := "echo test"
	err := Output(cmd, ModeZLE, false)
	if err != nil {
		t.Fatalf("Output() error: %v", err)
	}

	got := stdoutBuf.String()

	// CRITICAL: Must not end with newline
	if strings.HasSuffix(got, "\n") {
		t.Errorf("ZLE mode output must NOT have trailing newline, got: %q", got)
	}

	// Must be exact match
	if got != cmd {
		t.Errorf("ZLE output = %q, want %q", got, cmd)
	}
}

// TestDangerousCommandHandling tests the danger warning behavior across modes.
func TestDangerousCommandHandling(t *testing.T) {
	tests := []struct {
		name        string
		mode        Mode
		wantWarning bool
	}{
		{"ZLE mode no warning", ModeZLE, false},
		{"Print mode has warning", ModePrint, true},
		{"Clipboard mode has warning", ModeClipboard, true},
		{"Auto mode has warning", ModeAuto, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}
			SetOutputWriters(stdoutBuf, stderrBuf)
			defer SetOutputWriters(nil, nil)

			// Mock clipboard to always succeed
			SetClipboardFunc(func(text string) error {
				return nil
			})
			defer SetClipboardFunc(nil)

			_ = Output("rm -rf /", tt.mode, true)

			gotStderr := stderrBuf.String()
			hasWarning := strings.Contains(gotStderr, "WARNING")

			if hasWarning != tt.wantWarning {
				t.Errorf("warning presence = %v, want %v, stderr: %q", hasWarning, tt.wantWarning, gotStderr)
			}
		})
	}
}

// TestHasClipboard tests the HasClipboard function.
// Note: This test's behavior depends on the system's available tools.
func TestHasClipboard(t *testing.T) {
	// This is a simple smoke test - actual behavior depends on system
	// We just verify it doesn't panic and returns a boolean
	result := HasClipboard()
	t.Logf("HasClipboard() = %v", result)
}

// TestHasCommand tests the hasCommand helper function.
func TestHasCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantNot bool // true if we expect false (command doesn't exist)
	}{
		// Commands that should exist on most systems
		{"sh exists", "sh", false},

		// Commands that should NOT exist
		{"nonexistent command", "qcmd_nonexistent_command_xyz123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCommand(tt.cmd)
			if tt.wantNot && got {
				t.Errorf("hasCommand(%q) = true, expected false", tt.cmd)
			}
			if !tt.wantNot && !got {
				t.Errorf("hasCommand(%q) = false, expected true", tt.cmd)
			}
		})
	}
}

// TestCopyToClipboardErrors tests error conditions for clipboard operations.
func TestCopyToClipboardErrors(t *testing.T) {
	// Test that the function returns appropriate errors for unsupported OS
	// This is tricky to test without mocking runtime.GOOS
	// We can at least verify the error types exist and are usable

	if ErrNoClipboard == nil {
		t.Error("ErrNoClipboard should not be nil")
	}

	if ErrUnsupportedOS == nil {
		t.Error("ErrUnsupportedOS should not be nil")
	}

	if ErrInvalidMode == nil {
		t.Error("ErrInvalidMode should not be nil")
	}
}

// TestSetOutputWritersRestore tests that SetOutputWriters(nil, nil) restores defaults.
func TestSetOutputWritersRestore(t *testing.T) {
	// Capture current values
	originalStdout := stdout

	// Set custom writers
	buf := &bytes.Buffer{}
	SetOutputWriters(buf, buf)

	if stdout == originalStdout {
		t.Error("SetOutputWriters should have changed stdout")
	}

	// Restore defaults
	SetOutputWriters(nil, nil)

	// After restoration, stdout should be os.Stdout (not the buffer)
	// We can't directly compare to os.Stdout, but we can verify it's not the buffer
	if stdout == buf {
		t.Error("SetOutputWriters(nil, nil) should have restored stdout")
	}
}

// TestMultiLineCommandPreservation tests that multi-line commands are preserved correctly.
func TestMultiLineCommandPreservation(t *testing.T) {
	commands := []string{
		"docker run \\\n  --name myapp \\\n  -v /data:/data \\\n  nginx",
		"find . -name '*.go' \\\n  | xargs grep TODO",
		"cat <<EOF\nhello\nworld\nEOF",
		"for i in 1 2 3; do\n  echo $i\ndone",
	}

	for _, cmd := range commands {
		t.Run("", func(t *testing.T) {
			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}
			SetOutputWriters(stdoutBuf, stderrBuf)
			defer SetOutputWriters(nil, nil)

			// Test ZLE mode (no trailing newline)
			err := Output(cmd, ModeZLE, false)
			if err != nil {
				t.Errorf("Output() error: %v", err)
				return
			}

			got := stdoutBuf.String()
			if got != cmd {
				t.Errorf("Multi-line command not preserved.\ngot:\n%s\nwant:\n%s", got, cmd)
			}
		})
	}
}
