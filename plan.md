# qcmd - Technical Specification & Implementation Plan

## Executive Summary

qcmd is a CLI tool that bridges natural language and shell commands. Users describe what they want in their editor, and qcmd returns a ready-to-execute command via LLM, with safety checks and flexible output methods.

---

## 1. System Architecture

### 1.1 High-Level Flow

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Trigger   │───▶│   Editor    │───▶│  LLM Call   │───▶│  Sanitize   │───▶│   Error     │
│   $ q       │    │  ($EDITOR)  │    │  (backend)  │    │  (cleanup)  │    │  Sentinel   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └──────┬──────┘
                                                                                    │
                                                         ┌─────────────┐            │
                                                         │   Output    │◀───────────┘
                                                         │   Router    │◀──┐
                                                         └──────┬──────┘   │
                                                                │          │
                                                                ▼          │
                                                         ┌─────────────┐   │
                                                         │   Safety    │───┘
                                                         │   Check     │ (exit 3 if danger)
                                                         └─────────────┘
```

### 1.2 Output Protocol

**Strict separation of concerns:**
- **stdout:** Command only (for ZLE capture) — no prefixes, no messages
- **stderr:** All diagnostics, warnings, errors (human-readable)
- **Exit codes:** Signal status to wrapper

| Exit Code | Meaning | Wrapper Behavior |
|-----------|---------|------------------|
| 0 | Success, safe to inject | `print -z` the command |
| 1 | User/input error (empty query, bad config, LLM error) | Print stderr, abort |
| 2 | System/API error (network, auth, timeout) | Print stderr, abort |
| 3 | Dangerous command (blocked) | Print command to terminal, do NOT inject |

### 1.3 Directory Structure

```
qcmd/
├── cmd/
│   └── qcmd/
│       └── main.go                 # Entry point, CLI flags, orchestration
├── internal/
│   ├── backend/
│   │   ├── backend.go              # Backend interface definition
│   │   ├── anthropic.go            # Anthropic Claude API client
│   │   ├── openai.go               # OpenAI API client
│   │   ├── openrouter.go           # OpenRouter API client
│   │   └── backend_test.go         # Backend tests with mocked HTTP
│   ├── config/
│   │   ├── config.go               # TOML config loading, defaults, env override
│   │   └── config_test.go
│   ├── editor/
│   │   ├── editor.go               # Temp file creation, $EDITOR invocation
│   │   └── editor_test.go
│   ├── safety/
│   │   ├── checker.go              # Danger pattern matching + normalization
│   │   ├── patterns.go             # Pattern registry (separated for clarity)
│   │   └── checker_test.go         # Extensive table-driven tests
│   ├── sanitize/
│   │   ├── sanitize.go             # Strip markdown, backticks; preserve structure
│   │   └── sanitize_test.go
│   ├── output/
│   │   ├── output.go               # Output mode router
│   │   ├── clipboard.go            # Cross-platform clipboard
│   │   └── output_test.go
│   └── shellctx/
│       ├── shellctx.go             # Gather shell context (pwd, shell, etc.)
│       └── shellctx_test.go
├── shell/
│   └── qcmd.zsh                    # Zsh function wrapper for ZLE integration
├── testdata/
│   └── ...                         # Test fixtures
├── go.mod
├── go.sum
├── Makefile
├── CLAUDE.md
├── plan.md
├── README.md
└── LICENSE
```

---

## 2. Component Specifications

### 2.1 Backend Interface (`internal/backend/`)

#### Interface Definition

```go
// Backend defines the contract for LLM providers
type Backend interface {
    // GenerateCommand sends a query to the LLM and returns a shell command
    GenerateCommand(ctx context.Context, request *Request) (*Response, error)

    // Name returns the backend identifier for logging/debugging
    Name() string
}

type Request struct {
    Query       string            // User's natural language query
    Context     *ShellContext     // Optional: pwd, shell type, OS
    Model       string            // Model override (optional)
}

type Response struct {
    Command     string            // The generated shell command
    Model       string            // Model that was used
    TokensUsed  int               // For cost tracking (optional)
}

type ShellContext struct {
    WorkingDir  string
    Shell       string            // e.g., "zsh", "bash"
    OS          string            // e.g., "darwin", "linux"
}
```

#### System Prompt (Shared)

```
You are a shell command generator. Your ONLY job is to output a valid shell command.

Rules:
1. Output ONLY the raw shell command - no explanation, no markdown, no code fences
2. Do not include any text before or after the command
3. If multiple commands are needed, chain them with && or ;
4. For complex commands, use proper line continuation with backslashes
5. If the request is unclear or impossible, output exactly: echo "QCMD_ERROR: <brief reason>"
6. If the request would require dangerous operations, still provide the command (the tool handles safety)

Context provided:
- Working directory: {{.WorkingDir}}
- Shell: {{.Shell}}
- OS: {{.OS}}
```

#### Anthropic Backend

- **Endpoint:** `https://api.anthropic.com/v1/messages`
- **Headers:** `x-api-key`, `anthropic-version: 2023-06-01`
- **Default model:** `claude-4-haiku` (fast, cheap)
- **Max tokens:** 512 (allow room for multi-line commands)
- **Timeout:** 30 seconds (exit code 2 on timeout)
- **Model selection:** Any valid Anthropic model ID (e.g., `claude-4-haiku`, `claude-4-sonnet`, `claude-4-opus`)

#### OpenAI Backend

