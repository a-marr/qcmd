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
	// DefaultOpenAIBaseURL is the default API endpoint for OpenAI.
	DefaultOpenAIBaseURL = "https://api.openai.com/v1/chat/completions"

	// DefaultOpenAIModel is the default model for OpenAI.
	DefaultOpenAIModel = "gpt-5o"
)

// OpenAIBackend implements the Backend interface for the OpenAI API.
type OpenAIBackend struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// OpenAIOption is a functional option for configuring OpenAIBackend.
type OpenAIOption func(*OpenAIBackend)

// WithOpenAIAPIKey sets the API key for the OpenAI backend.
func WithOpenAIAPIKey(key string) OpenAIOption {
	return func(b *OpenAIBackend) {
		b.apiKey = key
	}
}

// WithOpenAIBaseURL sets a custom base URL (useful for testing).
func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(b *OpenAIBackend) {
		b.baseURL = url
	}
}

// WithOpenAIModel sets the model to use.
func WithOpenAIModel(model string) OpenAIOption {
	return func(b *OpenAIBackend) {
		b.model = model
	}
}

// WithOpenAIMaxTokens sets the maximum tokens for responses.
func WithOpenAIMaxTokens(tokens int) OpenAIOption {
	return func(b *OpenAIBackend) {
		b.maxTokens = tokens
	}
}

// WithOpenAIHTTPClient sets a custom HTTP client.
func WithOpenAIHTTPClient(client *http.Client) OpenAIOption {
	return func(b *OpenAIBackend) {
		b.httpClient = client
	}
}

// NewOpenAIBackend creates a new OpenAI backend with the given options.
func NewOpenAIBackend(opts ...OpenAIOption) *OpenAIBackend {
	b := &OpenAIBackend{
		baseURL:    DefaultOpenAIBaseURL,
		model:      DefaultOpenAIModel,
		maxTokens:  DefaultMaxTokens,
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Name returns the backend identifier.
func (b *OpenAIBackend) Name() string {
	return "openai"
}

// openaiRequest is the request body for the OpenAI API.
type openaiRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []openaiMessage `json:"messages"`
}

// openaiMessage represents a message in the OpenAI API.
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openaiResponse is the response from the OpenAI API.
type openaiResponse struct {
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
	Error *openaiError `json:"error,omitempty"`
}

// openaiError represents an error from the OpenAI API.
type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

// GenerateCommand sends a query to the OpenAI API and returns a shell command.
func (b *OpenAIBackend) GenerateCommand(ctx context.Context, request *Request) (*Response, error) {
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
	reqBody := openaiRequest{
		Model:     model,
		MaxTokens: b.maxTokens,
		Messages: []openaiMessage{
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
		var apiResp openaiResponse
		if err := json.Unmarshal(body, &apiResp); err == nil && apiResp.Error != nil {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiResp.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp openaiResponse
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
func (b *OpenAIBackend) buildSystemPrompt(shellCtx *ShellContext) (string, error) {
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
