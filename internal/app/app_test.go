package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestScanBaselineFiltersPreexistingCommits(t *testing.T) {
	dir := setupAppRepo(t)

	first := &App{Workspace: dir, NoLLM: true}
	firstResult, err := first.Scan()
	if err != nil {
		t.Fatalf("first Scan failed: %v", err)
	}
	firstEvidence := readAppFile(t, firstResult.EvidencePath)
	if strings.Contains(firstEvidence, "initial commit") {
		t.Fatal("first scan should not report commits that are part of the initial baseline")
	}
	if _, err := os.Stat(filepath.Join(dir, ".daily-report-daemon", "baseline.json")); err != nil {
		t.Fatalf("expected baseline file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n\nfunc feature() {}\n"), 0644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	runAppGit(t, dir, "add", "feature.go")
	runAppGit(t, dir, "commit", "-m", "feat: add scan feature")

	second := &App{Workspace: dir, NoLLM: true}
	secondResult, err := second.Scan()
	if err != nil {
		t.Fatalf("second Scan failed: %v", err)
	}
	secondEvidence := readAppFile(t, secondResult.EvidencePath)
	if !strings.Contains(secondEvidence, "feat: add scan feature") {
		t.Fatal("second scan should report commits after the baseline")
	}
	if strings.Contains(secondEvidence, "initial commit") {
		t.Fatal("second scan should keep filtering baseline-era commits")
	}
}

func TestWeeklyReportDryRunUsesCurrentWeekReports(t *testing.T) {
	dir := setupAppRepo(t)
	reportsDir := filepath.Join(dir, ".daily-report-daemon", "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		t.Fatalf("create reports dir: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := os.WriteFile(filepath.Join(reportsDir, today+".md"), []byte("# 日报\n\n完成日报管线测试。\n"), 0644); err != nil {
		t.Fatalf("write daily report: %v", err)
	}
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("DEEPSEEK_API_KEY")

	a := &App{Workspace: dir, DryRun: true}
	result, err := a.WeeklyReport()
	if err != nil {
		t.Fatalf("WeeklyReport dry-run failed: %v", err)
	}
	if len(result.Reports) != 0 {
		t.Fatalf("dry-run should not write weekly report, got %+v", result.Reports)
	}
	inputPath := filepath.Join(dir, ".daily-report-daemon", "model-io", "weekly-report-input.json")
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("expected weekly dry-run model input at %s: %v", inputPath, err)
	}
}

func TestReportOutputDirsHonorConfig(t *testing.T) {
	dir := t.TempDir()
	a := &App{Workspace: dir}
	cfg := config.DefaultConfig(dir)
	cfg.Reports.OutputDir = "custom/daily"
	cfg.Reports.WeeklyOutputDir = "custom/weekly"

	if got := a.dailyReportDir(&cfg); got != filepath.Join(dir, "custom", "daily") {
		t.Fatalf("unexpected daily report dir: %s", got)
	}
	if got := a.weeklyReportDir(&cfg); got != filepath.Join(dir, "custom", "weekly") {
		t.Fatalf("unexpected weekly report dir: %s", got)
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

func readAppFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
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