- **Endpoint:** `https://api.openai.com/v1/chat/completions`
- **Headers:** `Authorization: Bearer $KEY`
- **Default model:** `gpt-5o`
- **Max tokens:** 512
- **Timeout:** 30 seconds
- **Model selection:** Any valid OpenAI model ID (e.g., `gpt-5o`, `gpt-5o-mini`, `gpt-4o`)

#### OpenRouter Backend

- **Endpoint:** `https://openrouter.ai/api/v1/chat/completions`
- **Headers:** `Authorization: Bearer $KEY`, `HTTP-Referer`, `X-Title`
- **Default model:** `anthropic/claude-4-haiku` (configurable to any)
- **Max tokens:** 512
- **Timeout:** 30 seconds
- **Model selection:** Any model available on OpenRouter (e.g., `anthropic/claude-4-haiku`, `openai/gpt-5o`, `meta-llama/llama-3-70b`)

### 2.2 Config (`internal/config/`)

#### Config File Location

Priority (highest to lowest):
1. `--config` flag
2. `$QCMD_CONFIG` env var
3. `$XDG_CONFIG_HOME/qcmd/config.toml`
4. `~/.config/qcmd/config.toml`

#### TOML Schema

```toml
# Default backend to use
backend = "anthropic"  # anthropic | openai | openrouter

# Include shell context in prompts
include_context = true

# Output mode preference (when run directly, not via shell wrapper)
# "auto" = try clipboard, then print
# "clipboard" = always clipboard
# "print" = always print
output_mode = "auto"

[anthropic]
api_key = ""  # or use ANTHROPIC_API_KEY env var
model = "claude-4-haiku"  # any valid Anthropic model

[openai]
api_key = ""  # or use OPENAI_API_KEY env var
model = "gpt-5o"  # any valid OpenAI model

[openrouter]
api_key = ""  # or use OPENROUTER_API_KEY env var
model = "anthropic/claude-4-haiku"  # any model on OpenRouter

[safety]
# Block dangerous commands from being injected (still prints them)
block_dangerous = true
# Show warnings for cautionary commands
show_warnings = true

[editor]
# Override $EDITOR/$VISUAL
# editor = "nvim"

[advanced]
timeout_seconds = 30
max_tokens = 512
```

#### Environment Variable Overrides

| Config Key | Env Var |
|------------|---------|
| `anthropic.api_key` | `ANTHROPIC_API_KEY` |
| `openai.api_key` | `OPENAI_API_KEY` |
| `openrouter.api_key` | `OPENROUTER_API_KEY` |
| `backend` | `QCMD_BACKEND` |
| config path | `QCMD_CONFIG` |

### 2.3 Editor (`internal/editor/`)

#### Responsibilities

1. Create secure temp file (`os.CreateTemp` with `0600` perms)
2. Optionally pre-populate with template/hints
3. Launch `$VISUAL` or `$EDITOR` (fallback: `vi`)
   - Handle editors with arguments (e.g., `EDITOR="code --wait"`)
4. Wait for editor to exit
5. Read and return file contents
6. Clean up temp file

#### Interface

```go
type Editor struct {
    EditorCmd string  // Override for editor command
}

// GetInput opens the editor and returns user input.
// Returns empty string if file is empty or only comments.
// Returns error if editor fails to launch.
func (e *Editor) GetInput(ctx context.Context) (string, error)
```

#### Input Processing

```go
func ProcessInput(raw string) string {
    // 1. Split into lines
    // 2. Remove lines starting with # (comments)
    // 3. Trim leading/trailing blank lines
    // 4. Return remaining content (may be multi-line)
}
```

#### Temp File Template (Optional)

```
# Describe the shell command you need
# Lines starting with # are ignored
# Save and quit when done (:wq in vim)

```

### 2.4 Safety Checker (`internal/safety/`)

#### Danger Levels

```go
type DangerLevel int

const (
    Safe     DangerLevel = iota  // No issues detected
    Caution                       // Potentially risky (sudo, curl|sh)
    Danger                        // Definitely dangerous (rm -rf /, fork bomb)
)

type CheckResult struct {
    Level       DangerLevel
    Pattern     string    // Which pattern matched (for debugging)
    Description string    // Human-readable explanation
    Category    string    // "filesystem", "network", "system"
}
```

#### Pre-check Normalization

Before pattern matching, normalize the command:

```go
func Normalize(cmd string) string {
    // 1. Trim leading/trailing whitespace
    // 2. Collapse multiple spaces to single space
    // 3. Normalize path separators: // → /
    // 4. Expand common aliases/shortcuts if detectable
    return normalized
}
```

#### Pattern Registry

```go
type Pattern struct {
    Regex       *regexp.Regexp
    Level       DangerLevel
    Description string
    Category    string  // "filesystem", "network", "system", etc.
}
```

#### Pattern Categories

**DANGER (blocks injection, exit code 3):**

All patterns are anchored to prevent false positives on partial matches.

| Pattern | Description |
|---------|-------------|
| `rm\s+(-[rf]+\s+)*(\/|~|\$HOME)(\s|$)` | Recursive delete on root/home |
| `rm\s+(-[rf]+\s+)*\/\*(\s|$)` | Delete everything in root |
| `rm\s+(-[rf]+\s+)*\*(\s|$)` | Delete all in current dir with force |
| `dd\s+.*of=/dev/[sh]d[a-z]*(\s|$)` | Direct disk write |
| `mkfs\.[a-z0-9]+\s+/dev/` | Filesystem format |
| `>\s*/dev/[sh]d[a-z]` | Redirect to disk device |
| `:\s*\(\s*\)\s*\{[^}]*:\s*\|\s*:` | Fork bomb pattern |
| `chmod\s+(-R\s+)*(000|777)\s+\/(\s|$)` | Dangerous permission change on root |
| `chown\s+(-R\s+).*\s+\/(\s|$)` | Recursive chown on root |
| `mv\s+\/\s+` | Move root directory |
| `cat\s*/dev/u?random\s*>\s*/dev/sd` | Random to disk |
| `>\s*/etc/(passwd|shadow)` | Overwrite auth files |

