package sanitize

import (
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cleanup
		{
			name:     "code fence bash",
			input:    "```bash\nls -la\n```",
			expected: "ls -la",
		},
		{
			name:     "code fence plain",
			input:    "```\nls -la\n```",
			expected: "ls -la",
		},
		{
			name:     "code fence sh",
			input:    "```sh\necho hello\n```",
			expected: "echo hello",
		},
		{
			name:     "code fence shell",
			input:    "```shell\nfind . -name '*.go'\n```",
			expected: "find . -name '*.go'",
		},
		{
			name:     "code fence shell-session",
			input:    "```shell-session\nls -la\n```",
			expected: "ls -la",
		},
		{
			name:     "code fence console",
			input:    "```console\necho hello\n```",
			expected: "echo hello",
		},
		{
			name:     "code fence zsh",
			input:    "```zsh\necho $SHELL\n```",
			expected: "echo $SHELL",
		},
		{
			name:     "code fence fish",
			input:    "```fish\nset -x PATH /usr/bin\n```",
			expected: "set -x PATH /usr/bin",
		},
		{
			name:     "code fence powershell",
			input:    "```powershell\nGet-Process\n```",
			expected: "Get-Process",
		},
		{
			name:     "code fence language with number",
			input:    "```python3\nprint('hi')\n```",
			expected: "print('hi')",
		},
		{
			name:     "inline backticks",
			input:    "`ls -la`",
			expected: "ls -la",
		},
		{
			name:     "dollar prefix",
			input:    "$ ls -la",
			expected: "ls -la",
		},
		{
			name:     "dollar prefix with extra spaces",
			input:    "$   ls -la",
			expected: "ls -la",
		},
		{
			name:     "leading whitespace",
			input:    "  ls -la",
			expected: "ls -la",
		},
		{
			name:     "trailing whitespace",
			input:    "ls -la  ",
			expected: "ls -la",
		},
		{
			name:     "leading newlines",
			input:    "\n\n\nls -la",
			expected: "ls -la",
		},
		{
			name:     "trailing newlines",
			input:    "ls -la\n\n\n",
			expected: "ls -la",
		},
		{
			name:     "combined leading trailing whitespace",
			input:    "\n\n  ls -la  \n\n",
			expected: "ls -la",
		},
		{
			name:     "code fence with leading trailing whitespace",
			input:    "  ```bash\nls -la\n```  ",
			expected: "ls -la",
		},

		// Multi-line preservation (critical)
		{
			name: "multi-line preserved",
			input: `docker run \
  -v /data:/data \
  nginx`,
			expected: `docker run \
  -v /data:/data \
  nginx`,
		},
		{
			name: "multi-line docker full",
			input: `docker run \
  --name myapp \
  -v /data:/data \
  -p 8080:80 \
  nginx:latest`,
			expected: `docker run \
  --name myapp \
  -v /data:/data \
  -p 8080:80 \
  nginx:latest`,
		},
		{
			name: "pipeline multi-line",
			input: `find . -name '*.go' \
  | xargs grep TODO`,
			expected: `find . -name '*.go' \
  | xargs grep TODO`,
		},
		{
			name: "heredoc preserved",
			input: `cat <<EOF
hello
world
EOF`,
			expected: `cat <<EOF
hello
world
EOF`,
		},
		{
			name: "heredoc with variable",
			input: `cat <<'EOF'
$PATH
$HOME
EOF`,
			expected: `cat <<'EOF'
$PATH
$HOME
EOF`,
		},
		{
			name: "multi-line in code fence",
			input: "```bash\ndocker run \\\n  -v /data:/data \\\n  nginx\n```",
			expected: "docker run \\\n  -v /data:/data \\\n  nginx",
		},
		{
			name: "awk command preserved",
			input: `awk '{print $1, $2}' file.txt`,
			expected: `awk '{print $1, $2}' file.txt`,
		},
		{
			name: "complex pipeline",
			input: `ps aux | grep nginx | awk '{print $2}' | xargs kill`,
			expected: `ps aux | grep nginx | awk '{print $2}' | xargs kill`,
		},

		// No over-sanitization
		{
			name:     "internal spaces preserved",
			input:    "echo 'hello   world'",
			expected: "echo 'hello   world'",
		},
		{
			name:     "tabs preserved in content",
			input:    "echo '\t\t'",
			expected: "echo '\t\t'",
		},
		{
			name:     "internal backticks preserved",
			input:    "echo `date`",
			expected: "echo `date`",
		},
		{
			name:     "mixed quotes preserved",
			input:    `echo "it's" 'a "test"'`,
			expected: `echo "it's" 'a "test"'`,
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \n\n   ",
			expected: "",
		},
		{
			name:     "only code fence markers",
			input:    "```\n```",
			expected: "",
		},
		{
			name:     "nested backticks in code fence",
			input:    "```bash\necho `hostname`\n```",
			expected: "echo `hostname`",
		},
		{
			name:     "dollar in middle of command",
			input:    "echo $HOME",
			expected: "echo $HOME",
		},
		{
			name:     "backslash at end of single line",
			input:    "echo hello \\",
			expected: "echo hello \\",
		},

		// Real-world LLM output scenarios
		{
			name:     "simple command no formatting",
			input:    "ls -la",
			expected: "ls -la",
		},
		{
			name: "formatted command with explanation prefix stripped",
			input: `$ git status
`,
			expected: "git status",
		},
		{
			name: "code block with language and newlines",
			input: `

` + "```" + `bash
git add . && git commit -m "update"
` + "```" + `

`,
			expected: `git add . && git commit -m "update"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sanitize(tt.input)
			if got != tt.expected {
				t.Errorf("Sanitize() =\n%q\nwant:\n%q", got, tt.expected)
			}
		})
	}
}

func TestCheckErrorSentinel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		isError   bool
		message   string
	}{
		// Error sentinel patterns
		{
			name:    "double quote error",
			input:   `echo "QCMD_ERROR: unclear request"`,
			isError: true,
			message: "unclear request",
		},
		{
			name:    "single quote error",
			input:   `echo 'QCMD_ERROR: not possible'`,
			isError: true,
			message: "not possible",
		},
		{
			name:    "error with extra spaces",
			input:   `echo "QCMD_ERROR:   spaces in message"`,
			isError: true,
			message: "spaces in message",
		},
		{
			name:    "error with special chars",
			input:   `echo "QCMD_ERROR: can't find file"`,
			isError: true,
			message: "can't find file",
		},
		{
			name:    "error with leading/trailing whitespace in cmd",
			input:   `  echo "QCMD_ERROR: trimmed"  `,
			isError: true,
			message: "trimmed",
		},

		// Non-error patterns
		{
			name:    "regular echo",
			input:   `echo "hello world"`,
			isError: false,
			message: "",
		},
		{
			name:    "ls command",
			input:   `ls -la`,
			isError: false,
			message: "",
		},
		{
			name:    "echo with QCMD but not error",
			input:   `echo "QCMD is great"`,
			isError: false,
			message: "",
		},
		{
			name:    "partial match no colon",
			input:   `echo "QCMD_ERROR"`,
			isError: false,
			message: "",
		},
		{
			name:    "echo in middle of command",
			input:   `ls && echo "QCMD_ERROR: test"`,
			isError: false,
			message: "",
		},
		{
			name:    "empty string",
			input:   ``,
			isError: false,
			message: "",
		},
		{
			name:    "similar but different prefix",
			input:   `echo "ERROR: something failed"`,
			isError: false,
			message: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isErr, msg := CheckErrorSentinel(tt.input)
			if isErr != tt.isError {
				t.Errorf("CheckErrorSentinel(%q) isError = %v, want %v", tt.input, isErr, tt.isError)
			}
			if msg != tt.message {
				t.Errorf("CheckErrorSentinel(%q) message = %q, want %q", tt.input, msg, tt.message)
			}
		})
	}
}

func TestSanitizePreservesMultiLineCommands(t *testing.T) {
	// This is a critical test - multi-line commands must be preserved exactly
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "docker run with many flags",
			input: `docker run \
  --name mycontainer \
  --restart unless-stopped \
  -e DEBUG=true \
  -v /host/path:/container/path:ro \
  -p 8080:80 \
  -p 8443:443 \
  --network bridge \
  nginx:latest`,
		},
		{
			name: "curl with headers",
			input: `curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"key": "value"}' \
  https://api.example.com/endpoint`,
		},
		{
			name: "git commit with multiline message",
			input: `git commit -m "feat: add new feature

This implements the following:
- Feature A
- Feature B

Closes #123"`,
		},
		{
			name: "for loop",
			input: `for f in *.txt; do
  echo "Processing $f"
  cat "$f" | wc -l
done`,
		},
		{
			name: "if statement",
			input: `if [ -f "file.txt" ]; then
  echo "File exists"
else
  echo "File not found"
fi`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sanitize(tt.input)
			if got != tt.input {
				t.Errorf("Sanitize() modified multi-line command:\ninput:\n%s\ngot:\n%s", tt.input, got)
			}
		})
	}
}

func TestSanitizeCodeFenceEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "code fence without trailing newline",
			input:    "```bash\nls -la```",
			expected: "ls -la",
		},
		{
			name:     "code fence with extra newlines inside",
			input:    "```bash\n\nls -la\n\n```",
			expected: "ls -la",
		},
		{
			name:     "multiple code fences takes first",
			input:    "```bash\necho 1\n```\n```bash\necho 2\n```",
			expected: "echo 1\n```\n```bash\necho 2",
		},
		{
			name:     "unclosed code fence",
			input:    "```bash\nls -la",
			expected: "```bash\nls -la",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sanitize(tt.input)
			if got != tt.expected {
				t.Errorf("Sanitize() =\n%q\nwant:\n%q", got, tt.expected)
			}
		})
	}
}

// BenchmarkSanitize benchmarks the sanitize function.
func BenchmarkSanitize(b *testing.B) {
	inputs := []string{
		"ls -la",
		"```bash\nls -la\n```",
		"`echo hello`",
		"$ git status",
		"docker run \\\n  -v /data:/data \\\n  nginx",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			Sanitize(input)
		}
	}
}

// BenchmarkCheckErrorSentinel benchmarks the error sentinel check.
func BenchmarkCheckErrorSentinel(b *testing.B) {
	inputs := []string{
		`echo "QCMD_ERROR: test"`,
		`ls -la`,
		`echo "hello world"`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			CheckErrorSentinel(input)
		}
	}
}
