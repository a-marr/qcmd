// Package safety provides deterministic safety checking for shell commands.
package safety

import (
	"regexp"
	"strings"
)

// DangerLevel represents the severity of a command's potential risk.
type DangerLevel int

const (
	// Safe indicates no issues detected.
	Safe DangerLevel = iota
	// Caution indicates potentially risky operations (sudo, curl|sh).
	Caution
	// Danger indicates definitely dangerous operations (rm -rf /, fork bomb).
	Danger
)

// String returns the string representation of the danger level.
func (d DangerLevel) String() string {
	switch d {
	case Safe:
		return "safe"
	case Caution:
		return "caution"
	case Danger:
		return "danger"
	default:
		return "unknown"
	}
}

// CheckResult contains the result of a safety check.
type CheckResult struct {
	// Level is the determined danger level.
	Level DangerLevel
	// Pattern is which pattern matched (for debugging).
	Pattern string
	// Description is a human-readable explanation.
	Description string
	// Category is the type of danger (filesystem, network, system).
	Category string
}

// Checker performs safety checks on shell commands.
type Checker struct {
	// dangerPatterns contains the registered danger patterns.
	dangerPatterns []Pattern
	// cautionPatterns contains the registered caution patterns.
	cautionPatterns []Pattern
	// shellWrappers contains compiled regexes for extracting nested commands.
	shellWrappers []*regexp.Regexp
}

// NewChecker creates a new Checker with the default pattern registry.
func NewChecker() *Checker {
	// Compile shell wrapper patterns
	wrappers := make([]*regexp.Regexp, 0, len(ShellWrappers))
	for _, pattern := range ShellWrappers {
		wrappers = append(wrappers, regexp.MustCompile(pattern))
	}

	return &Checker{
		dangerPatterns:  DangerPatterns,
		cautionPatterns: CautionPatterns,
		shellWrappers:   wrappers,
	}
}

// Check analyzes a command and returns the safety check result.
// It handles command normalization and nested command extraction.
func (c *Checker) Check(cmd string) CheckResult {
	normalized := Normalize(cmd)

	// First, check the full command against danger patterns
	result := c.checkPatterns(normalized)
	if result.Level == Danger {
		return result
	}

	// Extract and check nested commands recursively
	nestedResult := c.checkNestedCommands(normalized, 0)
	if nestedResult.Level > result.Level {
		return nestedResult
	}

	// If no danger found, check the full command against caution patterns
	if result.Level == Safe {
		result = c.checkCautionPatterns(normalized)
	}

	return result
}

// checkPatterns checks a command against danger patterns only.
func (c *Checker) checkPatterns(cmd string) CheckResult {
	for _, pattern := range c.dangerPatterns {
		if pattern.Regex.MatchString(cmd) {
			return CheckResult{
				Level:       pattern.Level,
				Pattern:     pattern.Regex.String(),
				Description: pattern.Description,
				Category:    pattern.Category,
			}
		}
	}

	return CheckResult{Level: Safe}
}

// checkCautionPatterns checks a command against caution patterns.
func (c *Checker) checkCautionPatterns(cmd string) CheckResult {
	for _, pattern := range c.cautionPatterns {
		if pattern.Regex.MatchString(cmd) {
			return CheckResult{
				Level:       pattern.Level,
				Pattern:     pattern.Regex.String(),
				Description: pattern.Description,
				Category:    pattern.Category,
			}
		}
	}

	return CheckResult{Level: Safe}
}

// checkNestedCommands extracts and checks commands inside shell wrappers.
// It supports recursive checking up to a maximum depth to prevent infinite loops.
func (c *Checker) checkNestedCommands(cmd string, depth int) CheckResult {
	// Prevent infinite recursion (max depth of 5)
	const maxDepth = 5
	if depth >= maxDepth {
		return CheckResult{Level: Safe}
	}

	var highestResult CheckResult
	highestResult.Level = Safe

	for _, wrapper := range c.shellWrappers {
		matches := wrapper.FindStringSubmatch(cmd)
		if len(matches) >= 2 {
			innerCmd := strings.TrimSpace(matches[1])
			if innerCmd == "" {
				continue
			}

			// Normalize and check the inner command
			normalizedInner := Normalize(innerCmd)

			// Check inner command against danger patterns
			innerResult := c.checkPatterns(normalizedInner)
			if innerResult.Level == Danger {
				innerResult.Pattern = innerResult.Pattern + " (via wrapper)"
				return innerResult
			}

			// Recursively check for nested wrappers
			nestedResult := c.checkNestedCommands(normalizedInner, depth+1)
			if nestedResult.Level > highestResult.Level {
				highestResult = nestedResult
			}

			// Update highest result if inner result is higher
			if innerResult.Level > highestResult.Level {
				highestResult = innerResult
			}
		}
	}

	return highestResult
}

// Normalize prepares a command for pattern matching.
// It performs the following transformations:
// 1. Trim leading/trailing whitespace
// 2. Collapse multiple spaces to single space
// 3. Normalize path separators: // -> /
func Normalize(cmd string) string {
	// Trim leading and trailing whitespace
	cmd = strings.TrimSpace(cmd)

	// Collapse multiple spaces to single space
	spaceRegex := regexp.MustCompile(`\s+`)
	cmd = spaceRegex.ReplaceAllString(cmd, " ")

	// Normalize path separators (// -> /)
	for strings.Contains(cmd, "//") {
		cmd = strings.ReplaceAll(cmd, "//", "/")
	}

	return cmd
}