**CAUTION (warns via stderr but allows, exit code 0):**

| Pattern | Description |
|---------|-------------|
| `sudo\s+` | Elevated privileges |
| `curl.*\|\s*(ba)?sh` | Pipe remote script to shell |
| `wget.*\|\s*(ba)?sh` | Pipe remote script to shell |
| `eval\s+` | Dynamic execution |
| `rm\s+-[rf]*\s+` | Recursive delete (non-critical path) |
| `chmod\s+-R\s+` | Recursive permission change |
| `chown\s+-R\s+` | Recursive ownership change |
| `pkill\s+` | Kill processes by pattern |
| `killall\s+` | Kill processes by name |

#### Nested Command Detection

Commands can be obfuscated via shell wrappers. The safety checker must extract and check inner commands:

```go
// Shell wrapper patterns that contain nested commands
var shellWrappers = []string{
    `sudo\s+(.+)`,           // sudo <cmd>
    `sh\s+-c\s+["'](.+)["']`,    // sh -c "<cmd>"
    `bash\s+-c\s+["'](.+)["']`,  // bash -c "<cmd>"
    `zsh\s+-c\s+["'](.+)["']`,   // zsh -c "<cmd>"
    `eval\s+["']?(.+)["']?`,     // eval <cmd>
}

func (c *Checker) Check(cmd string) CheckResult {
    // 1. Check the full command against all patterns
    result := c.checkPatterns(cmd)
    if result.Level == Danger {
        return result
    }

    // 2. Extract and check nested commands
    for _, wrapper := range shellWrappers {
        if inner := extractInnerCommand(cmd, wrapper); inner != "" {
            innerResult := c.checkPatterns(inner)
            if innerResult.Level > result.Level {
                result = innerResult
                result.Pattern = fmt.Sprintf("%s (via wrapper)", result.Pattern)
            }
        }
    }

    return result
}
```

This catches:
- `sudo rm -rf /` → extracts `rm -rf /` → Danger
- `bash -c "rm -rf /"` → extracts `rm -rf /` → Danger
- `sh -c 'chmod 777 /'` → extracts `chmod 777 /` → Danger

#### Test Cases (Critical)

```go
// Must be Safe (no false positives)
{"rm -rf /tmp/cache", Safe},           // /tmp is safe
{"rm -rf ./build", Safe},              // relative path
{"rm file.txt", Safe},                 // single file
{"sudo apt update", Caution},          // sudo is caution, not danger
{"find / -name foo", Safe},            // read-only

// Must be Danger
{"rm -rf /", Danger},
{"rm -rf ~", Danger},
{"rm -rf /*", Danger},
{"rm -rf $HOME", Danger},
{":(){ :|:& };:", Danger},
{"dd if=/dev/zero of=/dev/sda", Danger},
{"mkfs.ext4 /dev/sda1", Danger},
{"chmod -R 777 /", Danger},

// Nested/wrapped commands - Must be Danger
{"sudo rm -rf /", Danger},             // sudo + dangerous
{"sudo rm -rf ~", Danger},             // sudo + dangerous
{"sh -c \"rm -rf /\"", Danger},        // shell wrapper
{"bash -c 'rm -rf /'", Danger},        // bash wrapper
{"bash -c 'chmod 777 /'", Danger},     // nested chmod
{"eval 'rm -rf /'", Danger},           // eval wrapper
```

### 2.5 Sanitizer (`internal/sanitize/`)

LLM outputs may include unwanted formatting. Clean it while **preserving command structure**.

#### Operations

```go
func Sanitize(raw string) string {
    // 1. Remove markdown code fences: ```bash ... ``` or ``` ... ```
    //    - Handle fences at start/end of output
    //    - Preserve content between fences

    // 2. Remove inline backticks: `command` → command
    //    - Only if entire output is wrapped in single backticks

    // 3. Remove "$ " prefix from first line if present

    // 4. Strip LEADING blank lines and whitespace

    // 5. Strip TRAILING blank lines and whitespace

    // 6. PRESERVE internal newlines and structure
    //    - Multi-line commands (with \ continuation) stay multi-line
    //    - Pipelines can span multiple lines
    //    - Heredocs preserved

    return cleaned
}
```

#### Multi-line Preservation Examples

Input from LLM:
```
docker run \
  --name myapp \
  -v /data:/data \
  -p 8080:80 \
  nginx:latest
```

Output after sanitization (SAME - preserved):
```
docker run \
  --name myapp \
  -v /data:/data \
  -p 8080:80 \
  nginx:latest
```

### 2.6 Error Sentinel Detection (`internal/sanitize/`)

After sanitization, detect if LLM returned an error response:

```go
func CheckErrorSentinel(cmd string) (bool, string) {
    // Check for our specific error format
    if strings.HasPrefix(cmd, `echo "QCMD_ERROR:`) ||
       strings.HasPrefix(cmd, `echo 'QCMD_ERROR:`) {
        // Extract error message
        msg := extractErrorMessage(cmd)
        return true, msg
    }
    return false, ""
}
```

If error sentinel detected:
- Print error to stderr: `LLM could not generate command: <reason>`
- Exit with code 1
- Do NOT output anything to stdout

### 2.7 Output Router (`internal/output/`)

#### Output Modes

```go
type Mode int

