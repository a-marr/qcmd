// Package backend provides LLM backend implementations.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
)

const (
	// DefaultAnthropicBaseURL is the default API endpoint for Anthropic.
	DefaultAnthropicBaseURL = "https://api.anthropic.com/v1/messages"

	// DefaultAnthropicModel is the default model for Anthropic.
	DefaultAnthropicModel = "claude-haiku-4-5-20251001"

	// DefaultMaxTokens is the default maximum tokens for responses.
	DefaultMaxTokens = 512

	// AnthropicAPIVersion is the required API version header.
	AnthropicAPIVersion = "2023-06-01"
)

// AnthropicBackend implements the Backend interface for the Anthropic API.
type AnthropicBackend struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// AnthropicOption is a functional option for configuring AnthropicBackend.
type AnthropicOption func(*AnthropicBackend)

// WithAnthropicAPIKey sets the API key for the Anthropic backend.
func WithAnthropicAPIKey(key string) AnthropicOption {
	return func(b *AnthropicBackend) {
		b.apiKey = key
	}
}

// WithAnthropicBaseURL sets a custom base URL (useful for testing).
func WithAnthropicBaseURL(url string) AnthropicOption {
	return func(b *AnthropicBackend) {
		b.baseURL = url
	}
}

// WithAnthropicModel sets the model to use.
func WithAnthropicModel(model string) AnthropicOption {
	return func(b *AnthropicBackend) {
		b.model = model
	}
}

// WithAnthropicMaxTokens sets the maximum tokens for responses.
func WithAnthropicMaxTokens(tokens int) AnthropicOption {
	return func(b *AnthropicBackend) {
		b.maxTokens = tokens
	}
}

// WithAnthropicHTTPClient sets a custom HTTP client.
func WithAnthropicHTTPClient(client *http.Client) AnthropicOption {
	return func(b *AnthropicBackend) {
		b.httpClient = client
	}
}

// NewAnthropicBackend creates a new Anthropic backend with the given options.
func NewAnthropicBackend(opts ...AnthropicOption) *AnthropicBackend {
	b := &AnthropicBackend{
		baseURL:    DefaultAnthropicBaseURL,
		model:      DefaultAnthropicModel,
		maxTokens:  DefaultMaxTokens,
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Name returns the backend identifier.
func (b *AnthropicBackend) Name() string {
	return "anthropic"
}

// anthropicRequest is the request body for the Anthropic API.
type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
}

// anthropicMessage represents a message in the Anthropic API.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the response from the Anthropic API.
type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *anthropicError `json:"error,omitempty"`
}

// anthropicError represents an error from the Anthropic API.
type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// GenerateCommand sends a query to the Anthropic API and returns a shell command.
func (b *AnthropicBackend) GenerateCommand(ctx context.Context, request *Request) (*Response, error) {
	if b.apiKey == "" {
		return nil, ErrNoAPIKey
	}

	if request.Query == "" {
		return nil, ErrEmptyQuery
	}

	// Build system prompt
	systemPrompt, err := b.buildSystemPrompt(request.Context)
	if err != nil {
		return nil, fmt.Errorf("building system prompt: %w", err)
	}

	// Determine model to use
	model := b.model
	if request.Model != "" {
		model = request.Model
	}

	// Build request body
	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: b.maxTokens,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: request.Query},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", b.apiKey)
	httpReq.Header.Set("anthropic-version", AnthropicAPIVersion)

	// Execute request
	resp, err := b.httpClient.Do(httpReq)
	if err != nil {
		// Check for context deadline exceeded
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("request timeout: %w", context.DeadlineExceeded)
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("request canceled: %w", context.Canceled)
		}
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Handle non-2xx responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiResp anthropicResponse
		if err := json.Unmarshal(body, &apiResp); err == nil && apiResp.Error != nil {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiResp.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Extract command from response
	if len(apiResp.Content) == 0 {
		return nil, ErrEmptyResponse
	}

	command := ""
	for _, content := range apiResp.Content {
		if content.Type == "text" {
			command = strings.TrimSpace(content.Text)
			break
		}
	}

	if command == "" {
		return nil, ErrEmptyResponse
	}

	return &Response{
		Command:    command,
		Model:      apiResp.Model,
		TokensUsed: apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
	}, nil
}

// buildSystemPrompt constructs the system prompt with optional context.
func (b *AnthropicBackend) buildSystemPrompt(shellCtx *ShellContext) (string, error) {
	if shellCtx == nil {
		return SystemPromptNoContext, nil
	}

	tmpl, err := template.New("system").Parse(SystemPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		WorkingDir string
		Shell      string
		OS         string
	}{
		WorkingDir: shellCtx.WorkingDir,
		Shell:      shellCtx.Shell,
		OS:         shellCtx.OS,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
