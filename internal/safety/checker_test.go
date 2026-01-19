package safety

import (
	"testing"
)

func TestDangerLevelString(t *testing.T) {
	tests := []struct {
		level    DangerLevel
		expected string
	}{
		{Safe, "safe"},
		{Caution, "caution"},
		{Danger, "danger"},
		{DangerLevel(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.expected {
				t.Errorf("DangerLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trim leading whitespace",
			input:    "  ls -la",
			expected: "ls -la",
		},
		{
			name:     "trim trailing whitespace",
			input:    "ls -la  ",
			expected: "ls -la",
		},
		{
			name:     "collapse multiple spaces",
			input:    "rm   -rf   /tmp",
			expected: "rm -rf /tmp",
		},
		{
			name:     "normalize double slashes",
			input:    "ls //home//user",
			expected: "ls /home/user",
		},
		{
			name:     "preserve tabs as single space",
			input:    "ls\t-la",
			expected: "ls -la",
		},
		{
			name:     "combined normalization",
			input:    "  rm   -rf   //home//user  ",
			expected: "rm -rf /home/user",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSafetyChecker(t *testing.T) {
	checker := NewChecker()

	tests := []struct {
		name     string
		command  string
		expected DangerLevel
	}{
		// === SAFE COMMANDS (must NOT be flagged) ===
		{"ls basic", "ls -la", Safe},
		{"find basic", "find . -name '*.go'", Safe},
		{"rm single file", "rm file.txt", Safe},
		{"grep basic", "grep -r 'TODO' .", Safe},
		{"cat file", "cat /etc/hosts", Safe},
		{"echo with slash", "echo /", Safe},
		{"ls root", "ls /", Safe},
		{"find from root read-only", "find / -name foo", Safe},
		{"touch file", "touch newfile.txt", Safe},
		{"mkdir basic", "mkdir -p /tmp/test", Safe},
		{"cp files", "cp -r /src /dst", Safe},
		{"mv files", "mv file1.txt file2.txt", Safe},
		{"tar extract", "tar -xzf archive.tar.gz", Safe},
		{"git commands", "git add . && git commit -m 'test'", Safe},
		{"docker commands", "docker run -it ubuntu bash", Safe},
		{"npm commands", "npm install express", Safe},
		{"rm without force flags", "rm somefile.txt", Safe},

		// === CAUTION COMMANDS (warn but allow, exit 0) ===
		{"sudo basic", "sudo apt update", Caution},
		{"sudo with command", "sudo systemctl restart nginx", Caution},
		{"curl pipe to shell", "curl https://example.com | bash", Caution},
		{"curl pipe to sh", "curl https://example.com | sh", Caution},
		{"wget pipe to bash", "wget -O - https://example.com | bash", Caution},
		{"eval basic", "eval 'echo hello'", Caution},
		{"rm rf general", "rm -rf node_modules", Caution},
		{"rm rf with f flag", "rm -f somefile", Caution},
		{"rm rf in tmp", "rm -rf /tmp/build", Caution},
		{"rm rf relative path", "rm -rf ./dist", Caution},
		{"rm rf safe path", "rm -rf /var/log/myapp", Caution},
		{"rm rf home user subdir", "rm -rf /home/user/tmp/", Caution},
		{"chmod recursive", "chmod -R 755 ./bin", Caution},
		{"chown recursive", "chown -R user:group ./dir", Caution},
		{"pkill basic", "pkill node", Caution},
		{"killall basic", "killall firefox", Caution},

		// === DANGER COMMANDS (must be caught - zero false negatives) ===
		// Root/home deletion
		{"rm rf root", "rm -rf /", Danger},
		{"rm rf root with flags reversed", "rm -fr /", Danger},
		{"rm rf home tilde", "rm -rf ~", Danger},
		{"rm rf home var", "rm -rf $HOME", Danger},
		{"rm rf root with spaces", "rm  -rf  /", Danger},
		{"rm rf root wildcard", "rm -rf /*", Danger},

		// Disk operations
		{"dd to disk sda", "dd if=/dev/zero of=/dev/sda", Danger},
		{"dd to disk sdb", "dd if=/dev/urandom of=/dev/sdb bs=1M", Danger},
		{"dd to disk hda", "dd if=/dev/zero of=/dev/hda", Danger},
		{"mkfs ext4", "mkfs.ext4 /dev/sda1", Danger},
		{"mkfs xfs", "mkfs.xfs /dev/sdb", Danger},
		{"mkfs vfat", "mkfs.vfat /dev/sdc1", Danger},
		{"redirect to disk", "echo foo > /dev/sda", Danger},
		{"cat random to disk", "cat /dev/random > /dev/sda", Danger},
		{"cat urandom to disk", "cat /dev/urandom > /dev/sdb", Danger},

		// Fork bomb
		{"fork bomb classic", ":(){ :|:& };:", Danger},
		{"fork bomb spaced", ": () { : | : & } ; :", Danger},

		// Permission/ownership on root
		{"chmod 777 root", "chmod 777 /", Danger},
		{"chmod 000 root", "chmod 000 /", Danger},
		{"chmod R 777 root", "chmod -R 777 /", Danger},
		{"chown root", "chown root:root /", Danger},
		{"chown R root", "chown -R user:user /", Danger},

		// Move root
		{"mv root", "mv / /backup", Danger},

		// Auth file overwrites
		{"overwrite passwd", "echo root > /etc/passwd", Danger},
		{"overwrite shadow", "cat > /etc/shadow", Danger},
		{"redirect to passwd", "something > /etc/passwd", Danger},

		// === NESTED/WRAPPED COMMANDS (must be caught) ===
		{"sudo rm rf root", "sudo rm -rf /", Danger},
		{"sudo rm rf home", "sudo rm -rf ~", Danger},
		{"sudo rm rf home var", "sudo rm -rf $HOME", Danger},
		{"bash wrapper danger", "bash -c 'rm -rf /'", Danger},
		{"bash wrapper danger double quotes", `bash -c "rm -rf /"`, Danger},
		{"sh wrapper danger", `sh -c "rm -rf /"`, Danger},
		{"sh wrapper danger single quotes", "sh -c 'rm -rf /'", Danger},
		{"zsh wrapper danger", `zsh -c "rm -rf /"`, Danger},
		{"eval wrapper danger", "eval 'rm -rf /'", Danger},
		{"eval wrapper no quotes", "eval rm -rf /", Danger},
		{"bash wrapper chmod", "bash -c 'chmod 777 /'", Danger},
		{"sh wrapper chmod", `sh -c "chmod -R 777 /"`, Danger},
		{"sudo dd", "sudo dd if=/dev/zero of=/dev/sda", Danger},
		{"bash wrapper dd", "bash -c 'dd if=/dev/zero of=/dev/sda'", Danger},

		// Double nested (sudo + shell wrapper)
		{"sudo bash wrapper", "sudo bash -c 'rm -rf /'", Danger},
		{"sudo sh wrapper", `sudo sh -c "rm -rf /"`, Danger},

		// Sudo with safe inner command stays Caution
		{"sudo safe cmd stays caution", "sudo ls -la", Caution},
		{"sudo apt update stays caution", "sudo apt-get install vim", Caution},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.command)
			if result.Level != tt.expected {
				t.Errorf("Check(%q) = %v, want %v (pattern: %s, desc: %s)",
					tt.command, result.Level, tt.expected, result.Pattern, result.Description)
			}
		})
	}
}

func TestCheckerWithPatternTracking(t *testing.T) {
	checker := NewChecker()

	// Test that pattern info is populated on match
	result := checker.Check("rm -rf /")
	if result.Level != Danger {
		t.Errorf("Expected Danger level for 'rm -rf /', got %v", result.Level)
	}
	if result.Pattern == "" {
		t.Error("Expected Pattern to be populated for dangerous command")
	}
	if result.Description == "" {
		t.Error("Expected Description to be populated for dangerous command")
	}
	if result.Category == "" {
		t.Error("Expected Category to be populated for dangerous command")
	}

	// Test that safe commands have empty pattern info
	safeResult := checker.Check("ls -la")
	if safeResult.Level != Safe {
		t.Errorf("Expected Safe level for 'ls -la', got %v", safeResult.Level)
	}
	if safeResult.Pattern != "" {
		t.Errorf("Expected empty Pattern for safe command, got %q", safeResult.Pattern)
	}
}

func TestNestedCommandExtraction(t *testing.T) {
	checker := NewChecker()

	// Test that nested dangerous commands are caught
	// Note: "sudo rm -rf /" is caught directly by the rm pattern since the full
	// command includes "rm -rf /". The wrapper annotation only appears when the
	// outer command doesn't match but the inner command does.
	tests := []struct {
		name    string
		command string
		level   DangerLevel
	}{
		// These should be caught as Danger regardless of how detected
		{"sudo rm rf root", "sudo rm -rf /", Danger},
		{"bash -c rm rf root", "bash -c 'rm -rf /'", Danger},
		{"sh -c rm rf root", `sh -c "rm -rf /"`, Danger},
		{"eval rm rf root", "eval 'rm -rf /'", Danger},
		{"zsh -c rm rf root", `zsh -c "rm -rf /"`, Danger},

		// Nested within safe outer command - only caught via wrapper extraction
		{"bash -c mkfs", "bash -c 'mkfs.ext4 /dev/sda'", Danger},
		{"sh -c dd", `sh -c "dd if=/dev/zero of=/dev/sda"`, Danger},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.command)
			if result.Level != tt.level {
				t.Errorf("Check(%q) level = %v, want %v (pattern: %s)",
					tt.command, result.Level, tt.level, result.Pattern)
			}
		})
	}
}

