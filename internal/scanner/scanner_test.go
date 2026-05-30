package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// setupFixtureDir creates a temporary project directory for testing.
func setupFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	write("README.md", "# My Project\n\nA test project.\n")
	write("go.mod", "module example.com/myproject\n\ngo 1.21\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	write("util.go", "package main\n\nfunc helper() bool { return true }\n")
	write("Makefile", "build:\n\tgo build ./...\ntest:\n\tgo test ./...\n")
	write("node_modules/should-be-ignored.js", "// ignored")
	write(".venv/ignored.py", "# ignored")
	write("dist/bundle.js", "// ignored")
	write("build/output", "ignored")

	return dir
}

func TestScannerBasic(t *testing.T) {
	dir := setupFixtureDir(t)
	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if meta.Root != dir {
		t.Errorf("expected root %s, got %s", dir, meta.Root)
	}
}

func TestScannerKeyFiles(t *testing.T) {
	dir := setupFixtureDir(t)
	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	keyNames := map[string]bool{}
	for _, kf := range meta.KeyFiles {
		keyNames[kf.Name] = true
	}
	if !keyNames["README.md"] {
		t.Error("expected README.md in key files")
	}
	if !keyNames["go.mod"] {
		t.Error("expected go.mod in key files")
	}
	if !keyNames["Makefile"] {
		t.Error("expected Makefile in key files")
	}

	// Verify content
	for _, kf := range meta.KeyFiles {
		if kf.Name == "README.md" && kf.Content == "" {
			t.Error("README.md content should not be empty")
		}
	}
}

func TestScannerIgnoresDirs(t *testing.T) {
	dir := setupFixtureDir(t)
	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	for _, kf := range meta.KeyFiles {
		if kf.Path == "node_modules/should-be-ignored.js" {
			t.Error("node_modules should be ignored")
		}
		if kf.Path == ".venv/ignored.py" {
			t.Error(".venv should be ignored")
		}
	}
}

func TestScannerLanguageStats(t *testing.T) {
	dir := setupFixtureDir(t)
	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have Go files counted
	foundGo := false
	for _, l := range meta.Languages {
		if l.Language == "Go" {
			foundGo = true
			if l.Files < 2 { // main.go + util.go
				t.Errorf("expected at least 2 Go files, got %d", l.Files)
			}
		}
	}
	if !foundGo {
		t.Error("expected Go language stats")
	}
}

func TestScannerCommands(t *testing.T) {
	dir := setupFixtureDir(t)
	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	hasBuild := false
	hasTest := false
	for _, c := range meta.BuildCommands {
		if c.Command == "make build" {
			hasBuild = true
		}
	}
	for _, c := range meta.TestCommands {
		if c.Command == "make test" {
			hasTest = true
		}
	}
	if !hasBuild {
		t.Error("expected make build in build commands")
	}
	if !hasTest {
		t.Error("expected make test in test commands")
	}
}

func TestScannerNoReadme(t *testing.T) {
	dir := t.TempDir()
	// No README, no anything — just a Makefile
	path := filepath.Join(dir, "Makefile")
	os.WriteFile(path, []byte("test:\n\t@echo test\n"), 0644)

	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	// Should not error just because README is missing
	if meta.TotalFiles < 1 {
		t.Error("expected at least 1 file")
	}
}

func TestScannerBinarySkipped(t *testing.T) {
	dir := t.TempDir()
	// Create a PDF — should be counted but content not read
	os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("%PDF-1.4 dummy"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes"), 0644)

	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if meta.TotalFiles < 2 {
		t.Error("expected both files counted")
	}
	// PDF should not be a key file
	for _, kf := range meta.KeyFiles {
		if kf.Name == "report.pdf" {
			t.Error("report.pdf should not be a key file with content")
		}
	}
}

func TestScannerPackageJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "name": "test",
  "scripts": {
    "build": "tsc",
    "test": "jest",
    "lint": "eslint ."
  }
}`), 0644)

	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	hasBuild := false
	hasTest := false
	for _, c := range meta.BuildCommands {
		if c.Command == "npm run build" {
			hasBuild = true
		}
	}
	for _, c := range meta.TestCommands {
		if c.Command == "npm run test" {
			hasTest = true
		}
	}
	if !hasBuild {
		t.Error("expected npm run build")
	}
	if !hasTest {
		t.Error("expected npm run test")
	}
}

func TestScannerTruncation(t *testing.T) {
	dir := t.TempDir()
	// Create a very long README
	content := "# Long README\n\n"
	for i := 0; i < 2000; i++ {
		content += "This is line number " + string(rune('0'+i%10)) + "\n"
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0644)

	s := NewScanner(dir)
	meta, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	for _, kf := range meta.KeyFiles {
		if kf.Name == "README.md" {
			if len(kf.Content) <= MaxKeyFileBytes && kf.Truncated {
				t.Error("untruncated content should not be marked truncated")
			}
			// Content should be under 16KB mark
			if len(kf.Content) > MaxKeyFileBytes+200 { // some buffer for truncation message
				t.Errorf("content too long: %d bytes", len(kf.Content))
			}
		}
	}
}
