package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daily-report-daemon/internal/git"
	"github.com/daily-report-daemon/internal/scanner"
)

func TestGenerateID(t *testing.T) {
	id1 := GenerateID(TypeDiff, "main.go", "unstaged")
	id2 := GenerateID(TypeDiff, "main.go", "unstaged")
	if id1 != id2 {
		t.Error("IDs should be deterministic")
	}
	if !strings.HasPrefix(id1, "diff:main.go:") {
		t.Errorf("unexpected ID format: %s", id1)
	}
}

func TestGenerateIDDifferentInputs(t *testing.T) {
	id1 := GenerateID(TypeDiff, "main.go", "unstaged")
	id2 := GenerateID(TypeDiff, "main.go", "staged")
	if id1 == id2 {
		t.Error("different discriminators should yield different IDs")
	}
}

func TestSaveAndLoadJSONL(t *testing.T) {
	items := []Item{
		{ID: "diff:main.go:abc123", Type: TypeDiff, Workspace: "test"},
		{ID: "commit:def456", Type: TypeCommit, Workspace: "test"},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "evidence.jsonl")
	if err := SaveJSONL(items, path); err != nil {
		t.Fatalf("SaveJSONL failed: %v", err)
	}
	// Verify the file exists and has content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) == 0 {
		t.Error("evidence file is empty")
	}
	// Should have 2 lines
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d", lines)
	}
}

func TestBuilderFromGit(t *testing.T) {
	act := &git.Activity{
		RepoRoot: "/tmp/test",
		Branch:   "main",
		Commits: []git.Commit{
			{Hash: "abc123def456", Author: "test", Date: "2026-05-29", Subject: "fix: bug"},
		},
		Diffs: []git.Diff{
			{
				Scope: "unstaged", File: "main.go", ChangeType: "modified",
				Additions: 5, Deletions: 2, Patch: "func main() {}",
			},
		},
	}

	b := NewBuilder("test-workspace")
	items := b.BuildFromGit(act)

	if len(items) < 2 {
		t.Fatalf("expected at least 2 evidence items, got %d", len(items))
	}

	// Check commit evidence
	foundCommit := false
	for _, item := range items {
		if item.Type == TypeCommit {
			foundCommit = true
			if item.ID == "" {
				t.Error("commit evidence missing ID")
			}
			if !strings.Contains(item.Summary, "fix: bug") {
				t.Error("commit summary missing subject")
			}
		}
	}
	if !foundCommit {
		t.Error("expected commit evidence")
	}
}

func TestBuilderFromGitBlocksSensitivePaths(t *testing.T) {
	act := &git.Activity{
		RepoRoot: "/tmp/test",
		Diffs: []git.Diff{
			{Scope: "unstaged", File: ".env", ChangeType: "modified", Patch: "SECRET=xxx"},
			{Scope: "unstaged", File: "main.go", ChangeType: "modified", Patch: "func main() {}"},
		},
	}

	b := NewBuilder("test-workspace")
	items := b.BuildFromGit(act)

	// .env should be excluded
	for _, item := range items {
		if item.Path == ".env" {
			t.Error(".env should be excluded by sanitizer")
		}
	}
	// main.go should still be there
	foundMain := false
	for _, item := range items {
		if item.Path == "main.go" {
			foundMain = true
		}
	}
	if !foundMain {
		t.Error("expected main.go evidence")
	}
}

func TestBuilderRedactsContent(t *testing.T) {
	act := &git.Activity{
		RepoRoot: "/tmp/test",
		Diffs: []git.Diff{
			{
				Scope: "unstaged", File: "config.go", ChangeType: "modified",
				Patch: `const API_KEY = "sk-thisisafakeapikey1234567890"`,
			},
		},
	}

	b := NewBuilder("test-workspace")
	items := b.BuildFromGit(act)

	for _, item := range items {
		if item.Path == "config.go" {
			if strings.Contains(item.Summary, "sk-thisisafakeapikey") {
				t.Error("API key should be redacted from summary")
			}
		}
	}
}

func TestBuilderFromScanner(t *testing.T) {
	meta := &scanner.ProjectMetadata{
		Root: "/tmp/test",
		KeyFiles: []scanner.KeyFile{
			{Path: "README.md", Name: "README.md", Content: "# Test Project\n"},
			{Path: "go.mod", Name: "go.mod", Content: "module test\n"},
		},
		BuildCommands: []scanner.CommandHint{
			{Command: "go build ./...", Source: "Makefile"},
		},
		TestCommands: []scanner.CommandHint{
			{Command: "go test ./...", Source: "Makefile"},
		},
	}

	b := NewBuilder("test-workspace")
	items := b.BuildFromScanner(meta)

	if len(items) < 3 {
		t.Fatalf("expected at least 3 evidence items, got %d", len(items))
	}

	foundBuild := false
	foundTest := false
	for _, item := range items {
		if item.Type == TypeCommand && strings.Contains(item.Summary, "build") {
			foundBuild = true
		}
		if item.Type == TypeCommand && strings.Contains(item.Summary, "test") {
			foundTest = true
		}
	}
	if !foundBuild {
		t.Error("expected build command evidence")
	}
	if !foundTest {
		t.Error("expected test command evidence")
	}
}
