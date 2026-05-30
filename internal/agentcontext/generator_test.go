package agentcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/scanner"
)

func fixtureMeta() *scanner.ProjectMetadata {
	return &scanner.ProjectMetadata{
		Root: "/tmp/test-project",
		Structure: []scanner.DirEntry{
			{Name: "cmd", Type: "dir"},
			{Name: "internal", Type: "dir"},
			{Name: "README.md", Type: "file", Size: 100},
			{Name: "go.mod", Type: "file", Size: 50},
			{Name: "Makefile", Type: "file", Size: 200},
		},
		KeyFiles: []scanner.KeyFile{
			{Path: "README.md", Name: "README.md", Content: "# My Project\n\nA test project for daily-report-daemon.\n"},
			{Path: "go.mod", Name: "go.mod", Content: "module example.com/test\n\ngo 1.21\n"},
			{Path: "Makefile", Name: "Makefile", Content: "build:\n\tgo build ./...\ntest:\n\tgo test ./...\nlint:\n\tgolangci-lint run\n"},
			{Path: ".gitignore", Name: ".gitignore", Content: "dist/\nbuild/\n"},
		},
		Languages: []scanner.LangStat{
			{Language: "Go", Files: 15},
			{Language: "Markdown", Files: 3},
		},
		BuildCommands: []scanner.CommandHint{
			{Command: "make build", Source: "Makefile"},
		},
		TestCommands: []scanner.CommandHint{
			{Command: "make test", Source: "Makefile"},
		},
	}
}

func fixtureEvidence() []evidence.Item {
	return []evidence.Item{
		{ID: "diff:main.go:abc", Type: evidence.TypeDiff, Summary: "新增 main.go 入口文件", Source: "git"},
		{ID: "commit:def456", Type: evidence.TypeCommit, Summary: "fix: 修复空 diff 时的 panic", Source: "git"},
	}
}

func TestGenerate(t *testing.T) {
	g := NewGenerator("/tmp/test-project", t.TempDir())
	content, err := g.Generate(fixtureMeta(), fixtureEvidence())
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check required sections
	required := []string{
		"AGENTS.generated.md",
		"Project Overview",
		"Repository Layout",
		"Build, Run, Test Commands",
		"Coding Conventions",
		"Important Files",
		"Today's Activity",
		"Known Risks and Open Questions",
		"Suggested Prompts for Agents",
	}
	for _, s := range required {
		if !strings.Contains(content, s) {
			t.Errorf("missing section: %s", s)
		}
	}

	// Check content
	if !strings.Contains(content, "test project for daily-report-daemon") {
		t.Error("missing project overview")
	}
	if !strings.Contains(content, "make build") {
		t.Error("missing build command")
	}
	if !strings.Contains(content, "make test") {
		t.Error("missing test command")
	}
	if !strings.Contains(content, "cmd") {
		t.Error("missing directory structure")
	}
	if !strings.Contains(content, "make lint") {
		t.Error("missing coding convention from Makefile")
	}

	// Line count
	lines := strings.Count(content, "\n") + 1
	if lines > MaxLines {
		t.Errorf("content is %d lines, max is %d", lines, MaxLines)
	}
}

func TestGenerateNoReadme(t *testing.T) {
	meta := &scanner.ProjectMetadata{
		Root:       "/tmp/noreadme",
		Languages:  []scanner.LangStat{{Language: "Python", Files: 5}},
		KeyFiles:   []scanner.KeyFile{},
	}
	g := NewGenerator("/tmp/noreadme", t.TempDir())
	content, err := g.Generate(meta, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Should not crash and should have a fallback overview
	if !strings.Contains(content, "noreadme") {
		t.Error("missing project name in fallback overview")
	}
	if !strings.Contains(content, "Python") {
		t.Error("missing language in fallback overview")
	}
	// No conventions detected
	if !strings.Contains(content, "No clear conventions detected") {
		t.Error("missing 'no conventions' message")
	}
	if strings.Contains(content, "Coding Conventions\n\n-") {
		t.Error("should not list conventions when none detected")
	}
}

func TestGenerateNoCommands(t *testing.T) {
	meta := &scanner.ProjectMetadata{
		Root: "/tmp/nocmds",
		KeyFiles: []scanner.KeyFile{
			{Path: "README.md", Name: "README.md", Content: "# Test\n\nhello\n"},
		},
	}
	g := NewGenerator("/tmp/nocmds", t.TempDir())
	content, err := g.Generate(meta, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(content, "未检测到构建或测试命令") {
		t.Error("missing fallback for no commands")
	}
}

func TestGenerateWithAgentMD(t *testing.T) {
	dir := t.TempDir()
	// Create AGENTS.md in root
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Manual context\n"), 0644)

	meta := &scanner.ProjectMetadata{Root: dir}
	g := NewGenerator(dir, t.TempDir())
	content, err := g.Generate(meta, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(content, "已存在") || !strings.Contains(content, "AGENTS.md") {
		t.Error("missing human context warning when AGENTS.md exists")
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	g := NewGenerator("/tmp/test", dir)
	path, err := g.Save("# Test content")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if !strings.HasSuffix(path, "AGENTS.generated.md") {
		t.Errorf("unexpected filename: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "# Test content" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestGenerateLineLimitWarning(t *testing.T) {
	meta := fixtureMeta()
	// Add lots of key files to push line count up
	for i := 0; i < 50; i++ {
		meta.KeyFiles = append(meta.KeyFiles, scanner.KeyFile{
			Path: "file.go", Name: "file.go", Content: "// line " + strings.Repeat("x", 100),
		})
	}
	g := NewGenerator("/tmp/test", t.TempDir())
	content, err := g.Generate(meta, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	lines := strings.Count(content, "\n") + 1
	if lines > MaxLines && !strings.Contains(content, "建议拆分") {
		t.Error("expected split suggestion for oversized file")
	}
}

func TestGenerateNoActivity(t *testing.T) {
	meta := &scanner.ProjectMetadata{
		Root: "/tmp/empty",
		KeyFiles: []scanner.KeyFile{
			{Path: "README.md", Name: "README.md", Content: "# Empty\n\nNo activity today.\n"},
		},
	}
	g := NewGenerator("/tmp/empty", t.TempDir())
	content, err := g.Generate(meta, nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(content, "无明显活动") {
		t.Error("missing 'no activity' fallback")
	}
}
