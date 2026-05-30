package report

import (
	"strings"
	"testing"
	"time"
)

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
	if !strings.Contains(user, time.Now().Format("2006-01-02")) {
		t.Error("expected date in user prompt")
	}
	if !strings.Contains(user, `"id":"test"`) {
		t.Error("expected workspace brief in user prompt")
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
		got := ExtractJSON(tt.input)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("ExtractJSON(%q) = %q, want contains %q", tt.input, got, tt.contains)
		}
	}
}
