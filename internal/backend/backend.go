// Package backend defines the interface for LLM providers and shared types.
package backend

import (
	"context"
	"errors"
)

// Common errors returned by backends.
var (
	// ErrNoAPIKey is returned when no API key is configured for a backend.
	ErrNoAPIKey = errors.New("no API key configured")

	// ErrEmptyQuery is returned when the query is empty.
	ErrEmptyQuery = errors.New("empty query")

	// ErrEmptyResponse is returned when the LLM returns an empty response.
	ErrEmptyResponse = errors.New("empty response from LLM")
)

// Backend defines the contract for LLM providers.
type Backend interface {
	// GenerateCommand sends a query to the LLM and returns a shell command.
	// The context should be used for cancellation and timeouts.
	GenerateCommand(ctx context.Context, request *Request) (*Response, error)

	// Name returns the backend identifier for logging/debugging.
	Name() string
}

// Request contains the input for command generation.
type Request struct {
	// Query is the user's natural language query describing the desired command.
	Query string

	// Context provides optional shell context (pwd, shell type, OS).
	// May be nil if context is not available or disabled.
	Context *ShellContext

	// Model overrides the default model for this request.
	// If empty, the backend's default model is used.
	Model string
}

// Response contains the result of command generation.
type Response struct {
	// Command is the generated shell command.
	Command string

	// Model is the model that was used for generation.
	Model string

	// TokensUsed is the number of tokens consumed (for cost tracking).
	// May be 0 if not available from the API.
	TokensUsed int
}

// ShellContext provides context about the user's shell environment.
// This information is included in the prompt to help the LLM generate
// more appropriate commands.
type ShellContext struct {
	// WorkingDir is the current working directory (pwd).
	WorkingDir string

	// Shell is the shell type, e.g., "zsh", "bash".
	Shell string

	// OS is the operating system, e.g., "darwin", "linux".
	OS string
}

// SystemPromptTemplate is the shared system prompt template for all backends.
// It instructs the LLM to output only shell commands.
const SystemPromptTemplate = `You are a shell command generator. Your ONLY job is to output a valid shell command.

Rules:
1. Output ONLY the raw shell command - no explanation, no markdown, no code fences
2. Do not include any text before or after the command
3. If multiple commands are needed, chain them with && or ;
4. For complex commands, use proper line continuation with backslashes
5. If the request is unclear or impossible, output exactly: echo "QCMD_ERROR: <brief reason>"
6. If the request would require dangerous operations, still provide the command (the tool handles safety)
7. Escape shell metacharacters properly (e.g., use \; not ; in find -exec, escape $ in strings)

Context provided:
- Working directory: {{.WorkingDir}}
- Shell: {{.Shell}}
- OS: {{.OS}}`

// SystemPromptNoContext is the system prompt when shell context is not available.
const SystemPromptNoContext = `You are a shell command generator. Your ONLY job is to output a valid shell command.

Rules:
1. Output ONLY the raw shell command - no explanation, no markdown, no code fences
2. Do not include any text before or after the command
3. If multiple commands are needed, chain them with && or ;
4. For complex commands, use proper line continuation with backslashes
5. If the request is unclear or impossible, output exactly: echo "QCMD_ERROR: <brief reason>"
6. If the request would require dangerous operations, still provide the command (the tool handles safety)
7. Escape shell metacharacters properly (e.g., use \; not ; in find -exec, escape $ in strings)`