const (
    ModeZLE       Mode = iota  // Output raw command to stdout (for shell wrapper)
    ModeClipboard              // Copy to clipboard, message to stderr
    ModePrint                  // Print command to stdout with newline
    ModeAuto                   // Try clipboard, fallback to print
)
```

#### Mode Selection

The output mode is **explicitly specified**, not auto-detected:

| Flag | Mode | Behavior |
|------|------|----------|
| `--output=zle` | ModeZLE | Raw command to stdout, no newline. Used by shell wrapper. |
| `--output=clipboard` | ModeClipboard | Copy to clipboard, print confirmation to stderr. |
| `--output=print` | ModePrint | Print command to stdout with newline. |
| `--output=auto` (default) | ModeAuto | Try clipboard; if unavailable, print. |

**ZLE mode is never auto-selected.** The shell wrapper explicitly passes `--output=zle`.

#### Clipboard Integration

```go
func CopyToClipboard(text string) error {
    var cmd *exec.Cmd

    switch runtime.GOOS {
    case "darwin":
        cmd = exec.Command("pbcopy")
    case "linux":
        // Try in order: wl-copy (Wayland), xclip, xsel
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

func hasCommand(name string) bool {
    _, err := exec.LookPath(name)
    return err == nil
}
```

### 2.8 Shell Integration (`shell/qcmd.zsh`)

```zsh
# qcmd.zsh - Source this in your .zshrc
# Usage: q
#
# The q function opens your editor, sends your query to an LLM,
# and places the resulting command in your shell buffer ready to execute.

function q() {
    local query_file
    query_file=$(mktemp) || { echo "qcmd: failed to create temp file" >&2; return 1; }

    # Open editor for user input
    ${VISUAL:-${EDITOR:-vi}} "$query_file"

    # Check if user wrote anything (non-empty after removing blank lines)
    if [[ ! -s "$query_file" ]] || ! grep -q '[^[:space:]]' "$query_file"; then
        rm -f "$query_file"
        return 0
    fi

    # Call qcmd binary with explicit ZLE output mode
    # stdout = command only, stderr = diagnostics (passed through to terminal)
    local cmd
    cmd=$(qcmd --query-file "$query_file" --output=zle)
    local exit_code=$?

    rm -f "$query_file"

    case $exit_code in
        0)
            # Success - inject command into ZLE buffer
            # User can review and press Enter to execute
            print -z "$cmd"
            ;;
        1)
            # User/input error - stderr already printed by qcmd
            return 1
            ;;
        2)
            # API/system error - stderr already printed by qcmd
            return 2
            ;;
        3)
            # Dangerous command - print but don't inject
            echo "" >&2
            echo "⚠️  Command blocked from injection (safety check triggered)" >&2
            echo "Review the command below. Copy manually if intended:" >&2
            echo "" >&2
            echo "$cmd"
            echo "" >&2
            return 3
            ;;
        *)
            echo "qcmd: unexpected exit code $exit_code" >&2
            return $exit_code
            ;;
    esac
}

# Optional: ZLE widget for direct keybind (uncomment to enable)
# This allows triggering qcmd with a key combo instead of typing 'q'
#
# function _qcmd_widget() {
#     zle -I  # Invalidate display
#     q       # Call the q function
#     zle reset-prompt
# }
# zle -N _qcmd_widget
# bindkey '^Q' _qcmd_widget  # Ctrl+Q (change as desired)
```

---

## 3. CLI Interface

### 3.1 Commands and Flags

```
qcmd - Natural language to shell command

Usage:
  qcmd [flags]
  qcmd [command]

Flags:
  --query-file <path>    Read query from file (used by shell wrapper)
  --query <string>       Direct query string (alternative to editor)
  --backend <name>       Override backend (anthropic|openai|openrouter)
  --model <name>         Override model
  --output <mode>        Output mode: zle|clipboard|print|auto (default: auto)
  --no-safety            Disable safety checks (use with caution)
  --config <path>        Config file path
  --verbose              Verbose output to stderr (for debugging)
  --version              Print version and exit
  --help                 Print help and exit

Input Precedence (highest to lowest):
  1. --query-file (if provided, file is read)
  2. --query (if provided and no --query-file)
  3. Interactive editor (if neither flag provided)

If both --query and --query-file are provided, --query-file takes precedence
and --query is ignored (with a warning to stderr if --verbose is set).

Commands:
  config                 Show current configuration
  config init            Create default config file with comments
  backends               List available backends and their status
