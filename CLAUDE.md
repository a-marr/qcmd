# CLAUDE.md - Project Intelligence for qcmd

## Project Overview

qcmd is a command-line tool that opens your editor, lets you describe what you want in natural language, and returns a ready-to-execute shell command via LLM.

## Tech Stack

- **Language:** Go 1.21+
- **Config:** TOML (`~/.config/qcmd/config.toml`)
- **Shell Integration:** Zsh (primary), clipboard fallback
- **LLM Backends:** Anthropic (claude-4-haiku), OpenAI (gpt-5o), OpenRouter (any model)

## Architecture

```
cmd/qcmd/main.go          → entry point, CLI parsing
internal/
  backend/                → LLM API clients (interface-based)
  editor/                 → temp file + $EDITOR handling
  safety/                 → deterministic danger pattern checks
  output/                 → ZLE injection, clipboard, print fallback
  config/                 → TOML config loading
  sanitize/               → strip markdown, backticks from LLM output
shell/
  qcmd.zsh                → zsh integration (function + optional keybind)
```

## Build Commands

```bash
go build -o qcmd ./cmd/qcmd
go test ./...
go test -race ./...
make build          # cross-compile for darwin/linux
make test
make lint           # golangci-lint
```

## Key Design Decisions

1. **Deterministic safety checks** - No LLM validation for danger detection (latency). Regex-based pattern matching.
2. **Interface-based backends** - All LLM providers implement `Backend` interface for easy extension.
3. **Graceful output degradation** - ZLE → clipboard → print fallback chain.
4. **No shell injection of dangerous commands** - High-risk commands print-only with warning.

## Testing Approach

- Unit tests for each package
- Table-driven tests for safety patterns
- Mock HTTP for backend tests
- Integration tests for editor flow (optional, requires TTY)

## Security Considerations

- API keys in config file with 0600 permissions
- Never log or print API keys
- Sanitize all LLM output before shell injection
- Dangerous command patterns block injection by default

## Git Commit Rules

- Never include "Co-Authored-By" or any AI/assistant attribution in commits
- Write clear, conventional commit messages (feat:, fix:, docs:, refactor:, etc.)

## Common Patterns

- Use `context.Context` for cancellation/timeouts on API calls
- Wrap errors with `fmt.Errorf("context: %w", err)`
- Config uses sensible defaults, env vars override file config
- Exit codes: 0=success, 1=user error, 2=system error, 3=dangerous command blocked
- stdout is ONLY for the command; stderr for all diagnostics