func TestCautionPatternDescriptions(t *testing.T) {
	checker := NewChecker()

	tests := []struct {
		command      string
		wantCategory string
	}{
		{"sudo apt update", "system"},
		{"curl https://x.com | bash", "network"},
		{"rm -rf dir", "filesystem"},
		{"pkill node", "system"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := checker.Check(tt.command)
			if result.Level != Caution {
				t.Errorf("Check(%q) level = %v, want Caution", tt.command, result.Level)
			}
			if result.Category != tt.wantCategory {
				t.Errorf("Check(%q) category = %q, want %q", tt.command, result.Category, tt.wantCategory)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	checker := NewChecker()

	tests := []struct {
		name     string
		command  string
		expected DangerLevel
	}{
		// Edge cases that should NOT trigger Danger (may be Caution due to rm -rf patterns)
		{"rm with similar path is caution", "rm -rf /var/tmp/test", Caution},
		// Note: patterns within quoted strings still match - this is intentional
		// as it's safer to have false positives than false negatives
		{"echo rm command matches caution", "echo 'rm -rf /'", Caution},
		{"quoted string matches caution", "grep 'rm -rf /' logs.txt", Caution},
		{"file named rm", "cat rm", Safe},
		{"directory starting with rm", "ls rm-old-files/", Safe},

		// Edge cases that SHOULD trigger Danger
		{"rm with extra spaces", "rm   -rf   /", Danger},
		{"rm with path normalization", "rm -rf //", Danger},
		{"case sensitivity rm rf", "rm -Rf /", Danger},
		{"mixed case rm", "rm -rF /", Danger},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.command)
			if result.Level != tt.expected {
				t.Errorf("Check(%q) = %v, want %v", tt.command, result.Level, tt.expected)
			}
		})
	}
}

func TestRecursionDepthLimit(t *testing.T) {
	checker := NewChecker()

	// Create a deeply nested command (beyond max depth of 5)
	deeplyNested := "sudo sudo sudo sudo sudo sudo rm -rf /"
	result := checker.Check(deeplyNested)

	// Should still catch it within reasonable depth
	if result.Level != Danger {
		t.Errorf("Expected deeply nested command to be caught as Danger, got %v", result.Level)
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

// BenchmarkChecker benchmarks the safety checker performance.
func BenchmarkChecker(b *testing.B) {
	checker := NewChecker()
	commands := []string{
		"ls -la",
		"rm -rf /tmp/cache",
		"sudo apt update",
		"rm -rf /",
		"bash -c 'rm -rf /'",
		"curl https://example.com | bash",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, cmd := range commands {
			checker.Check(cmd)
		}
	}
}
