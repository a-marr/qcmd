package main

import (
	"testing"

	"github.com/user/qcmd/internal/config"
	"github.com/user/qcmd/internal/output"
	"github.com/user/qcmd/internal/sanitize"
)

// TestOutputModePrecedence verifies that --output flag overrides config
// and config is used when flag is absent.
func TestOutputModePrecedence(t *testing.T) {
	tests := []struct {
		name           string
		flagValue      string
		configValue    string
		expectedMode   output.Mode
		expectError    bool
	}{
		{
			name:         "flag overrides config - flag=print, config=clipboard",
			flagValue:    "print",
			configValue:  "clipboard",
			expectedMode: output.ModePrint,
			expectError:  false,
		},
		{
			name:         "flag overrides config - flag=clipboard, config=print",
			flagValue:    "clipboard",
			configValue:  "print",
			expectedMode: output.ModeClipboard,
			expectError:  false,
		},
		{
			name:         "flag overrides config - flag=zle, config=auto",
			flagValue:    "zle",
			configValue:  "auto",
			expectedMode: output.ModeZLE,
			expectError:  false,
		},
		{
			name:         "config used when flag absent - config=print",
			flagValue:    "",
			configValue:  "print",
			expectedMode: output.ModePrint,
			expectError:  false,
		},
		{
			name:         "config used when flag absent - config=clipboard",
			flagValue:    "",
			configValue:  "clipboard",
			expectedMode: output.ModeClipboard,
			expectError:  false,
		},
		{
			name:         "config used when flag absent - config=zle",
			flagValue:    "",
			configValue:  "zle",
			expectedMode: output.ModeZLE,
			expectError:  false,
		},
		{
			name:         "invalid flag returns error",
			flagValue:    "invalid",
			configValue:  "auto",
			expectedMode: output.ModeAuto,
			expectError:  true,
		},
		{
			name:         "invalid config falls back to auto",
			flagValue:    "",
			configValue:  "invalid",
			expectedMode: output.ModeAuto,
			expectError:  false,
		},
		{
			name:         "empty flag and empty config defaults to auto",
			flagValue:    "",
			configValue:  "",
			expectedMode: output.ModeAuto,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from main.go lines 103-121
			var outputMode output.Mode
			var parseErr error

			if tt.flagValue != "" {
				// Flag explicitly provided - parse it.
				outputMode, parseErr = output.ParseMode(tt.flagValue)
				if parseErr != nil {
					if !tt.expectError {
						t.Errorf("unexpected error parsing flag: %v", parseErr)
					}
					return
				}
			} else {
				// No flag provided - use config value (or default to auto).
				outputMode, parseErr = output.ParseMode(tt.configValue)
				if parseErr != nil {
					// Config has invalid value, fall back to auto.
					outputMode = output.ModeAuto
				}
			}

			if tt.expectError {
				t.Errorf("expected error but got none")
				return
			}

			if outputMode != tt.expectedMode {
				t.Errorf("outputMode = %v, want %v", outputMode, tt.expectedMode)
			}
		})
	}
}

// TestEmptySanitizedOutputHandling verifies that whitespace-only and
// fenced-only LLM responses result in empty sanitized output.
func TestEmptySanitizedOutputHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		isEmpty  bool
	}{
		{
			name:    "whitespace only",
			input:   "   \n\n\t  ",
			isEmpty: true,
		},
		{
			name:    "empty code fence",
			input:   "```\n```",
			isEmpty: true,
		},
		{
			name:    "code fence with only whitespace",
			input:   "```bash\n   \n```",
			isEmpty: true,
		},
		{
			name:    "code fence with only newlines",
			input:   "```\n\n\n```",
			isEmpty: true,
		},
		{
			name:    "empty string",
			input:   "",
			isEmpty: true,
		},
		{
			name:    "valid command should not be empty",
			input:   "ls -la",
			isEmpty: false,
		},
		{
			name:    "valid command in fence should not be empty",
			input:   "```bash\nls -la\n```",
			isEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized := sanitize.Sanitize(tt.input)
			gotEmpty := sanitized == ""

			if gotEmpty != tt.isEmpty {
				t.Errorf("Sanitize(%q) isEmpty = %v, want %v (got %q)",
					tt.input, gotEmpty, tt.isEmpty, sanitized)
			}
		})
	}
}

// TestEmptySanitizedOutputShouldFail verifies the error handling behavior
// that main.go should exhibit when sanitized output is empty.
func TestEmptySanitizedOutputShouldFail(t *testing.T) {
	// These inputs should all result in empty sanitized output,
	// which main.go should treat as an error (exitUserError).
	emptyInputs := []string{
		"",
		"   ",
		"\n\n",
		"```\n```",
		"```bash\n\n```",
		"```shell\n   \n```",
	}

	for _, input := range emptyInputs {
		sanitized := sanitize.Sanitize(input)
		if sanitized != "" {
			t.Errorf("expected Sanitize(%q) to return empty, got %q", input, sanitized)
		}
	}
}

// TestConfigOutputModeIntegration tests that config.Default() returns
// a valid output_mode that can be parsed.
func TestConfigOutputModeIntegration(t *testing.T) {
	cfg := config.Default()

	mode, err := output.ParseMode(cfg.OutputMode)
	if err != nil {
		t.Errorf("config.Default().OutputMode = %q is not parseable: %v", cfg.OutputMode, err)
	}

	if mode != output.ModeAuto {
		t.Errorf("config.Default() OutputMode = %v, want ModeAuto", mode)
	}
}
