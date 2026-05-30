package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daily-report-daemon/internal/config"
)

func TestReportUsesLatestScanEvidence(t *testing.T) {
	dir := setupAppRepo(t)

	scanApp := &App{Workspace: dir, NoLLM: true}
	scanResult, err := scanApp.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	reportApp := &App{Workspace: dir, NoLLM: true}
	reportResult, err := reportApp.Report()
	if err != nil {
		t.Fatalf("Report failed: %v", err)
	}
	if reportResult.RunDir != scanResult.RunDir {
		t.Fatalf("expected report to use latest scan dir %s, got %s", scanResult.RunDir, reportResult.RunDir)
	}
}

func TestDryRunReportDoesNotRequireAPIKey(t *testing.T) {
	dir := setupAppRepo(t)
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("DEEPSEEK_API_KEY")

	a := &App{Workspace: dir, NoLLM: true}
	if _, err := a.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	reportApp := &App{Workspace: dir, DryRun: true}
	if _, err := reportApp.Report(); err != nil {
		t.Fatalf("dry-run Report failed without API key: %v", err)
	}

	inputPath := filepath.Join(dir, ".daily-report-daemon", "model-io", "daily-report-input.json")
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("expected dry-run model input at %s: %v", inputPath, err)
	}
}

func TestScanWritesSanitizedGitActivityAndEvidence(t *testing.T) {
	dir := setupAppRepo(t)
	secret := "sk-thisisafakeapikey1234567890"
	if err := os.WriteFile(filepath.Join(dir, "config.go"), []byte(`package main

const APIKey = "`+secret+`"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	a := &App{Workspace: dir, NoLLM: true}
	result, err := a.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	for _, path := range []string{
		filepath.Join(result.RunDir, "git-activity.json"),
		result.EvidencePath,
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(data) == "" {
			t.Fatalf("%s should not be empty", path)
		}
		if strings.Contains(string(data), secret) {
			t.Fatalf("%s leaked secret", path)
		}
		if !strings.Contains(string(data), "[REDACTED]") {
			t.Fatalf("%s should contain redaction marker", path)
		}
	}
}

func setupAppRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runAppGit(t, dir, "init")
	runAppGit(t, dir, "config", "user.email", "test@test.com")
	runAppGit(t, dir, "config", "user.name", "test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runAppGit(t, dir, "add", "README.md")
	runAppGit(t, dir, "commit", "-m", "initial commit")

	cfg := config.DefaultConfig(dir)
	if err := cfg.Save(filepath.Join(dir, ".daily-report-daemon", "config.yaml")); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return dir
}

func runAppGit(t *testing.T, dir string, args ...string) {
	t.Helper()
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