```

#### `config init` Behavior

```go
func initConfig() error {
    configDir := getConfigDir() // ~/.config/qcmd or $XDG_CONFIG_HOME/qcmd

    // Create directory with secure permissions if it doesn't exist
    if err := os.MkdirAll(configDir, 0700); err != nil {
        return fmt.Errorf("failed to create config directory: %w", err)
    }

    configPath := filepath.Join(configDir, "config.toml")

    // Don't overwrite existing config
    if _, err := os.Stat(configPath); err == nil {
        return fmt.Errorf("config file already exists: %s", configPath)
    }

    // Write default config with 0600 permissions
    if err := os.WriteFile(configPath, []byte(defaultConfigTOML), 0600); err != nil {
        return fmt.Errorf("failed to write config file: %w", err)
    }

    fmt.Fprintf(os.Stderr, "Created config file: %s\n", configPath)
    return nil
}
```

### 3.2 Exit Codes

| Code | Meaning | stdout | stderr |
|------|---------|--------|--------|
| 0 | Success | Command | Warnings (if any) |
| 1 | User/input error | Empty | Error message |
| 2 | API/system error | Empty | Error message |
| 3 | Dangerous command blocked | Command | Warning |

### 3.3 Input Validation

The binary validates input before proceeding:

```go
func validateInput(query string) error {
    // Check for empty/whitespace-only input
    if strings.TrimSpace(query) == "" {
        return fmt.Errorf("empty query")
    }

    // Check for null bytes (security)
    if strings.ContainsRune(query, 0) {
        return fmt.Errorf("invalid input: contains null bytes")
    }

    // Check reasonable length (prevent abuse)
    if len(query) > 10000 {
        return fmt.Errorf("query too long (max 10000 bytes)")
    }

    return nil
}
```

---

## 4. Security Considerations

### 4.1 API Key Security

- Config file permissions checked: warn to stderr if not `0600`
- Keys never logged or printed
- Keys never included in error messages
- Env vars take precedence (for CI/CD, secrets managers)

### 4.2 Input Validation

- Query input validated (no null bytes, reasonable length)
- Config values validated (valid backend names, valid URLs)
- File paths validated (no directory traversal)

### 4.3 Output Safety

- All LLM output sanitized before use
- Dangerous commands blocked from shell injection (exit 3)
- User always sees command before execution (no auto-execute)
- Multi-line commands preserved for readability/inspection

### 4.4 Temp File Security

- Created with `0600` permissions
- Deleted immediately after reading (in defer)
- Use `os.CreateTemp` (secure random naming)
- Created in system temp dir (not in cwd)

---

## 5. Testing Strategy

### 5.1 Unit Tests

Each package has `*_test.go` with:
- Table-driven tests for comprehensive coverage
- Edge cases explicitly tested
- Mocks for external dependencies

### 5.2 Backend Tests

```go
// backend_test.go
func TestAnthropicBackend(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request headers
        if r.Header.Get("x-api-key") == "" {
            t.Error("missing API key header")
        }
        // Verify request body structure
        // Return mock response
        json.NewEncoder(w).Encode(mockResponse)
    }))
    defer server.Close()

    backend := NewAnthropicBackend(WithBaseURL(server.URL), WithAPIKey("test-key"))
    resp, err := backend.GenerateCommand(context.Background(), &Request{
        Query: "list files",
    })
    // Assert...
}

func TestAnthropicBackend_Timeout(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(100 * time.Millisecond)
    }))
    defer server.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()

    backend := NewAnthropicBackend(WithBaseURL(server.URL))
    _, err := backend.GenerateCommand(ctx, &Request{Query: "test"})

    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected timeout error, got %v", err)
    }
}
```

### 5.3 Safety Checker Tests

```go
// checker_test.go
func TestSafetyChecker(t *testing.T) {
    tests := []struct {
        name     string
        command  string
        expected DangerLevel
    }{
        // Safe commands
        {"ls", "ls -la", Safe},
        {"find", "find . -name '*.go'", Safe},
        {"rm single file", "rm file.txt", Safe},
        {"rm in tmp", "rm -rf /tmp/build", Safe},
        {"rm relative", "rm -rf ./dist", Safe},
        {"grep", "grep -r 'TODO' .", Safe},

        // Caution commands
        {"sudo", "sudo apt update", Caution},
        {"curl pipe", "curl https://example.com | bash", Caution},
        {"rm rf dir", "rm -rf node_modules", Caution},
        {"chmod recursive", "chmod -R 755 ./bin", Caution},

        // Danger commands - MUST be caught
        {"rm rf root", "rm -rf /", Danger},
        {"rm rf home", "rm -rf ~", Danger},
        {"rm rf home var", "rm -rf $HOME", Danger},
        {"rm rf root wildcard", "rm -rf /*", Danger},
        {"fork bomb", ":(){ :|:& };:", Danger},
        {"dd to disk", "dd if=/dev/zero of=/dev/sda", Danger},
        {"mkfs", "mkfs.ext4 /dev/sda1", Danger},
        {"chmod root", "chmod -R 777 /", Danger},
        {"mv root", "mv / /backup", Danger},

        // Edge cases - no false positives
        {"rm with path starting with slash", "rm -rf /var/log/myapp", Safe},
        {"echo with slash", "echo /", Safe},
        {"cat etc file", "cat /etc/hosts", Safe},

        // Nested/wrapped commands
        {"sudo rm rf root", "sudo rm -rf /", Danger},
        {"bash wrapper danger", "bash -c 'rm -rf /'", Danger},
        {"sh wrapper danger", `sh -c "rm -rf /"`, Danger},
        {"eval danger", "eval 'rm -rf /'", Danger},
        {"sudo safe cmd", "sudo apt update", Caution},  // sudo is caution, inner cmd is safe
    }

    checker := NewChecker()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := checker.Check(tt.command)
            if result.Level != tt.expected {
                t.Errorf("Check(%q) = %v, want %v", tt.command, result.Level, tt.expected)
            }
        })
    }
}
```

### 5.4 Sanitizer Tests

```go
// sanitize_test.go
func TestSanitize(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        // Basic cleanup
        {"code fence bash", "```bash\nls -la\n```", "ls -la"},
        {"code fence plain", "```\nls -la\n```", "ls -la"},
        {"inline backticks", "`ls -la`", "ls -la"},
        {"dollar prefix", "$ ls -la", "ls -la"},
        {"leading whitespace", "  ls -la", "ls -la"},
        {"trailing whitespace", "ls -la  ", "ls -la"},
        {"leading newlines", "\n\n\nls -la", "ls -la"},
        {"trailing newlines", "ls -la\n\n\n", "ls -la"},

        // Multi-line preservation
        {"multi-line preserved", "docker run \\\n  -v /data:/data \\\n  nginx",
            "docker run \\\n  -v /data:/data \\\n  nginx"},
        {"pipeline multi-line", "find . -name '*.go' \\\n  | xargs grep TODO",
            "find . -name '*.go' \\\n  | xargs grep TODO"},
        {"heredoc", "cat <<EOF\nhello\nEOF", "cat <<EOF\nhello\nEOF"},

        // No over-sanitization
        {"internal spaces preserved", "echo 'hello   world'", "echo 'hello   world'"},
        {"tabs preserved", "echo '\t\t'", "echo '\t\t'"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Sanitize(tt.input)
            if got != tt.expected {
                t.Errorf("Sanitize(%q) = %q, want %q", tt.input, got, tt.expected)
            }
        })
    }
}

