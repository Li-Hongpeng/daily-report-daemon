package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClientMissingKey(t *testing.T) {
	os.Unsetenv("OPENAI_API_KEY")
	_, err := NewClient("https://api.openai.com/v1", "gpt-4", "OPENAI_API_KEY", false, "")
	if err == nil {
		t.Error("expected error when API key missing")
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClientWithKey(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	c, err := NewClient("https://api.openai.com/v1", "gpt-4", "OPENAI_API_KEY", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.APIKey != "sk-test" {
		t.Errorf("expected key sk-test, got %s", c.APIKey)
	}
}

func TestDryRun(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	dir := t.TempDir()
	c, err := NewClient("https://api.openai.com/v1", "gpt-4", "OPENAI_API_KEY", true, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := c.Chat("You are helpful.", "Hello", "test-dryrun")
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if !result.DryRun {
		t.Error("expected dry run result")
	}

	inputPath := filepath.Join(dir, "model-io", "test-dryrun-input.json")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Errorf("expected model input file at %s", inputPath)
	}
}

func TestMockServerSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test")
		}
		resp := ChatResponse{
			ID: "chat-123",
			Choices: []Choice{
				{Message: ChoiceMessage{Role: "assistant", Content: `{"date":"2026-05-30","summary":["test"]}`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	dir := t.TempDir()
	c, err := NewClient(server.URL, "gpt-4", "OPENAI_API_KEY", false, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := c.Chat("system", "user", "test-mock")
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if result.Content == "" {
		t.Error("expected content in response")
	}
	if result.DryRun {
		t.Error("expected real (non-dry-run) result")
	}
}

func TestMockServer4xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	os.Setenv("OPENAI_API_KEY", "sk-bad")
	defer os.Unsetenv("OPENAI_API_KEY")
	c, err := NewClient(server.URL, "gpt-4", "OPENAI_API_KEY", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c.MaxRetries = 0

	_, err = c.Chat("system", "user", "test-4xx")
	if err == nil {
		t.Fatal("expected error on 4xx")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestMockServer5xxRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(500)
			return
		}
		resp := ChatResponse{
			ID: "chat-ok",
			Choices: []Choice{
				{Message: ChoiceMessage{Role: "assistant", Content: "ok after retry"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	c, err := NewClient(server.URL, "gpt-4", "OPENAI_API_KEY", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c.MaxRetries = 2

	result, err := c.Chat("system", "user", "test-retry")
	if err != nil {
		t.Fatalf("Chat failed after retries: %v", err)
	}
	if result.Content != "ok after retry" {
		t.Errorf("unexpected content: %s", result.Content)
	}
	if attempts < 3 {
		t.Errorf("expected at least 3 attempts (2 failures + 1 success), got %d", attempts)
	}
}
