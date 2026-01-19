package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Anthropic Backend Tests
// =============================================================================

func TestAnthropicBackend_Name(t *testing.T) {
	b := NewAnthropicBackend()
	if got := b.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want %q", got, "anthropic")
	}
}

func TestAnthropicBackend_GenerateCommand_Success(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "test-api-key" {
			t.Errorf("expected x-api-key header, got %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("expected anthropic-version header, got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type header, got %q", got)
		}

		// Verify request body
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if reqBody.Model != "claude-4-haiku" {
			t.Errorf("expected model claude-4-haiku, got %s", reqBody.Model)
		}
		if reqBody.MaxTokens != 512 {
			t.Errorf("expected max_tokens 512, got %d", reqBody.MaxTokens)
		}
		if len(reqBody.Messages) != 1 || reqBody.Messages[0].Role != "user" {
			t.Errorf("expected one user message, got %+v", reqBody.Messages)
		}

		// Send response
		resp := anthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-4-haiku",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "ls -la"},
			},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{InputTokens: 10, OutputTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create backend with mock server
	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	// Execute
	resp, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Command != "ls -la" {
		t.Errorf("expected command 'ls -la', got %q", resp.Command)
	}
	if resp.Model != "claude-4-haiku" {
		t.Errorf("expected model 'claude-4-haiku', got %q", resp.Model)
	}
	if resp.TokensUsed != 15 {
		t.Errorf("expected 15 tokens used, got %d", resp.TokensUsed)
	}
}

func TestAnthropicBackend_GenerateCommand_WithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// Verify context is included in system prompt
		if !strings.Contains(reqBody.System, "/home/user") {
			t.Errorf("expected WorkingDir in system prompt, got %q", reqBody.System)
		}
		if !strings.Contains(reqBody.System, "zsh") {
			t.Errorf("expected Shell in system prompt, got %q", reqBody.System)
		}
		if !strings.Contains(reqBody.System, "darwin") {
			t.Errorf("expected OS in system prompt, got %q", reqBody.System)
		}

		resp := anthropicResponse{
			Model: "claude-4-haiku",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "pwd"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "show current directory",
		Context: &ShellContext{
			WorkingDir: "/home/user",
			Shell:      "zsh",
			OS:         "darwin",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnthropicBackend_GenerateCommand_ModelOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody.Model != "claude-3-opus" {
			t.Errorf("expected model override 'claude-3-opus', got %q", reqBody.Model)
		}

		resp := anthropicResponse{
			Model: "claude-3-opus",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "echo test"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	resp, err := b.GenerateCommand(context.Background(), &Request{
		Query: "test",
		Model: "claude-3-opus",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "claude-3-opus" {
		t.Errorf("expected model 'claude-3-opus', got %q", resp.Model)
	}
}

func TestAnthropicBackend_GenerateCommand_NoAPIKey(t *testing.T) {
	b := NewAnthropicBackend()

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestAnthropicBackend_GenerateCommand_EmptyQuery(t *testing.T) {
	b := NewAnthropicBackend(WithAnthropicAPIKey("test-key"))

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "",
	})

	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestAnthropicBackend_GenerateCommand_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			Model:   "claude-4-haiku",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("expected ErrEmptyResponse, got %v", err)
	}
}

func TestAnthropicBackend_GenerateCommand_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(anthropicResponse{})
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.GenerateCommand(ctx, &Request{
		Query: "list files",
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestAnthropicBackend_GenerateCommand_APIError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantErr    string
	}{
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			response:   `{"error":{"type":"authentication_error","message":"Invalid API key"}}`,
			wantErr:    "API error (401): Invalid API key",
		},
		{
			name:       "429 Rate Limited",
			statusCode: 429,
			response:   `{"error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`,
			wantErr:    "API error (429): Rate limit exceeded",
		},
		{
			name:       "500 Server Error",
			statusCode: 500,
			response:   `{"error":{"type":"server_error","message":"Internal server error"}}`,
			wantErr:    "API error (500): Internal server error",
		},
		{
			name:       "Malformed error response",
			statusCode: 400,
			response:   `not json`,
			wantErr:    "API error (400): not json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			b := NewAnthropicBackend(
				WithAnthropicAPIKey("test-api-key"),
				WithAnthropicBaseURL(server.URL),
			)

			_, err := b.GenerateCommand(context.Background(), &Request{
				Query: "test",
			})

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// =============================================================================
// OpenAI Backend Tests
// =============================================================================

func TestOpenAIBackend_Name(t *testing.T) {
	b := NewOpenAIBackend()
	if got := b.Name(); got != "openai" {
		t.Errorf("Name() = %q, want %q", got, "openai")
	}
}

func TestOpenAIBackend_GenerateCommand_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Errorf("expected Authorization header, got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type header, got %q", got)
		}

		// Verify request body
		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if reqBody.Model != "gpt-5o" {
			t.Errorf("expected model gpt-5o, got %s", reqBody.Model)
		}
		if len(reqBody.Messages) != 2 {
			t.Errorf("expected 2 messages (system + user), got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("expected first message to be system, got %s", reqBody.Messages[0].Role)
		}

		// Send response
		resp := openaiResponse{
			ID:      "chatcmpl-123",
			Model:   "gpt-5o",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{Role: "assistant", Content: "ls -la"},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenAIBackend(
		WithOpenAIAPIKey("test-api-key"),
		WithOpenAIBaseURL(server.URL),
	)

	resp, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Command != "ls -la" {
		t.Errorf("expected command 'ls -la', got %q", resp.Command)
	}
	if resp.Model != "gpt-5o" {
		t.Errorf("expected model 'gpt-5o', got %q", resp.Model)
	}
	if resp.TokensUsed != 15 {
		t.Errorf("expected 15 tokens used, got %d", resp.TokensUsed)
	}
}

func TestOpenAIBackend_GenerateCommand_WithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openaiRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		// Verify context in system message
		systemMsg := reqBody.Messages[0].Content
		if !strings.Contains(systemMsg, "/home/user") {
			t.Errorf("expected WorkingDir in system prompt")
		}

		resp := openaiResponse{
			Model: "gpt-5o",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{{Message: struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{Content: "pwd"}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenAIBackend(
		WithOpenAIAPIKey("test-api-key"),
		WithOpenAIBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "show current directory",
		Context: &ShellContext{
			WorkingDir: "/home/user",
			Shell:      "bash",
			OS:         "linux",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIBackend_GenerateCommand_NoAPIKey(t *testing.T) {
	b := NewOpenAIBackend()

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestOpenAIBackend_GenerateCommand_EmptyQuery(t *testing.T) {
	b := NewOpenAIBackend(WithOpenAIAPIKey("test-key"))

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "",
	})

	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestOpenAIBackend_GenerateCommand_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Model:   "gpt-5o",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenAIBackend(
		WithOpenAIAPIKey("test-api-key"),
		WithOpenAIBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("expected ErrEmptyResponse, got %v", err)
	}
}

func TestOpenAIBackend_GenerateCommand_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(openaiResponse{})
	}))
	defer server.Close()

	b := NewOpenAIBackend(
		WithOpenAIAPIKey("test-api-key"),
		WithOpenAIBaseURL(server.URL),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.GenerateCommand(ctx, &Request{
		Query: "list files",
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestOpenAIBackend_GenerateCommand_APIError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantErr    string
	}{
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			response:   `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`,
			wantErr:    "API error (401): Invalid API key",
		},
		{
			name:       "429 Rate Limited",
			statusCode: 429,
			response:   `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`,
			wantErr:    "API error (429): Rate limit exceeded",
		},
		{
			name:       "500 Server Error",
			statusCode: 500,
			response:   `{"error":{"message":"Internal server error","type":"server_error"}}`,
			wantErr:    "API error (500): Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			b := NewOpenAIBackend(
				WithOpenAIAPIKey("test-api-key"),
				WithOpenAIBaseURL(server.URL),
			)

			_, err := b.GenerateCommand(context.Background(), &Request{
				Query: "test",
			})

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// =============================================================================
// OpenRouter Backend Tests
// =============================================================================

func TestOpenRouterBackend_Name(t *testing.T) {
	b := NewOpenRouterBackend()
	if got := b.Name(); got != "openrouter" {
		t.Errorf("Name() = %q, want %q", got, "openrouter")
	}
}

func TestOpenRouterBackend_GenerateCommand_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Errorf("expected Authorization header, got %q", got)
		}
		if got := r.Header.Get("HTTP-Referer"); got != "https://github.com/user/qcmd" {
			t.Errorf("expected HTTP-Referer header, got %q", got)
		}
		if got := r.Header.Get("X-Title"); got != "qcmd" {
			t.Errorf("expected X-Title header, got %q", got)
		}

		// Verify request body
		var reqBody openrouterRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if reqBody.Model != "anthropic/claude-4-haiku" {
			t.Errorf("expected model anthropic/claude-4-haiku, got %s", reqBody.Model)
		}

		// Send response
		resp := openrouterResponse{
			ID:    "gen-123",
			Model: "anthropic/claude-4-haiku",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{Role: "assistant", Content: "ls -la"},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{TotalTokens: 20},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenRouterBackend(
		WithOpenRouterAPIKey("test-api-key"),
		WithOpenRouterBaseURL(server.URL),
	)

	resp, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Command != "ls -la" {
		t.Errorf("expected command 'ls -la', got %q", resp.Command)
	}
	if resp.Model != "anthropic/claude-4-haiku" {
		t.Errorf("expected model 'anthropic/claude-4-haiku', got %q", resp.Model)
	}
	if resp.TokensUsed != 20 {
		t.Errorf("expected 20 tokens used, got %d", resp.TokensUsed)
	}
}

func TestOpenRouterBackend_GenerateCommand_CustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify custom headers
		if got := r.Header.Get("HTTP-Referer"); got != "https://custom.example.com" {
			t.Errorf("expected custom HTTP-Referer header, got %q", got)
		}
		if got := r.Header.Get("X-Title"); got != "custom-title" {
			t.Errorf("expected custom X-Title header, got %q", got)
		}

		resp := openrouterResponse{
			Model: "anthropic/claude-4-haiku",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{{Message: struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{Content: "ls"}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenRouterBackend(
		WithOpenRouterAPIKey("test-api-key"),
		WithOpenRouterBaseURL(server.URL),
		WithOpenRouterHTTPReferer("https://custom.example.com"),
		WithOpenRouterXTitle("custom-title"),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenRouterBackend_GenerateCommand_NoAPIKey(t *testing.T) {
	b := NewOpenRouterBackend()

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestOpenRouterBackend_GenerateCommand_EmptyQuery(t *testing.T) {
	b := NewOpenRouterBackend(WithOpenRouterAPIKey("test-key"))

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "",
	})

	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestOpenRouterBackend_GenerateCommand_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openrouterResponse{
			Model:   "anthropic/claude-4-haiku",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenRouterBackend(
		WithOpenRouterAPIKey("test-api-key"),
		WithOpenRouterBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("expected ErrEmptyResponse, got %v", err)
	}
}

func TestOpenRouterBackend_GenerateCommand_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(openrouterResponse{})
	}))
	defer server.Close()

	b := NewOpenRouterBackend(
		WithOpenRouterAPIKey("test-api-key"),
		WithOpenRouterBaseURL(server.URL),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.GenerateCommand(ctx, &Request{
		Query: "list files",
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestOpenRouterBackend_GenerateCommand_APIError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantErr    string
	}{
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			response:   `{"error":{"message":"Invalid API key","type":"auth_error"}}`,
			wantErr:    "API error (401): Invalid API key",
		},
		{
			name:       "402 Payment Required",
			statusCode: 402,
			response:   `{"error":{"message":"Insufficient credits","type":"payment_error"}}`,
			wantErr:    "API error (402): Insufficient credits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			b := NewOpenRouterBackend(
				WithOpenRouterAPIKey("test-api-key"),
				WithOpenRouterBaseURL(server.URL),
			)

			_, err := b.GenerateCommand(context.Background(), &Request{
				Query: "test",
			})

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// =============================================================================
// Functional Options Tests
// =============================================================================

func TestAnthropicBackend_FunctionalOptions(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-key"),
		WithAnthropicBaseURL("https://custom.api.com"),
		WithAnthropicModel("claude-3-opus"),
		WithAnthropicMaxTokens(1024),
		WithAnthropicHTTPClient(client),
	)

	if b.apiKey != "test-key" {
		t.Errorf("expected apiKey 'test-key', got %q", b.apiKey)
	}
	if b.baseURL != "https://custom.api.com" {
		t.Errorf("expected baseURL 'https://custom.api.com', got %q", b.baseURL)
	}
	if b.model != "claude-3-opus" {
		t.Errorf("expected model 'claude-3-opus', got %q", b.model)
	}
	if b.maxTokens != 1024 {
		t.Errorf("expected maxTokens 1024, got %d", b.maxTokens)
	}
	if b.httpClient != client {
		t.Error("expected custom HTTP client")
	}
}

func TestOpenAIBackend_FunctionalOptions(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	b := NewOpenAIBackend(
		WithOpenAIAPIKey("test-key"),
		WithOpenAIBaseURL("https://custom.api.com"),
		WithOpenAIModel("gpt-4-turbo"),
		WithOpenAIMaxTokens(2048),
		WithOpenAIHTTPClient(client),
	)

	if b.apiKey != "test-key" {
		t.Errorf("expected apiKey 'test-key', got %q", b.apiKey)
	}
	if b.baseURL != "https://custom.api.com" {
		t.Errorf("expected baseURL 'https://custom.api.com', got %q", b.baseURL)
	}
	if b.model != "gpt-4-turbo" {
		t.Errorf("expected model 'gpt-4-turbo', got %q", b.model)
	}
	if b.maxTokens != 2048 {
		t.Errorf("expected maxTokens 2048, got %d", b.maxTokens)
	}
	if b.httpClient != client {
		t.Error("expected custom HTTP client")
	}
}

func TestOpenRouterBackend_FunctionalOptions(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	b := NewOpenRouterBackend(
		WithOpenRouterAPIKey("test-key"),
		WithOpenRouterBaseURL("https://custom.api.com"),
		WithOpenRouterModel("openai/gpt-4"),
		WithOpenRouterMaxTokens(4096),
		WithOpenRouterHTTPReferer("https://myapp.com"),
		WithOpenRouterXTitle("MyApp"),
		WithOpenRouterHTTPClient(client),
	)

	if b.apiKey != "test-key" {
		t.Errorf("expected apiKey 'test-key', got %q", b.apiKey)
	}
	if b.baseURL != "https://custom.api.com" {
		t.Errorf("expected baseURL 'https://custom.api.com', got %q", b.baseURL)
	}
	if b.model != "openai/gpt-4" {
		t.Errorf("expected model 'openai/gpt-4', got %q", b.model)
	}
	if b.maxTokens != 4096 {
		t.Errorf("expected maxTokens 4096, got %d", b.maxTokens)
	}
	if b.httpReferer != "https://myapp.com" {
		t.Errorf("expected httpReferer 'https://myapp.com', got %q", b.httpReferer)
	}
	if b.xTitle != "MyApp" {
		t.Errorf("expected xTitle 'MyApp', got %q", b.xTitle)
	}
	if b.httpClient != client {
		t.Error("expected custom HTTP client")
	}
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestBackendInterfaceCompliance(t *testing.T) {
	// Compile-time check that all backends implement the Backend interface
	var _ Backend = (*AnthropicBackend)(nil)
	var _ Backend = (*OpenAIBackend)(nil)
	var _ Backend = (*OpenRouterBackend)(nil)
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestAnthropicBackend_WhitespaceOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			Model: "claude-4-haiku",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "   \n\t  "},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "test",
	})

	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("expected ErrEmptyResponse for whitespace-only response, got %v", err)
	}
}

func TestOpenAIBackend_WhitespaceOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Model: "gpt-5o",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{{Message: struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{Content: "   \n\t  "}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenAIBackend(
		WithOpenAIAPIKey("test-api-key"),
		WithOpenAIBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "test",
	})

	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("expected ErrEmptyResponse for whitespace-only response, got %v", err)
	}
}

func TestOpenRouterBackend_WhitespaceOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openrouterResponse{
			Model: "anthropic/claude-4-haiku",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{{Message: struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{Content: "   \n\t  "}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewOpenRouterBackend(
		WithOpenRouterAPIKey("test-api-key"),
		WithOpenRouterBaseURL(server.URL),
	)

	_, err := b.GenerateCommand(context.Background(), &Request{
		Query: "test",
	})

	if !errors.Is(err, ErrEmptyResponse) {
		t.Errorf("expected ErrEmptyResponse for whitespace-only response, got %v", err)
	}
}

func TestAnthropicBackend_ResponseTrimming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			Model: "claude-4-haiku",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "  ls -la  \n"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	resp, err := b.GenerateCommand(context.Background(), &Request{
		Query: "list files",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Command != "ls -la" {
		t.Errorf("expected trimmed command 'ls -la', got %q", resp.Command)
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		json.NewEncoder(w).Encode(anthropicResponse{})
	}))
	defer server.Close()

	b := NewAnthropicBackend(
		WithAnthropicAPIKey("test-api-key"),
		WithAnthropicBaseURL(server.URL),
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := b.GenerateCommand(ctx, &Request{
		Query: "test",
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
