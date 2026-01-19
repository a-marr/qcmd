// Package safety provides deterministic safety checking for shell commands.
package safety

import "regexp"

// Pattern defines a danger pattern for command matching.
type Pattern struct {
	// Regex is the compiled regular expression.
	Regex *regexp.Regexp
	// Level is the danger level for this pattern.
	Level DangerLevel
	// Description is a human-readable explanation of the danger.
	Description string
	// Category is the type of danger (filesystem, network, system).
	Category string
}

// DangerPatterns contains patterns that should block command injection.
// These patterns match commands that could cause irreversible damage.
var DangerPatterns = []Pattern{
	{
		// Match rm with -r/-f flags followed by / or ~ or $HOME
		// Case insensitive for r/R and f/F flags
		Regex:       regexp.MustCompile(`(?i)rm\s+(-[rf]+\s+)*(/|~|\$HOME)(\s|$)`),
		Level:       Danger,
		Description: "Recursive delete on root or home directory",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`rm\s+(-[rRf]+\s+)*/\*(\s|$)`),
		Level:       Danger,
		Description: "Delete everything in root directory",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`rm\s+-[rRf]*[rRf][rRf]*\s+\*(\s|$)`),
		Level:       Danger,
		Description: "Delete all files in current directory with force/recursive flags",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`dd\s+.*of=/dev/[sh]d[a-z]+`),
		Level:       Danger,
		Description: "Direct disk write (dd to block device)",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`mkfs\.[a-z0-9]+\s+/dev/`),
		Level:       Danger,
		Description: "Filesystem format on a device",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`>\s*/dev/[sh]d[a-z]`),
		Level:       Danger,
		Description: "Redirect output to disk device",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`:\s*\(\s*\)\s*\{[^}]*:\s*\|\s*:`),
		Level:       Danger,
		Description: "Fork bomb pattern detected",
		Category:    "system",
	},
	{
		Regex:       regexp.MustCompile(`chmod\s+(-[rR]+\s+)*(000|777)\s+/(\s|$)`),
		Level:       Danger,
		Description: "Dangerous permission change on root filesystem",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`chown\s+(-[rR]+\s+)*.+\s+/(\s|$)`),
		Level:       Danger,
		Description: "Recursive ownership change on root filesystem",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`mv\s+/\s+`),
		Level:       Danger,
		Description: "Move root directory",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`cat\s+/dev/u?random\s*>\s*/dev/sd`),
		Level:       Danger,
		Description: "Write random data to disk device",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`>\s*/etc/(passwd|shadow)`),
		Level:       Danger,
		Description: "Overwrite authentication files",
		Category:    "system",
	},
}

// CautionPatterns contains patterns that should warn but allow execution.
// These patterns match commands that are potentially risky but may be legitimate.
var CautionPatterns = []Pattern{
	{
		Regex:       regexp.MustCompile(`sudo\s+`),
		Level:       Caution,
		Description: "Command requires elevated privileges",
		Category:    "system",
	},
	{
		Regex:       regexp.MustCompile(`curl\s+.*\|\s*(ba)?sh`),
		Level:       Caution,
		Description: "Piping remote script directly to shell",
		Category:    "network",
	},
	{
		Regex:       regexp.MustCompile(`wget\s+.*\|\s*(ba)?sh`),
		Level:       Caution,
		Description: "Piping remote script directly to shell",
		Category:    "network",
	},
	{
		Regex:       regexp.MustCompile(`eval\s+`),
		Level:       Caution,
		Description: "Dynamic command execution with eval",
		Category:    "system",
	},
	{
		Regex:       regexp.MustCompile(`rm\s+-[rRf]+\s+`),
		Level:       Caution,
		Description: "Recursive or forced file deletion",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`chmod\s+-[rR]+\s+`),
		Level:       Caution,
		Description: "Recursive permission change",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`chown\s+-[rR]+\s+`),
		Level:       Caution,
		Description: "Recursive ownership change",
		Category:    "filesystem",
	},
	{
		Regex:       regexp.MustCompile(`pkill\s+`),
		Level:       Caution,
		Description: "Kill processes by pattern",
		Category:    "system",
	},
	{
		Regex:       regexp.MustCompile(`killall\s+`),
		Level:       Caution,
		Description: "Kill all processes by name",
		Category:    "system",
	},
}

// ShellWrappers contains patterns for extracting nested commands.
// These patterns match shell constructs that wrap other commands.
var ShellWrappers = []string{
	`sudo\s+(.+)`,              // sudo <cmd>
	`sh\s+-c\s+["'](.+)["']`,   // sh -c "<cmd>"
	`bash\s+-c\s+["'](.+)["']`, // bash -c "<cmd>"
	`zsh\s+-c\s+["'](.+)["']`,  // zsh -c "<cmd>"
	`eval\s+["']?(.+?)["']?$`,  // eval <cmd>
}
