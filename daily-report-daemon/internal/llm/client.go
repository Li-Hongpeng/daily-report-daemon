package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client wraps an OpenAI-compatible chat completions endpoint.
type Client struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTPClient *http.Client
	MaxRetries int
	DryRun     bool
	OutputDir  string // where to save model-input.json / model-output.json
}

// NewClient creates a Client from config values.
func NewClient(baseURL, model, apiKeyEnv string, dryRun bool, outputDir string) (*Client, error) {
	key := os.Getenv(apiKeyEnv)
	if key == "" {
		return nil, fmt.Errorf("API key not set: environment variable %s is empty", apiKeyEnv)
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  key,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		MaxRetries: 2,
		DryRun:     dryRun,
		OutputDir:  outputDir,
	}, nil
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the body for /v1/chat/completions.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// ChatResponse is the response from /v1/chat/completions.
type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// ErrorResponse is the error body from the API.
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// CallResult holds both the structured response and metadata.
type CallResult struct {
	Content      string       `json:"-"`
	RawResponse  ChatResponse `json:"raw_response"`
	InputTokens  int          `json:"input_tokens_estimate"`
	OutputTokens int          `json:"output_tokens"`
	DryRun       bool         `json:"dry_run"`
}

// Chat sends a chat completion request and returns the response content.
// On dry-run, it saves the input and returns an empty result without calling the API.
func (c *Client) Chat(systemPrompt, userPrompt, label string) (*CallResult, error) {
	req := ChatRequest{
		Model: c.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   4096,
	}

	// Estimate input tokens (rough: 1 token ≈ 4 chars)
	inputTokens := (len(systemPrompt) + len(userPrompt)) / 4

	// Save model input
	if c.OutputDir != "" {
		if err := c.saveInput(req, label); err != nil {
			// Non-fatal; log but continue
			fmt.Fprintf(os.Stderr, "warn: failed to save model input: %v\n", err)
		}
	}

	if c.DryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] skipping API call for %s\n", label)
		return &CallResult{
			Content:     "",
			InputTokens: inputTokens,
			DryRun:      true,
		}, nil
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 2 * time.Second
			time.Sleep(backoff)
		}

		resp, err := c.doRequest(url, body)
		if err != nil {
			lastErr = err
			// Retry on 5xx; fail fast on 4xx
			if isRetryable(err) {
				continue
			}
			return nil, err
		}
		if resp == nil {
			continue
		}

		// Save raw output
		if c.OutputDir != "" {
			c.saveOutput(resp, label)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no choices in response")
		}

		content := resp.Choices[0].Message.Content
		outputTokens := 0
		if resp.Usage != nil {
			outputTokens = resp.Usage.CompletionTokens
			inputTokens = resp.Usage.PromptTokens
		}

		return &CallResult{
			Content:      content,
			RawResponse:  *resp,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}, nil
	}

	return nil, fmt.Errorf("all %d retries exhausted: %w", c.MaxRetries+1, lastErr)
}

func (c *Client) doRequest(url string, body []byte) (*ChatResponse, error) {
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("API error (status %d): %s", resp.StatusCode, truncateStr(string(respBody), 500))
		// Try to parse structured error
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			errMsg = fmt.Sprintf("API error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    errMsg,
		}
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parse response: %w\nraw: %s", err, truncateStr(string(respBody), 500))
	}

	return &chatResp, nil
}

func (c *Client) saveInput(req ChatRequest, label string) error {
	dir := filepath.Join(c.OutputDir, "model-io")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, label+"-input.json"), data, 0644)
}

func (c *Client) saveOutput(resp *ChatResponse, label string) {
	dir := filepath.Join(c.OutputDir, "model-io")
	os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(resp, "", "  ")
	os.WriteFile(filepath.Join(dir, label+"-output.json"), data, 0644)
}

// APIError represents an HTTP error from the LLM API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}

// Is4xx returns true for client errors (no retry).
func (e *APIError) Is4xx() bool {
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// Is5xx returns true for server errors (retryable).
func (e *APIError) Is5xx() bool {
	return e.StatusCode >= 500
}

func isRetryable(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.Is5xx()
	}
	// Only retry on transient network errors, not DNS/TLS/config errors
	msg := err.Error()
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EOF")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