func TestCheckErrorSentinel(t *testing.T) {
    tests := []struct {
        input     string
        isError   bool
        message   string
    }{
        {`echo "QCMD_ERROR: unclear request"`, true, "unclear request"},
        {`echo 'QCMD_ERROR: not possible'`, true, "not possible"},
        {`echo "hello world"`, false, ""},
        {`ls -la`, false, ""},
    }

    for _, tt := range tests {
        isErr, msg := CheckErrorSentinel(tt.input)
        if isErr != tt.isError {
            t.Errorf("CheckErrorSentinel(%q) isError = %v, want %v", tt.input, isErr, tt.isError)
        }
        if msg != tt.message {
            t.Errorf("CheckErrorSentinel(%q) message = %q, want %q", tt.input, msg, tt.message)
        }
    }
}
```

### 5.5 Integration Tests

```go
// integration_test.go (build tag: integration)
//go:build integration

// These require actual API keys and are run manually or in CI with secrets

func TestEndToEnd_Anthropic(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" {
        t.Skip("ANTHROPIC_API_KEY not set")
    }

    // Test simple query
    // Test multi-line result
    // Test error handling
}
```

---

## 6. Build & Distribution

### 6.1 Makefile

```makefile
.PHONY: build test lint clean install

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/qcmd ./cmd/qcmd

build-all:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/qcmd-darwin-amd64 ./cmd/qcmd
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/qcmd-darwin-arm64 ./cmd/qcmd
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/qcmd-linux-amd64 ./cmd/qcmd
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/qcmd-linux-arm64 ./cmd/qcmd

test:
	go test -v -race ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-integration:
	go test -v -race -tags=integration ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ coverage.out coverage.html

install: build
	mkdir -p $(HOME)/.local/bin
	cp bin/qcmd $(HOME)/.local/bin/qcmd
	@echo "Installed to $(HOME)/.local/bin/qcmd"
	@echo "Add 'source /path/to/qcmd.zsh' to your .zshrc"
```

### 6.2 CI/CD (GitHub Actions)

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go test -v -race ./...

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v4

  build:
    runs-on: ubuntu-latest
    needs: [test, lint]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: make build-all
      - uses: actions/upload-artifact@v4
        with:
          name: binaries
          path: bin/
```

---

## 7. Implementation Phases

### Phase 1: Foundation
- [x] ~~Project scaffold (go.mod, directory structure)~~
- [x] ~~Config package (TOML loading, defaults, env override)~~
- [x] ~~Editor package (temp file, $EDITOR invocation)~~
- [ ] Basic CLI structure (flags, help, version)
- [ ] Input validation

### Phase 2: Core Functionality
- [x] ~~Backend interface definition~~
- [x] ~~Anthropic backend implementation~~
- [x] ~~OpenAI backend implementation~~
- [x] ~~OpenRouter backend implementation~~
- [x] ~~Sanitizer package (with multi-line preservation)~~
- [x] ~~Error sentinel detection~~

### Phase 3: Safety & Output
- [x] ~~Safety checker (patterns, normalization, levels)~~
- [x] ~~Output router (modes, explicit selection)~~
- [x] ~~Clipboard detection (cross-platform)~~
- [x] ~~Exit code handling~~

### Phase 4: Integration
- [x] ~~Zsh shell wrapper (`qcmd.zsh`)~~
- [x] ~~End-to-end flow testing~~
- [x] ~~Error handling refinement~~
- [x] ~~Stderr/stdout separation verification~~

### Phase 5: Polish
- [x] ~~Comprehensive unit tests~~
- [x] ~~Safety checker test coverage~~
- [x] ~~Documentation (README, --help)~~
- [x] ~~Makefile, CI/CD~~
- [x] ~~Release binaries~~

---

## 8. Future Considerations (Out of Scope for v1)

- Bash shell support
- Fish shell support
- Command history/logging
- Multiple commands in one query
- Explain mode (`qcmd explain "awk command"`)
- Local LLM support (Ollama)
- Interactive mode (REPL)
- Shell completion
- Command snippets/templates

---

## 9. Dependencies

```go
// go.mod
module github.com/user/qcmd

go 1.21

require (
    github.com/BurntSushi/toml v1.3.2    // TOML parsing
)

// No other external dependencies - stdlib for HTTP, JSON, regex, exec
```

Minimal dependencies by design. Go stdlib handles HTTP, JSON, regex, exec.

---

## 10. Agent Delegation

This section defines how implementation work is distributed across parallel agents for maximum efficiency.

### 10.1 Dependency Graph

