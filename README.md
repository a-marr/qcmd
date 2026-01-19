# qcmd

A command-line tool that opens your editor, lets you describe what you want in natural language, and returns a ready-to-execute shell command via LLM.

## Features

- **Natural language to shell commands**: Describe what you want, get a working command
- **Editor-based input**: Opens your preferred editor ($EDITOR) for comfortable multi-line queries
- **Multiple LLM backends**: Anthropic (Claude), OpenAI (GPT), and OpenRouter support
- **Shell integration**: Seamless Zsh integration with the `q` function
- **Safety checks**: Detects and warns about dangerous commands (rm -rf, sudo, etc.)
- **Context-aware**: Optionally includes your current directory, shell, and OS in prompts
- **Graceful output**: ZLE injection, clipboard, or print fallback

## Quick Start

### 1. Install

```bash
git clone https://github.com/user/qcmd.git
cd qcmd
make build && make install   # installs to ~/.local/bin/
```

### 2. Set API Key

```bash
# Add to ~/.zshrc
export ANTHROPIC_API_KEY="sk-ant-..."
```

### 3. Add Shell Integration

`make install` copies the shell integration to `~/.config/qcmd/qcmd.zsh`. Add to your `~/.zshrc`:

```bash
source ~/.config/qcmd/qcmd.zsh
```

### 4. Use It

```bash
q                    # opens $EDITOR, describe what you want, save & quit
                     # command appears in shell buffer ready to execute
```

Or use directly:

```bash
qcmd --query "find files modified today" --output=print
```

### Key Flags

| Flag | Purpose |
|------|---------|
| `--query "..."` | Direct query without opening editor |
| `--output=print` | Print command (instead of clipboard) |
| `--output=zle` | Output for shell wrapper (no newline) |
| `--backend=openai` | Switch LLM provider |
| `--model=gpt-4o` | Override model |
| `--verbose` | Show model and token info |
| `--no-safety` | Disable safety checks |

## Installation

### Download Binary

Download the latest release for your platform from [Releases](../../releases):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/user/qcmd/releases/latest/download/qcmd-darwin-arm64 -o qcmd
chmod +x qcmd
mv qcmd ~/.local/bin/

# macOS (Intel)
curl -L https://github.com/user/qcmd/releases/latest/download/qcmd-darwin-amd64 -o qcmd
chmod +x qcmd
mv qcmd ~/.local/bin/

# Linux (x86_64)
curl -L https://github.com/user/qcmd/releases/latest/download/qcmd-linux-amd64 -o qcmd
chmod +x qcmd
mv qcmd ~/.local/bin/

# Linux (ARM64)
curl -L https://github.com/user/qcmd/releases/latest/download/qcmd-linux-arm64 -o qcmd
chmod +x qcmd
mv qcmd ~/.local/bin/
```

### Build from Source

Requires Go 1.21+:

```bash
git clone https://github.com/user/qcmd.git
cd qcmd
make build
make install  # Installs to ~/.local/bin
```

### Cross-compile All Platforms

```bash
make build-all  # Creates binaries in bin/
```

## Configuration

### Initialize Config

Create a default config file:

```bash
qcmd config init
```

This creates `~/.config/qcmd/config.toml` with sensible defaults.

### Config File

```toml
# ~/.config/qcmd/config.toml

# Default backend: anthropic | openai | openrouter
backend = "anthropic"

# Include shell context (pwd, shell, OS) in prompts
include_context = true

# Output mode when run directly: auto | clipboard | print
output_mode = "auto"

[anthropic]
api_key = ""  # Or use ANTHROPIC_API_KEY env var
model = "claude-haiku-4-5-20251001"

[openai]
api_key = ""  # Or use OPENAI_API_KEY env var
model = "gpt-5o"

[openrouter]
api_key = ""  # Or use OPENROUTER_API_KEY env var
model = "anthropic/claude-haiku-4-5-20251001"

