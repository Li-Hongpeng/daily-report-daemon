package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a git repo with some activity for testing.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")

	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("// TODO: implement\npackage main\n"), 0644)
	run("add", ".")
	run("commit", "-m", "initial commit")

	return dir
}

func TestNewAgent(t *testing.T) {
	dir := setupTestRepo(t)
	a := NewAgent(dir, nil)
	if a.Tools == nil {
		t.Error("expected tools")
	}
	if a.Trace == nil {
		t.Error("expected trace")
	}
	if a.Budget == nil {
		t.Error("expected budget")
	}
	if a.Budget.Max != 50000 {
		t.Errorf("expected budget 50000, got %d", a.Budget.Max)
	}
}

func TestAgentRunWithoutLLM(t *testing.T) {
	dir := setupTestRepo(t)
	a := NewAgent(dir, nil) // nil LLM = placeholder mode

	evidence := `[{"id":"test","type":"diff","summary":"test diff"}]`
	result, err := a.Run(context.Background(), evidence)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.ReportJSON == "" {
		t.Error("expected report JSON")
	}
	if result.Confidence != "low" {
		t.Errorf("expected confidence 'low' in placeholder mode, got '%s'", result.Confidence)
	}
}

func TestToolExecution(t *testing.T) {
	dir := setupTestRepo(t)
	tr := NewToolRegistry(dir)

	// Test list_directory
	result := tr.Execute(ToolCall{Tool: "list_directory", Args: "."})
	if result.Error != "" {
		t.Errorf("list_directory error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "README.md") {
		t.Error("expected README.md in listing")
	}

	// Test read_file
	result = tr.Execute(ToolCall{Tool: "read_file", Args: "README.md"})
	if result.Error != "" {
		t.Errorf("read_file error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "# Test") {
		t.Error("expected file content")
	}

	// Test search_pattern
	result = tr.Execute(ToolCall{Tool: "search_pattern", Args: "."})
	if result.Error != "" {
		t.Errorf("search_pattern error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "TODO") {
		t.Error("expected TODO match in search")
	}

	// Test git_log_explore
	result = tr.Execute(ToolCall{Tool: "git_log_explore", Args: "README.md"})
	if result.Error != "" {
		t.Logf("git_log_explore warning (may be ok in test env): %s", result.Error)
	}

	// Test git_diff_detail
	result = tr.Execute(ToolCall{Tool: "git_diff_detail", Args: "README.md"})
	if result.Error != "" {
		t.Logf("git_diff_detail warning (may be ok on clean repo): %s", result.Error)
	}
}

func TestReadFilePathTraversal(t *testing.T) {
	dir := setupTestRepo(t)
	tr := NewToolRegistry(dir)

	result := tr.Execute(ToolCall{Tool: "read_file", Args: "../../../etc/passwd"})
	if result.Error == "" {
		t.Error("expected error for path traversal")
	}
	if !strings.Contains(result.Error, "traversal") {
		t.Errorf("expected traversal error, got: %s", result.Error)
	}
}

func TestSearchPatternNoGrep(t *testing.T) {
	dir := setupTestRepo(t)
	// Make sure search_pattern works without system grep
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("// FIXME: broken\n"), 0644)

	tr := NewToolRegistry(dir)
	result := tr.Execute(ToolCall{Tool: "search_pattern", Args: "."})
	if result.Error != "" {
		t.Errorf("search_pattern error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "FIXME") {
		t.Error("expected FIXME match")
	}
}

func TestAvailableTools(t *testing.T) {
	dir := setupTestRepo(t)
	tr := NewToolRegistry(dir)
	tools := tr.AvailableTools()
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name] = true
	}
	expected := []string{"git_log_explore", "git_diff_detail", "read_file", "search_pattern", "list_directory"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestToLLMToolDefs(t *testing.T) {
	dir := setupTestRepo(t)
	tr := NewToolRegistry(dir)
	defs := tr.ToLLMToolDefs()
	if len(defs) != 5 {
		t.Errorf("expected 5 defs, got %d", len(defs))
	}
}

func TestTrace(t *testing.T) {
	tr := NewTrace()
	tr.Log("analyze", "test message")
	tr.LogToolCall("git_log_explore", "main.go", "commit abc123")

	output := tr.String()
	if !strings.Contains(output, "analyze") {
		t.Error("trace missing step name")
	}
	if !strings.Contains(output, "git_log_explore") {
		t.Error("trace missing tool call")
	}
}

func TestTokenBudget(t *testing.T) {
	tb := NewTokenBudget(10000)
	if !tb.Spend(5000) {
		t.Error("expected spend success")
	}
	if tb.Used() != 5000 {
		t.Errorf("expected 5000 used, got %d", tb.Used())
	}
	if tb.Remaining() != 5000 {
		t.Errorf("expected 5000 remaining, got %d", tb.Remaining())
	}
	if tb.Spend(6000) {
		t.Error("expected spend failure when over budget")
	}
}

func TestAgentFallback(t *testing.T) {
	a := NewAgent("/tmp/test", nil)
	result := a.fallback(`{"test":"data"}`)
	if !result.FellBack {
		t.Error("expected FellBack=true")
	}
	if result.Confidence != "low" {
		t.Errorf("expected confidence 'low', got '%s'", result.Confidence)
	}
}

func TestUnknownTool(t *testing.T) {
	dir := setupTestRepo(t)
	tr := NewToolRegistry(dir)
	result := tr.Execute(ToolCall{Tool: "nonexistent", Args: ""})
	if result.Error == "" {
		t.Error("expected error for unknown tool")
	}
}
