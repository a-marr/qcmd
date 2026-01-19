// Package sanitize cleans LLM output while preserving command structure.
package sanitize

import (
	"regexp"
	"strings"
)

// codeFenceRegex matches markdown code fences with optional language specifier.
// Matches patterns like: ```bash\n...\n```, ```shell-session\n...\n```, or ```\n...\n```
var codeFenceRegex = regexp.MustCompile("(?s)^\\s*```[a-zA-Z0-9_-]*\\n?(.*?)\\n?```\\s*$")

// inlineBacktickRegex matches content wrapped in single backticks.
var inlineBacktickRegex = regexp.MustCompile("^`([^`]+)`$")

// dollarPrefixRegex matches a leading "$ " on the first line.
var dollarPrefixRegex = regexp.MustCompile(`^\$\s+`)

// errorSentinelRegex matches the QCMD_ERROR sentinel format.
// Matches: echo "QCMD_ERROR: message" or echo 'QCMD_ERROR: message'
var errorSentinelRegex = regexp.MustCompile(`^echo\s+["']QCMD_ERROR:\s*(.+?)["']$`)

// Sanitize cleans LLM output by removing markdown formatting while
// preserving multi-line command structure.
//
// Operations performed:
// 1. Remove markdown code fences (```bash ... ``` or ``` ... ```)
// 2. Remove inline backticks if entire output is wrapped
// 3. Remove "$ " prefix from first line if present
// 4. Strip leading blank lines and whitespace
// 5. Strip trailing blank lines and whitespace
// 6. Preserve internal newlines and structure (multi-line commands, heredocs)
func Sanitize(raw string) string {
	result := raw

	// Step 1: Remove markdown code fences if present
	// Handle fenced code blocks like ```bash\ncommand\n```
	if matches := codeFenceRegex.FindStringSubmatch(result); matches != nil {
		result = matches[1]
	}

	// Step 2: Remove inline backticks if entire output is wrapped
	// Only if the entire (trimmed) content is wrapped in single backticks
	trimmed := strings.TrimSpace(result)
	if matches := inlineBacktickRegex.FindStringSubmatch(trimmed); matches != nil {
		result = matches[1]
	}

	// Step 3: Strip leading blank lines
	// Split into lines, find first non-empty line, rejoin
	lines := strings.Split(result, "\n")
	firstNonEmpty := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstNonEmpty = i
			break
		}
	}

	// If all lines are empty/whitespace, return empty string
	if firstNonEmpty == -1 {
		return ""
	}

	lines = lines[firstNonEmpty:]

	// Step 4: Strip trailing blank lines
	lastNonEmpty := len(lines) - 1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastNonEmpty = i
			break
		}
	}
	if lastNonEmpty < len(lines)-1 {
		lines = lines[:lastNonEmpty+1]
	}

	// Rejoin lines
	result = strings.Join(lines, "\n")

	// Step 5: Remove "$ " prefix from first line if present
	// Only remove from the very start of the content
	if len(lines) > 0 {
		if dollarPrefixRegex.MatchString(lines[0]) {
			lines[0] = dollarPrefixRegex.ReplaceAllString(lines[0], "")
			result = strings.Join(lines, "\n")
		}
	}

	// Step 6: Final trim of leading/trailing whitespace on the entire result
	// BUT preserve internal structure (newlines, indentation)
	result = trimLeadingTrailingWhitespace(result)

	return result
}

// trimLeadingTrailingWhitespace removes leading whitespace from the first line
// and trailing whitespace from the last line, but preserves internal structure.
func trimLeadingTrailingWhitespace(s string) string {
	if s == "" {
		return s
	}

	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return s
	}

	// Trim leading whitespace from first line only
	lines[0] = strings.TrimLeft(lines[0], " \t")

	// Trim trailing whitespace from last line only
	lastIdx := len(lines) - 1
	lines[lastIdx] = strings.TrimRight(lines[lastIdx], " \t")

	return strings.Join(lines, "\n")
}

// CheckErrorSentinel checks if the command is an LLM error response.
// Returns true if the command matches the error sentinel format:
//
//	echo "QCMD_ERROR: <message>"
//
// Returns the error message if found, empty string otherwise.
func CheckErrorSentinel(cmd string) (bool, string) {
	// Trim the command for matching
	trimmed := strings.TrimSpace(cmd)

	// Check against the error sentinel regex
	if matches := errorSentinelRegex.FindStringSubmatch(trimmed); matches != nil {
		return true, strings.TrimSpace(matches[1])
	}

	return false, ""
}
