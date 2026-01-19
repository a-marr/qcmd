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
	// DefaultOpenRouterBaseURL is the default API endpoint for OpenRouter.
	DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1/chat/completions"

	// DefaultOpenRouterModel is the default model for OpenRouter.
	DefaultOpenRouterModel = "anthropic/claude-4-haiku"

	// DefaultHTTPReferer is the default referer header for OpenRouter.
	DefaultHTTPReferer = "https://github.com/user/qcmd"

	// DefaultXTitle is the default X-Title header for OpenRouter.
	DefaultXTitle = "qcmd"
)

// OpenRouterBackend implements the Backend interface for the OpenRouter API.
type OpenRouterBackend struct {
	apiKey      string
	baseURL     string
	model       string
	maxTokens   int
	httpReferer string
	xTitle      string
	httpClient  *http.Client
}

// OpenRouterOption is a functional option for configuring OpenRouterBackend.
type OpenRouterOption func(*OpenRouterBackend)

// WithOpenRouterAPIKey sets the API key for the OpenRouter backend.
func WithOpenRouterAPIKey(key string) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.apiKey = key
	}
}

// WithOpenRouterBaseURL sets a custom base URL (useful for testing).
func WithOpenRouterBaseURL(url string) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.baseURL = url
	}
}

// WithOpenRouterModel sets the model to use.
func WithOpenRouterModel(model string) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.model = model
	}
}

// WithOpenRouterMaxTokens sets the maximum tokens for responses.
func WithOpenRouterMaxTokens(tokens int) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.maxTokens = tokens
	}
}

// WithOpenRouterHTTPReferer sets the HTTP-Referer header.
func WithOpenRouterHTTPReferer(referer string) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.httpReferer = referer
	}
}

// WithOpenRouterXTitle sets the X-Title header.
func WithOpenRouterXTitle(title string) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.xTitle = title
	}
}

// WithOpenRouterHTTPClient sets a custom HTTP client.
func WithOpenRouterHTTPClient(client *http.Client) OpenRouterOption {
	return func(b *OpenRouterBackend) {
		b.httpClient = client
	}
}

// NewOpenRouterBackend creates a new OpenRouter backend with the given options.
func NewOpenRouterBackend(opts ...OpenRouterOption) *OpenRouterBackend {
	b := &OpenRouterBackend{
		baseURL:     DefaultOpenRouterBaseURL,
		model:       DefaultOpenRouterModel,
		maxTokens:   DefaultMaxTokens,
		httpReferer: DefaultHTTPReferer,
		xTitle:      DefaultXTitle,
		httpClient:  http.DefaultClient,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Name returns the backend identifier.
func (b *OpenRouterBackend) Name() string {
	return "openrouter"
}

// openrouterRequest is the request body for the OpenRouter API.
// OpenRouter uses OpenAI-compatible format.
type openrouterRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	Messages  []openrouterMessage `json:"messages"`
}

// openrouterMessage represents a message in the OpenRouter API.
type openrouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openrouterResponse is the response from the OpenRouter API.
// OpenRouter uses OpenAI-compatible format.
type openrouterResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *openrouterError `json:"error,omitempty"`
}

// openrouterError represents an error from the OpenRouter API.
type openrouterError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code,omitempty"`
}

// GenerateCommand sends a query to the OpenRouter API and returns a shell command.
func (b *OpenRouterBackend) GenerateCommand(ctx context.Context, request *Request) (*Response, error) {
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

	// Build request body (OpenAI-compatible format)
	reqBody := openrouterRequest{
		Model:     model,
		MaxTokens: b.maxTokens,
		Messages: []openrouterMessage{
			{Role: "system", Content: systemPrompt},
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
	httpReq.Header.Set("Authorization", "Bearer "+b.apiKey)
	httpReq.Header.Set("HTTP-Referer", b.httpReferer)
	httpReq.Header.Set("X-Title", b.xTitle)

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
		var apiResp openrouterResponse
		if err := json.Unmarshal(body, &apiResp); err == nil && apiResp.Error != nil {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiResp.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp openrouterResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Extract command from response
	if len(apiResp.Choices) == 0 {
		return nil, ErrEmptyResponse
	}

	command := strings.TrimSpace(apiResp.Choices[0].Message.Content)
	if command == "" {
		return nil, ErrEmptyResponse
	}

	return &Response{
		Command:    command,
		Model:      apiResp.Model,
		TokensUsed: apiResp.Usage.TotalTokens,
	}, nil
}

// buildSystemPrompt constructs the system prompt with optional context.
func (b *OpenRouterBackend) buildSystemPrompt(shellCtx *ShellContext) (string, error) {
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