[safety]
block_dangerous = true   # Block dangerous commands from injection
show_warnings = true     # Show warnings for cautionary commands

[editor]
# editor = "nvim"  # Override $EDITOR/$VISUAL

[advanced]
timeout_seconds = 30
max_tokens = 512
```

### Environment Variables

Environment variables override config file values:

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `QCMD_BACKEND` | Override default backend |
| `QCMD_CONFIG` | Path to config file |

### Config Priority

1. Command-line flags (highest)
2. Environment variables
3. Config file
4. Default values (lowest)

## Usage

### Shell Integration (Recommended)

`make install` automatically copies the shell integration file. Add to your `~/.zshrc`:

```bash
source ~/.config/qcmd/qcmd.zsh
```

Then use the `q` function:

```bash
q  # Opens editor, describe what you want, command appears ready to execute
```

The command is placed in your shell buffer - review it and press Enter to execute, or Ctrl+C to cancel.

### Direct Usage

```bash
# Interactive (opens editor)
qcmd

# Direct query
qcmd --query "find all .go files modified in the last week"

# Query from file
qcmd --query-file prompt.txt

# Override backend/model
qcmd --backend openai --model gpt-5o --query "list large files"

# Different output modes
qcmd --output clipboard --query "show disk usage"
qcmd --output print --query "count lines in src/"
```

### Flags

| Flag | Description |
|------|-------------|
| `--query` | Direct query string |
| `--query-file` | Read query from file |
| `--backend` | Override backend (anthropic, openai, openrouter) |
| `--model` | Override model |
| `--output` | Output mode: zle, clipboard, print, auto |
| `--no-safety` | Disable safety checks |
| `--config` | Path to config file |
| `--verbose` | Verbose output to stderr |
| `--version` | Print version and exit |

### Subcommands

```bash
qcmd config       # Show current configuration
qcmd config init  # Create default config file
qcmd backends     # List available backends and status
```

## Safety Features

qcmd includes deterministic safety checks that detect potentially dangerous commands:

### Danger Level (Blocked by Default)

Commands that match these patterns are blocked from shell injection:

- `rm -rf /` or similar destructive patterns
- `dd` writes to devices (`of=/dev/`)
- `mkfs` filesystem operations
- `:(){ :|:& };:` fork bombs
- `chmod -R 777 /` recursive permission changes
- `> /dev/sda` device writes

When a dangerous command is detected:
1. A warning is displayed with the category and reason
2. The command is printed but NOT injected into your shell
3. Exit code 3 is returned

### Caution Level (Warnings)

Commands that warrant review show warnings but are still executed:

- `sudo` usage
- `rm -rf` (non-root paths)
- `curl | bash` patterns
- `chmod/chown -R` recursive operations
- Environment modifications (`export`, `unset`)

### Disabling Safety Checks

```bash
qcmd --no-safety --query "..."  # Disable for one command
```

Or in config:

```toml
[safety]
block_dangerous = false
show_warnings = false
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | User error (invalid input, config error) |
| 2 | System error (API failure, timeout) |
| 3 | Dangerous command blocked |

## Development

```bash
make build         # Build for current platform
make build-all     # Cross-compile all platforms
make test          # Run tests with race detector
make test-coverage # Generate coverage report
make lint          # Run golangci-lint
make clean         # Remove build artifacts
```

## LLM Backends

### Anthropic (Default)

Uses Claude models. Fast and accurate for shell commands.

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

### OpenAI

Uses GPT models.

```bash
export OPENAI_API_KEY="sk-..."
```

### OpenRouter

Access any model available on OpenRouter.

```bash
export OPENROUTER_API_KEY="sk-or-..."
```

Configure in config.toml:

```toml
[openrouter]
model = "anthropic/claude-haiku-4-5-20251001"  # Or any OpenRouter model
```

## License

MIT License - see [LICENSE](LICENSE) for details.
