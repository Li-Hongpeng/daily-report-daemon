package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/scanner"
	"github.com/daily-report-daemon/internal/store"
)

type scriptedModel struct {
	mu      sync.Mutex
	outputs []string
}

func (m *scriptedModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.outputs) == 0 {
		return schema.AssistantMessage(`{"summary":"empty"}`, nil), nil
	}
	out := m.outputs[0]
	m.outputs = m.outputs[1:]
	return schema.AssistantMessage(out, nil), nil
}

func (m *scriptedModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func TestToolSetBlocksTraversalAndDaemonData(t *testing.T) {
	dir := setupAgentRepo(t)
	ws := WorkspaceRef{ID: "ws", Name: "repo", Path: dir, Date: "2026-05-30"}
	ts := NewWorkspaceToolSet(dir, ws, nil, nil, nil, time.Second)

	if obs := ts.InvokeForFallback(context.Background(), "read_file", `{"path":"../../../etc/passwd"}`); obs.Error == "" {
		t.Fatal("expected traversal to be blocked")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".daily-report-daemon"), 0755); err != nil {
		t.Fatalf("mkdir daemon dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".daily-report-daemon", "config.yaml"), []byte("secret: x"), 0644); err != nil {
		t.Fatalf("write daemon config: %v", err)
	}
	if obs := ts.InvokeForFallback(context.Background(), "read_file", `{"path":".daily-report-daemon/config.yaml"}`); obs.Error == "" {
		t.Fatal("expected daemon data to be blocked")
	}
}

func TestToolSetRedactsSecrets(t *testing.T) {
	dir := setupAgentRepo(t)
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte(secret), 0644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	ws := WorkspaceRef{ID: "ws", Name: "repo", Path: dir, Date: "2026-05-30"}
	ts := NewWorkspaceToolSet(dir, ws, nil, nil, nil, time.Second)
	obs := ts.InvokeForFallback(context.Background(), "read_file", `{"path":"config.txt"}`)
	if obs.Error != "" {
		t.Fatalf("read_file error: %s", obs.Error)
	}
	if strings.Contains(obs.Output, secret) {
		t.Fatal("tool output leaked secret")
	}
	if !strings.Contains(obs.Output, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", obs.Output)
	}
}

func TestMemoryNamespaceIsolation(t *testing.T) {
	dir := setupAgentRepo(t)
	st, err := store.Open(filepath.Join(t.TempDir(), "daemon.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ws := WorkspaceRef{ID: "alpha", Name: "repo", Path: dir, Date: "2026-05-30"}
	ts := NewWorkspaceToolSet(dir, ws, nil, nil, st, time.Second)

	okArgs := `{"namespace":"workspace:alpha:facts","key":"build","value":"{\"cmd\":\"go test ./...\"}"}`
	if obs := ts.InvokeForFallback(context.Background(), "write_agent_memory", okArgs); obs.Error != "" {
		t.Fatalf("expected memory write success, got %s", obs.Error)
	}
	badArgs := `{"namespace":"workspace:beta:facts","key":"build","value":"x"}`
	if obs := ts.InvokeForFallback(context.Background(), "write_agent_memory", badArgs); obs.Error == "" {
		t.Fatal("expected cross-workspace memory write to be blocked")
	}
}

func TestRuntimeDailyUsesSupervisorOutputAndWorkspaceBrief(t *testing.T) {
	dir := setupAgentRepo(t)
	now := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	items := []evidence.Item{
		{ID: "diff:main.go:abcd", Type: evidence.TypeDiff, Path: "main.go", Summary: "更新 main.go 的启动逻辑", Source: "git"},
	}
	meta := &scanner.ProjectMetadata{Root: dir, Languages: []scanner.LangStat{{Language: "Go", Files: 1}}}
	model := &scriptedModel{outputs: []string{
		`{"workspace":{"id":"x","name":"repo","path":"` + dir + `","date":"2026-05-30"},"summary":"完成启动逻辑调整","completed":["完成启动逻辑调整"],"changes":[{"path":"main.go","description":"更新 main.go","evidence_ids":["diff:main.go:abcd"]}],"risks":[],"blockers":[],"next_steps":["补充测试"],"evidence_ids":["diff:main.go:abcd"],"tool_observations":[],"confidence":"medium","memory_updates":[],"context_suggestions":[]}`,
		`{"date":"2026-05-30","summary":["Supervisor 聚合 workspace brief 形成日报"],"completed":[{"description":"完成启动逻辑调整","evidence_ids":["diff:main.go:abcd"]}],"changes":[{"file":"main.go","description":"更新 main.go","module":"root","evidence_ids":["diff:main.go:abcd"]}],"risks":[],"blockers":[],"next_steps":["补充测试"]}`,
	}}
	st, err := store.Open(filepath.Join(t.TempDir(), "daemon.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	rt := NewRuntimeWithModel(dir, "zh-CN", RuntimeConfigFromConfig(config.AgentConfig{}), model, st, meta, items)

	result, err := rt.GenerateDaily(context.Background(), now)
	if err != nil {
		t.Fatalf("GenerateDaily: %v", err)
	}
	if result.DailyReport == nil {
		t.Fatal("expected daily report")
	}
	if got := result.DailyReport.Summary[0]; !strings.Contains(got, "Supervisor") {
		t.Fatalf("expected supervisor output to win, got %q", got)
	}
	if len(result.WorkspaceBriefs) != 1 {
		t.Fatalf("expected one workspace brief, got %d", len(result.WorkspaceBriefs))
	}
	if len(result.WorkspaceBriefs[0].ToolObservations) == 0 {
		t.Fatal("expected at least one workspace tool observation")
	}
	var stepCount int
	if err := st.DB().QueryRow("SELECT COUNT(*) FROM agent_steps WHERE session_id = ?", result.TraceSessionID).Scan(&stepCount); err != nil {
		t.Fatalf("count agent steps: %v", err)
	}
	if stepCount == 0 {
		t.Fatal("expected Eino event stream to be persisted into agent_steps")
	}
	memoryRows, err := st.AgentMemoryByPrefix("workspace:" + result.WorkspaceBriefs[0].Workspace.ID + ":facts:")
	if err != nil {
		t.Fatalf("AgentMemoryByPrefix: %v", err)
	}
	if len(memoryRows) == 0 {
		t.Fatal("expected workspace brief memory to be persisted")
	}
}

func setupAgentRepo(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	run("add", "main.go")
	run("commit", "-m", "initial")
	return dir
}