```
                    ┌─────────────────────┐
                    │  Agent 1: Foundation │
                    │  (sequential first)  │
                    └──────────┬──────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         │                     │                     │
         ▼                     ▼                     ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│ Agent 2:        │  │ Agent 3:        │  │ Agent 4:        │
│ Backends        │  │ Safety+Sanitize │  │ Output          │
└────────┬────────┘  └────────┬────────┘  └────────┬────────┘
         │                     │                     │
         │           ┌─────────────────┐             │
         │           │ Agent 5:        │             │
         │           │ Editor+Shellctx │             │
         │           └────────┬────────┘             │
         │                     │                     │
         └─────────────────────┼─────────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │ Agent 6: CLI Main   │
                    │ (after 2,3,4,5)     │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │ Agent 7: Polish     │
                    │ (final)             │
                    └─────────────────────┘
```

### 10.2 Agent Assignments

| Agent | Focus | Packages/Files | Dependencies |
|-------|-------|----------------|--------------|
| **1** | Foundation | `go.mod`, directory scaffold, `internal/config/`, type definitions | None (runs first) |
| **2** | LLM Backends | `internal/backend/backend.go`, `anthropic.go`, `openai.go`, `openrouter.go`, `backend_test.go` | Agent 1 |
| **3** | Safety & Sanitization | `internal/safety/checker.go`, `patterns.go`, `checker_test.go`, `internal/sanitize/sanitize.go`, `sanitize_test.go` | Agent 1 |
| **4** | Output System | `internal/output/output.go`, `clipboard.go`, `output_test.go` | Agent 1 |
| **5** | Editor & Shell Context | `internal/editor/editor.go`, `editor_test.go`, `internal/shellctx/shellctx.go`, `shellctx_test.go` | Agent 1 |
| **6** | CLI Orchestration | `cmd/qcmd/main.go`, `shell/qcmd.zsh`, end-to-end integration | Agents 2, 3, 4, 5 |
| **7** | Polish | `Makefile`, `.github/workflows/ci.yml`, `README.md`, integration tests, final test coverage | Agent 6 |

### 10.3 Execution Windows

```
Time ──────────────────────────────────────────────────────────────▶

Window 1 (sequential):
┌──────────────────────────────────────┐
│           Agent 1: Foundation        │
└──────────────────────────────────────┘

Window 2 (parallel - 4 agents simultaneously):
┌─────────────────┐ ┌─────────────────┐
│ Agent 2:        │ │ Agent 3:        │
│ Backends        │ │ Safety+Sanitize │
└─────────────────┘ └─────────────────┘
┌─────────────────┐ ┌─────────────────┐
│ Agent 4:        │ │ Agent 5:        │
│ Output          │ │ Editor+Shellctx │
└─────────────────┘ └─────────────────┘

Window 3 (sequential):
┌──────────────────────────────────────┐
│       Agent 6: CLI Orchestration     │
└──────────────────────────────────────┘

Window 4 (sequential):
┌──────────────────────────────────────┐
│           Agent 7: Polish            │
└──────────────────────────────────────┘
```

### 10.4 Agent Specifications

#### Agent 1: Foundation ✅ COMPLETE
**Goal:** Establish project structure and shared types that all other agents depend on.

**Deliverables:**
- [x] ~~Initialize `go.mod` with module path and Go version~~
- [x] ~~Create full directory structure (all `internal/` subdirs, `cmd/`, `shell/`, `testdata/`)~~
- [x] ~~Implement `internal/config/config.go` with TOML loading, defaults, env overrides~~
- [x] ~~Implement `internal/config/config_test.go`~~
- [x] ~~Define shared types in `internal/backend/backend.go` (interface + Request/Response/ShellContext structs)~~
- [x] ~~Create placeholder files for other packages (enables parallel agents to start)~~

**Interface Contract (for downstream agents):**
```go
// internal/backend/backend.go - Agent 1 defines, Agent 2 implements
type Backend interface {
    GenerateCommand(ctx context.Context, request *Request) (*Response, error)
    Name() string
}
```

---

#### Agent 2: LLM Backends ✅ COMPLETE
**Goal:** Implement all three LLM provider clients.

**Deliverables:**
- [x] ~~`internal/backend/anthropic.go` - Anthropic Claude API client~~
- [x] ~~`internal/backend/openai.go` - OpenAI API client~~
- [x] ~~`internal/backend/openrouter.go` - OpenRouter API client~~
- [x] ~~`internal/backend/backend_test.go` - Mock HTTP tests for all backends~~
- [x] ~~Shared system prompt template~~
- [x] ~~Timeout and error handling per spec~~

**Key Requirements:**
- All backends implement `Backend` interface from Agent 1
- Use `context.Context` for cancellation/timeouts
- Never log API keys
- Return appropriate errors for timeout (exit code 2)

---

#### Agent 3: Safety & Sanitization ✅ COMPLETE
**Goal:** Implement command safety checking and LLM output sanitization.

**Deliverables:**
- [x] ~~`internal/safety/patterns.go` - Pattern registry with all DANGER/CAUTION patterns~~
- [x] ~~`internal/safety/checker.go` - Checker with normalization and nested command detection~~
- [x] ~~`internal/safety/checker_test.go` - Comprehensive table-driven tests~~
- [x] ~~`internal/sanitize/sanitize.go` - Markdown/backtick stripping, multi-line preservation~~
- [x] ~~`internal/sanitize/sanitize_test.go` - Including error sentinel tests~~

**Key Requirements:**
- Nested command extraction (sudo, sh -c, bash -c, eval)
- Zero false negatives on DANGER patterns
- Preserve multi-line command structure in sanitizer

---

