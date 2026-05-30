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

func TestParseReportJSON(t *testing.T) {
	raw := `{"date":"2026-05-30","summary":["Worked on feature X"],"completed":[],"changes":[],"risks":[],"blockers":[],"next_steps":[]}`
	report, err := ParseReportJSON(raw)
	if err != nil {
		t.Fatalf("ParseReportJSON failed: %v", err)
	}
	if report.Date != "2026-05-30" {
		t.Errorf("expected date 2026-05-30, got %s", report.Date)
	}
}

func TestParseReportJSONWithFences(t *testing.T) {
	raw := "```json\n{\"date\":\"2026-05-30\",\"summary\":[\"test\"],\"completed\":[],\"changes\":[],\"risks\":[],\"blockers\":[],\"next_steps\":[]}\n```"
	report, err := ParseReportJSON(raw)
	if err != nil {
		t.Fatalf("ParseReportJSON failed: %v", err)
	}
	if len(report.Summary) != 1 || report.Summary[0] != "test" {
		t.Errorf("unexpected summary: %v", report.Summary)
	}
}

func TestParseReportJSONMalformed(t *testing.T) {
	_, err := ParseReportJSON("not json at all")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse report JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateReportJSON(t *testing.T) {
	report := &DailyReportJSON{
		Date:    "2026-05-30",
		Summary: []string{"did stuff"},
		Completed: []WorkItem{
			{Description: "fixed bug", EvidenceIDs: []string{"diff:main.go:abc"}, Inferred: false},
		},
	}
	issues := ValidateReportJSON(report)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got: %v", issues)
	}

	report2 := &DailyReportJSON{
		Date:    "2026-05-30",
		Summary: []string{"did stuff"},
		Completed: []WorkItem{
			{Description: "fixed bug", EvidenceIDs: nil, Inferred: false},
		},
	}
	issues2 := ValidateReportJSON(report2)
	if len(issues2) == 0 {
		t.Error("expected issues for item with no evidence")
	}

	report3 := &DailyReportJSON{
		Date:    "2026-05-30",
		Summary: []string{"did stuff"},
		Completed: []WorkItem{
			{Description: "probably fixed bug", EvidenceIDs: nil, Inferred: true},
		},
	}
	issues3 := ValidateReportJSON(report3)
	if len(issues3) != 0 {
		t.Errorf("inferred items should not trigger issues, got: %v", issues3)
	}
}

func TestDailyReportPrompts(t *testing.T) {
	sys := DailyReportSystemPrompt("zh-CN")
	if !strings.Contains(sys, "zh-CN") {
		t.Error("expected zh-CN in system prompt")
	}

	user := DailyReportUserPrompt(`[{"id":"test"}]`)
	if !strings.Contains(user, "2026-05-30") {
		t.Error("expected date in user prompt")
	}
	if !strings.Contains(user, `"id":"test"`) {
		t.Error("expected evidence in user prompt")
	}
}

func TestAgentContextPrompt(t *testing.T) {
	prompt := AgentContextPrompt("project meta here", "git activity here")
	if !strings.Contains(prompt, "AGENTS.generated.md") {
		t.Error("expected AGENTS.generated.md reference")
	}
	if !strings.Contains(prompt, "project meta here") {
		t.Error("expected project meta in prompt")
	}
	if !strings.Contains(prompt, "git activity here") {
		t.Error("expected git activity in prompt")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{`{"key":"value"}`, `{"key":"value"}`},
		{"```json\n{\"key\":\"value\"}\n```", `{"key":"value"}`},
		{"Some text before {\"key\":\"value\"}", `{"key":"value"}`},
	}
	for _, tt := range tests {
		got := extractJSON(tt.input)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("extractJSON(%q) = %q, want contains %q", tt.input, got, tt.contains)
		}
	}
}