#### Agent 4: Output System ✅ COMPLETE
**Goal:** Implement output routing and clipboard integration.

**Deliverables:**
- [x] ~~`internal/output/output.go` - Output mode router (ZLE, clipboard, print, auto)~~
- [x] ~~`internal/output/clipboard.go` - Cross-platform clipboard (pbcopy, wl-copy, xclip, xsel)~~
- [x] ~~`internal/output/output_test.go`~~

**Key Requirements:**
- Graceful fallback chain: clipboard → print
- Detect missing clipboard tools without error spam
- ZLE mode outputs raw command to stdout (no newline formatting)

---

#### Agent 5: Editor & Shell Context ✅ COMPLETE
**Goal:** Implement editor invocation and shell context gathering.

**Deliverables:**
- [x] ~~`internal/editor/editor.go` - Temp file creation, $EDITOR/$VISUAL invocation~~
- [x] ~~`internal/editor/editor_test.go`~~
- [x] ~~`internal/shellctx/shellctx.go` - Gather pwd, shell type, OS~~
- [x] ~~`internal/shellctx/shellctx_test.go`~~

**Key Requirements:**
- Handle editors with arguments (e.g., `EDITOR="code --wait"`)
- Temp files with 0600 permissions, cleanup in defer
- Comment stripping from editor input

---

#### Agent 6: CLI Orchestration ✅ COMPLETE
**Goal:** Wire everything together into the main binary and shell wrapper.

**Deliverables:**
- [x] ~~`cmd/qcmd/main.go` - Full CLI with flag parsing, orchestration flow~~
- [x] ~~`shell/qcmd.zsh` - Zsh function wrapper with ZLE integration~~
- [x] ~~Input validation logic~~
- [x] ~~Exit code handling (0, 1, 2, 3)~~
- [x] ~~Stderr/stdout separation~~

**Key Requirements:**
- Import and orchestrate all `internal/` packages
- Input precedence: --query-file > --query > editor
- `config init` creates directory with 0700, file with 0600
- Pass `--output=zle` from shell wrapper

---

#### Agent 7: Polish ✅ COMPLETE
**Goal:** Production readiness - build system, CI/CD, documentation.

**Deliverables:**
- [x] ~~`Makefile` - build, build-all, test, test-coverage, lint, clean, install~~
- [x] ~~`.github/workflows/ci.yml` - Test, lint, build matrix~~
- [x] ~~`README.md` - Installation, usage, configuration, examples~~
- [x] ~~Integration tests (build tag: integration)~~
- [x] ~~Version injection via ldflags~~
- [x] ~~Final test coverage review (aim for >80%)~~

**Key Requirements:**
- Cross-compile for darwin/linux, amd64/arm64
- CI runs tests with `-race` flag
- README includes shell integration instructions

---

### 10.5 Handoff Protocol

1. **Agent 1 completes** → Creates GitHub issue or signal indicating foundation is ready
2. **Agents 2-5 start in parallel** → Each works independently on assigned packages
3. **Agents 2-5 complete** → All internal packages ready with tests passing
4. **Agent 6 starts** → Integrates all packages, may surface integration issues
5. **Agent 6 completes** → Working binary, shell wrapper functional
6. **Agent 7 starts** → Polish, documentation, CI/CD

### 10.6 Interface Contracts Summary

These are defined by Agent 1 and must be respected by all downstream agents:

```go
// Backend interface (Agent 1 defines, Agent 2 implements)
type Backend interface {
    GenerateCommand(ctx context.Context, request *Request) (*Response, error)
    Name() string
}

// Safety checker (Agent 3 implements, Agent 6 consumes)
type CheckResult struct {
    Level       DangerLevel
    Pattern     string
    Description string
    Category    string
}
func (c *Checker) Check(cmd string) CheckResult

// Sanitizer (Agent 3 implements, Agent 6 consumes)
func Sanitize(raw string) string
func CheckErrorSentinel(cmd string) (bool, string)

// Output router (Agent 4 implements, Agent 6 consumes)
func Output(cmd string, mode Mode, isDangerous bool) error

// Editor (Agent 5 implements, Agent 6 consumes)
func (e *Editor) GetInput(ctx context.Context) (string, error)

// Shell context (Agent 5 implements, Agent 6 consumes)
func GatherContext() *ShellContext
```

---

## Appendix A: Sample Interactions

**Example 1: Basic usage**
```
$ q
# (nvim opens)
# User types: find all .go files modified in the last 24 hours
# (save and quit)
$ find . -name "*.go" -mtime -1█
# (command is in the buffer, cursor at end, user reviews then presses Enter)
```

**Example 2: Multi-line command**
```
$ q
# User types: docker run nginx with port 80 mapped and a volume for /data
# (save and quit)
$ docker run \
  --name nginx \
  -p 80:80 \
  -v /data:/usr/share/nginx/html:ro \
  nginx:latest█
# (multi-line command preserved in buffer for easy review)
```

**Example 3: Dangerous command blocked**
```
$ q
# User types: delete everything in the root directory
# (save and quit)

⚠️  Command blocked from injection (safety check triggered)
Review the command below. Copy manually if intended:

rm -rf /

# (command printed but NOT in buffer - user must manually copy if really intended)
```

**Example 4: Clipboard fallback (when run directly)**
```
$ qcmd --query "find large files over 100MB"
Command copied to clipboard.
# (user pastes with Ctrl+Shift+V)
```

**Example 5: LLM returns error**
```
$ q
# User types: asdfghjkl
# (save and quit)
LLM could not generate command: unclear request
# (nothing in buffer, exit code 1)
```
