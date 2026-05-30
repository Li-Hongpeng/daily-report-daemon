package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/agent"
	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/git"
	"github.com/daily-report-daemon/internal/llm"
	"github.com/daily-report-daemon/internal/report"
	"github.com/daily-report-daemon/internal/sanitize"
	"github.com/daily-report-daemon/internal/scanner"
	"github.com/daily-report-daemon/internal/store"
)

// App orchestrates the full pipeline.
type App struct {
	Workspace string
	DryRun    bool
	NoLLM     bool
	RunDir    string

	// Internal state populated by Scan, consumed by Report/AgentContext in Run()
	evItems     []evidence.Item
	projectMeta *scanner.ProjectMetadata
	baseline    *baselineState
}

// RunResult holds the summary of a run.
type RunResult struct {
	RunDir       string
	FilesScanned int
	DiffFiles    int
	Redactions   int
	Reports      []string
	Contexts     []string
	EvidencePath string
	Publishes    []string
	Errors       []string
}

// Scan runs Git Analyzer + File Scanner + sanitization → evidence.
func (a *App) Scan() (*RunResult, error) {
	result := &RunResult{RunDir: a.ensureRunDir()}

	// 1. Init run dir
	os.MkdirAll(result.RunDir, 0755)

	// 2. Check Git repo
	repoRoot, err := git.FindRepoRoot(a.Workspace)
	if err != nil {
		return result, fmt.Errorf("not a git repository: %w", err)
	}

	// 3. Git analysis
	analyzer := git.NewAnalyzer(repoRoot)
	act, err := analyzer.Collect()
	if err != nil {
		return result, fmt.Errorf("git analysis failed: %w", err)
	}
	cfg, _ := a.loadConfig()
	baseline, err := a.ensureBaseline(repoRoot, act.Head)
	if err != nil {
		return result, fmt.Errorf("baseline: %w", err)
	}
	a.baseline = baseline
	if cfg == nil || !cfg.Reports.IncludeBaseline {
		act.Commits = filterBaselineCommits(repoRoot, act.Commits, baseline)
	}
	result.DiffFiles = len(act.Diffs)

	// Save git activity
	gitPath := filepath.Join(result.RunDir, "git-activity.json")
	if err := git.SaveActivity(sanitizedGitActivity(act), gitPath); err != nil {
		return result, fmt.Errorf("save git activity: %w", err)
	}

	// 4. File scanning
	sc := scanner.NewScanner(repoRoot)
	meta, err := sc.Scan()
	if err != nil {
		return result, fmt.Errorf("file scan failed: %w", err)
	}
	result.FilesScanned = meta.TotalTextFiles

	metaPath := filepath.Join(result.RunDir, "project-metadata.json")
	if err := scanner.SaveMetadata(meta, metaPath); err != nil {
		return result, fmt.Errorf("save metadata: %w", err)
	}

	// 5. Evidence building + sanitization
	san := sanitize.New()
	builder := evidence.NewBuilder(filepath.Base(repoRoot))
	builder.Sanitizer = san

	a.evItems = nil
	a.evItems = append(a.evItems, builder.BuildFromGit(act)...)
	a.evItems = append(a.evItems, builder.BuildFromScanner(meta)...)
	a.projectMeta = meta

	evPath := filepath.Join(result.RunDir, "evidence.jsonl")
	if err := evidence.SaveJSONL(a.evItems, evPath); err != nil {
		return result, fmt.Errorf("save evidence: %w", err)
	}
	result.EvidencePath = evPath

	// Redaction report
	reportPath := filepath.Join(result.RunDir, "redaction-report.json")
	if err := san.SaveReport(reportPath); err != nil {
		return result, fmt.Errorf("save redaction report: %w", err)
	}
	result.Redactions = san.Report().TotalRegex
	result.Redactions += san.Report().TotalBlocks

	return result, nil
}

// Report generates a daily report from previously scanned evidence.
func (a *App) Report() (*RunResult, error) {
	result := &RunResult{RunDir: a.RunDir}

	// Load evidence (prefer in-memory from Scan, fall back to disk)
	var evBytes []byte
	var evItems []evidence.Item
	var meta *scanner.ProjectMetadata
	if len(a.evItems) > 0 {
		result.RunDir = a.ensureRunDir()
		data, err := marshalEvidence(a.evItems)
		if err != nil {
			return result, fmt.Errorf("marshal evidence: %w", err)
		}
		evBytes = data
		evItems = a.evItems
		meta = a.projectMeta
	} else {
		runDir, err := a.runDirWith("evidence.jsonl")
		if err != nil {
			return result, err
		}
		a.RunDir = runDir
		result.RunDir = runDir
		evPath := filepath.Join(result.RunDir, "evidence.jsonl")
		data, err := os.ReadFile(evPath)
		if err != nil {
			return result, fmt.Errorf("read evidence: %w", err)
		}
		evBytes = data
		items, err := loadEvidenceItems(evPath)
		if err != nil {
			return result, fmt.Errorf("load evidence: %w", err)
		}
		evItems = items
	}

	if a.NoLLM {
		fmt.Fprintln(os.Stderr, "[no-llm] Skipping LLM call and report generation")
		return result, nil
	}

	// Load config
	cfg, err := a.loadConfig()
	if err != nil {
		return result, err
	}
	repoRoot, err := git.FindRepoRoot(a.Workspace)
	if err != nil {
		return result, fmt.Errorf("not a git repository: %w", err)
	}
	if meta == nil {
		loaded, loadErr := a.loadProjectMetadata(repoRoot, result.RunDir)
		if loadErr != nil {
			return result, loadErr
		}
		meta = loaded
	}

	// Create LLM client
	outputDir := filepath.Join(a.Workspace, ".daily-report-daemon")
	client, err := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.LLM.APIKeyEnv, a.DryRun, outputDir)
	if err != nil {
		return result, fmt.Errorf("LLM client: %w", err)
	}

	if a.DryRun {
		fmt.Fprintln(os.Stderr, "[dry-run] Skipping LLM call; model input saved to model-io/")
	}

	st, err := a.openStore()
	if err != nil {
		return result, err
	}
	defer st.Close()

	rt := agent.NewRuntime(repoRoot, cfg, client, st, meta, evItems, "daily-report")
	agentResult, err := rt.GenerateDaily(context.Background(), time.Now())
	if err != nil {
		return result, fmt.Errorf("agent daily generation failed: %w", err)
	}
	result.Errors = append(result.Errors, agentResult.Warnings...)
	if agentResult.DryRun {
		return result, nil
	}

	reportJSON := agentResult.DailyReport
	if reportJSON == nil {
		dateStr := time.Now().Format("2006-01-02")
		content := report.DegradedMarkdown(dateStr, "SupervisorAgent 未返回可渲染日报。")
		path, saveErr := report.SaveReport(content, a.dailyReportDir(cfg), dateStr)
		if saveErr != nil {
			return result, fmt.Errorf("save degraded report failed: %w", saveErr)
		}
		result.Reports = append(result.Reports, path)
		result.Errors = append(result.Errors, "agent daily degraded: empty report")
		return result, nil
	}

	issues := report.ValidateReportJSON(reportJSON)
	if len(issues) > 0 {
		result.Errors = append(result.Errors, issues...)
	}

	// Build evidence index from loaded evidence
	evIdx := report.BuildEvidenceIndex(evBytes)

	dateStr := reportJSON.Date
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	md := report.DeveloperMarkdown(reportJSON, evIdx)
	reportDir := a.dailyReportDir(cfg)
	path, err := report.SaveReport(md, reportDir, dateStr)
	if err != nil {
		return result, fmt.Errorf("save report: %w", err)
	}
	result.Reports = append(result.Reports, path)
	_, _ = st.InsertReport("", "daily", dateStr, md, "markdown")

	return result, nil
}

// WeeklyReport generates a weekly report from this week's daily reports.
func (a *App) WeeklyReport() (*RunResult, error) {
	result := &RunResult{RunDir: filepath.Join(a.Workspace, ".daily-report-daemon")}
	cfg, err := a.loadConfig()
	if err != nil {
		return result, err
	}

	now := time.Now()
	dailyReports, err := a.loadCurrentWeekReports(now, cfg)
	if err != nil {
		return result, err
	}
	if len(dailyReports) == 0 {
		return result, fmt.Errorf("no daily reports found for current week; generate daily reports first")
	}

	if a.NoLLM {
		fmt.Fprintln(os.Stderr, "[no-llm] Skipping LLM call and weekly report generation")
		return result, nil
	}

	outputDir := filepath.Join(a.Workspace, ".daily-report-daemon")
	client, err := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.LLM.APIKeyEnv, a.DryRun, outputDir)
	if err != nil {
		return result, fmt.Errorf("LLM client: %w", err)
	}

	repoRoot, err := git.FindRepoRoot(a.Workspace)
	if err != nil {
		return result, fmt.Errorf("not a git repository: %w", err)
	}
	st, err := a.openStore()
	if err != nil {
		return result, err
	}
	defer st.Close()
	rt := agent.NewRuntime(repoRoot, cfg, client, st, nil, nil, "weekly-report")
	agentResult, err := rt.GenerateWeekly(context.Background(), now, dailyReports)
	if err != nil {
		return result, fmt.Errorf("weekly agent generation failed: %w", err)
	}
	result.Errors = append(result.Errors, agentResult.Warnings...)
	if agentResult.DryRun {
		return result, nil
	}

	weeklyJSON := agentResult.WeeklyReport
	if weeklyJSON == nil {
		weeklyJSON = &report.WeeklyReportJSON{Week: report.WeekLabel(now), Summary: "SupervisorAgent 未返回可渲染周报。"}
		result.Errors = append(result.Errors, "agent weekly degraded: empty report")
	}
	if weeklyJSON.Week == "" {
		weeklyJSON.Week = report.WeekLabel(now)
	}
	md := report.WeeklyMarkdown(weeklyJSON)
	path, err := report.SaveWeeklyReport(md, a.weeklyReportDir(cfg), weeklyJSON.Week)
	if err != nil {
		return result, err
	}
	result.Reports = append(result.Reports, path)
	_, _ = st.InsertReport("", "weekly", weeklyJSON.Week, md, "markdown")
	publishes, publishErrors := a.publishReports(result, "周报")
	result.Publishes = append(result.Publishes, publishes...)
	result.Errors = append(result.Errors, publishErrors...)
	return result, nil
}

// AgentContext generates AGENTS.generated.md from scanned data.
func (a *App) AgentContext() (*RunResult, error) {
	result := &RunResult{RunDir: a.RunDir}

	// Use in-memory evidence and metadata from Scan if available
	var evItems []evidence.Item
	var meta *scanner.ProjectMetadata
	if len(a.evItems) > 0 && a.projectMeta != nil {
		result.RunDir = a.ensureRunDir()
		evItems = a.evItems
		meta = a.projectMeta
	} else {
		// Fall back to disk
		runDir, err := a.runDirWith("evidence.jsonl")
		if err != nil {
			return result, err
		}
		a.RunDir = runDir
		result.RunDir = runDir
		evPath := filepath.Join(result.RunDir, "evidence.jsonl")
		evItems, err = loadEvidenceItems(evPath)
		if err != nil {
			return result, fmt.Errorf("load evidence: %w", err)
		}

		repoRoot, err := git.FindRepoRoot(a.Workspace)
		if err != nil {
			return result, fmt.Errorf("not a git repository: %w", err)
		}
		metaPath := filepath.Join(result.RunDir, "project-metadata.json")
		meta, err = scanner.LoadMetadata(metaPath)
		if err != nil {
			sc := scanner.NewScanner(repoRoot)
			meta, err = sc.Scan()
			if err != nil {
				return result, fmt.Errorf("scan for context: %w", err)
			}
		}
	}

	repoRoot, err := git.FindRepoRoot(a.Workspace)
	if err != nil {
		return result, fmt.Errorf("not a git repository: %w", err)
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return result, err
	}
	var client *llm.Client
	if !a.NoLLM {
		outputDir := filepath.Join(a.Workspace, ".daily-report-daemon")
		client, err = llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.LLM.APIKeyEnv, a.DryRun, outputDir)
		if err != nil {
			return result, fmt.Errorf("LLM client: %w", err)
		}
	}
	st, err := a.openStore()
	if err != nil {
		return result, err
	}
	defer st.Close()

	rt := agent.NewRuntime(repoRoot, cfg, client, st, meta, evItems, "agent-context")
	if a.NoLLM {
		rt.Model = nil
		rt.DryRun = true
	}
	agentResult, err := rt.GenerateContext(context.Background(), time.Now())
	if err != nil {
		return result, fmt.Errorf("generate context: %w", err)
	}
	result.Errors = append(result.Errors, agentResult.Warnings...)
	contextDir := filepath.Join(a.Workspace, ".daily-report-daemon", "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		return result, fmt.Errorf("create context dir: %w", err)
	}
	path := filepath.Join(contextDir, "AGENTS.generated.md")
	if err := os.WriteFile(path, []byte(agentResult.ContextMarkdown), 0644); err != nil {
		return result, fmt.Errorf("save context: %w", err)
	}
	result.Contexts = append(result.Contexts, path)

	return result, nil
}

// Run executes the full pipeline: scan → report → agent-context.
func (a *App) Run() (*RunResult, error) {
	result, err := a.Scan()
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		fmt.Fprintf(os.Stderr, "scan error (evidence preserved): %v\n", err)
	}

	if !a.NoLLM {
		reportResult, err := a.Report()
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			fmt.Fprintf(os.Stderr, "report error: %v\n", err)
		} else {
			result.Reports = reportResult.Reports
			result.Errors = append(result.Errors, reportResult.Errors...)
		}
	}

	// Agent context doesn't need LLM, always generate it
	ctxResult, err := a.AgentContext()
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		fmt.Fprintf(os.Stderr, "agent-context error: %v\n", err)
	} else {
		result.Contexts = ctxResult.Contexts
	}

	// Publish to configured channels (DingTalk, email)
	publishes, publishErrors := a.publishReports(result, "日报")
	result.Publishes = append(result.Publishes, publishes...)
	result.Errors = append(result.Errors, publishErrors...)

	return result, nil
}

func (a *App) publishReports(result *RunResult, title string) (publishes, errors []string) {
	cfgPath := filepath.Join(a.Workspace, ".daily-report-daemon", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil || cfg.Publisher.Enabled == false {
		return nil, nil
	}

	for _, rpt := range result.Reports {
		data, err := os.ReadFile(rpt)
		if err != nil {
			msg := fmt.Sprintf("publisher read report error: %v", err)
			fmt.Fprintf(os.Stderr, "[publisher] %s\n", msg)
			errors = append(errors, msg)
			continue
		}
		content := string(data)

		// DingTalk
		if cfg.Publisher.PrimaryChannel == "dingtalk" && cfg.Publisher.DingTalk.WebhookURL != "" {
			if !cfg.Publisher.DingTalk.AutoSend {
				msg := fmt.Sprintf("DingTalk pending review: %s", rpt)
				fmt.Fprintf(os.Stderr, "[publisher] %s\n", msg)
				publishes = append(publishes, msg)
				continue
			}
			fmt.Fprintf(os.Stderr, "[publisher] sending to DingTalk...\n")
			if err := a.sendDingTalk(cfg.Publisher.DingTalk.WebhookURL, title, content); err != nil {
				msg := fmt.Sprintf("DingTalk error: %v", err)
				fmt.Fprintf(os.Stderr, "[publisher] %s\n", msg)
				errors = append(errors, msg)
			} else {
				fmt.Fprintf(os.Stderr, "[publisher] DingTalk sent ✓\n")
				publishes = append(publishes, fmt.Sprintf("DingTalk sent: %s", rpt))
			}
		}
	}
	return publishes, errors
}

func (a *App) sendDingTalk(webhookURL, title, content string) error {
	// Trim content for DingTalk (max 20000 chars)
	if len(content) > 20000 {
		content = content[:20000] + "\n\n... [内容过长已截断]"
	}

	msg := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  fmt.Sprintf("## %s\n\n%s\n\n> 由 daily-report-daemon 自动发送", title, content),
		},
	}
	payload, _ := json.Marshal(msg)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("POST: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("read response: %w", readErr)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, truncateString(string(body), 500))
	}
	var dingResp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &dingResp); err == nil && dingResp.ErrCode != 0 {
		return fmt.Errorf("dingtalk errcode %d: %s", dingResp.ErrCode, dingResp.ErrMsg)
	}
	return nil
}

// Summary returns a human-readable run summary.
func (r *RunResult) Summary() string {
	var b strings.Builder
	b.WriteString("\n=== Run Summary ===\n")
	b.WriteString(fmt.Sprintf("Run directory:   %s\n", r.RunDir))
	b.WriteString(fmt.Sprintf("Files scanned:   %d\n", r.FilesScanned))
	b.WriteString(fmt.Sprintf("Diff files:      %d\n", r.DiffFiles))
	b.WriteString(fmt.Sprintf("Redactions:      %d\n", r.Redactions))

	if r.EvidencePath != "" {
		b.WriteString(fmt.Sprintf("Evidence:        %s\n", r.EvidencePath))
	}
	for _, rpt := range r.Reports {
		b.WriteString(fmt.Sprintf("Report:          %s\n", rpt))
	}
	for _, ctx := range r.Contexts {
		b.WriteString(fmt.Sprintf("Agent context:   %s\n", ctx))
	}
	for _, pub := range r.Publishes {
		b.WriteString(fmt.Sprintf("Publish:         %s\n", pub))
	}
	if len(r.Errors) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, e := range r.Errors {
			b.WriteString(fmt.Sprintf("  ⚠ %s\n", e))
		}
	}
	b.WriteString("====================\n")
	return b.String()
}

func (a *App) ensureRunDir() string {
	if a.RunDir == "" {
		a.RunDir = a.newRunDir()
	}
	return a.RunDir
}

func (a *App) newRunDir() string {
	ts := time.Now().Format("2006-01-02-150405")
	return filepath.Join(a.Workspace, ".daily-report-daemon", "runs", ts)
}

func (a *App) latestRunDirWith(filename string) (string, error) {
	runsDir := filepath.Join(a.Workspace, ".daily-report-daemon", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no evidence found; run 'scan' first")
		}
		return "", fmt.Errorf("read runs dir: %w", err)
	}

	var latest string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(runsDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, filename)); err != nil {
			continue
		}
		if latest == "" || entry.Name() > filepath.Base(latest) {
			latest = candidate
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no evidence found; run 'scan' first")
	}
	return latest, nil
}

func (a *App) runDirWith(filename string) (string, error) {
	if a.RunDir != "" {
		if _, err := os.Stat(filepath.Join(a.RunDir, filename)); err == nil {
			return a.RunDir, nil
		}
	}
	return a.latestRunDirWith(filename)
}

func (a *App) loadCurrentWeekReports(now time.Time, cfg *config.Config) ([]string, error) {
	reportsDir := a.dailyReportDir(cfg)
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reports dir: %w", err)
	}

	weekStart, weekEnd := report.WeekRange(now)
	var reports []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		date := strings.TrimSuffix(entry.Name(), ".md")
		if date < weekStart || date > weekEnd {
			continue
		}
		data, err := os.ReadFile(filepath.Join(reportsDir, entry.Name()))
		if err != nil {
			continue
		}
		reports = append(reports, string(data))
	}
	return reports, nil
}

func (a *App) dailyReportDir(cfg *config.Config) string {
	if cfg != nil && cfg.Reports.OutputDir != "" {
		if filepath.IsAbs(cfg.Reports.OutputDir) {
			return cfg.Reports.OutputDir
		}
		return filepath.Join(a.Workspace, cfg.Reports.OutputDir)
	}
	return filepath.Join(a.Workspace, ".daily-report-daemon", "reports")
}

func (a *App) weeklyReportDir(cfg *config.Config) string {
	if cfg != nil && cfg.Reports.WeeklyOutputDir != "" {
		if filepath.IsAbs(cfg.Reports.WeeklyOutputDir) {
			return cfg.Reports.WeeklyOutputDir
		}
		return filepath.Join(a.Workspace, cfg.Reports.WeeklyOutputDir)
	}
	return filepath.Join(a.Workspace, ".daily-report-daemon", "reports", "weekly")
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func sanitizedGitActivity(act *git.Activity) *git.Activity {
	if act == nil {
		return nil
	}

	san := sanitize.New()
	clean := *act

	clean.Remotes = append([]git.Remote(nil), act.Remotes...)
	for i := range clean.Remotes {
		clean.Remotes[i].URL = san.Redact("git remote", clean.Remotes[i].URL)
	}

	clean.Commits = append([]git.Commit(nil), act.Commits...)
	for i := range clean.Commits {
		clean.Commits[i].Subject = san.Redact(clean.Commits[i].Hash, clean.Commits[i].Subject)
	}

	clean.Status = append([]git.StatusEntry(nil), act.Status...)
	for i := range clean.Status {
		if san.CheckPath(clean.Status[i].Path) {
			clean.Status[i].Path = "[redacted sensitive path]"
		}
	}

	clean.Diffs = append([]git.Diff(nil), act.Diffs...)
	for i := range clean.Diffs {
		if san.CheckPath(clean.Diffs[i].File) {
			clean.Diffs[i].Patch = "[redacted: sensitive path]"
			continue
		}
		clean.Diffs[i].Patch = san.Redact(clean.Diffs[i].File, clean.Diffs[i].Patch)
	}

	return &clean
}

type baselineState struct {
	Head      string `json:"head"`
	CreatedAt string `json:"created_at"`
}

func (a *App) baselinePath() string {
	return filepath.Join(a.Workspace, ".daily-report-daemon", "baseline.json")
}

func (a *App) ensureBaseline(repoRoot, head string) (*baselineState, error) {
	path := a.baselinePath()
	if data, err := os.ReadFile(path); err == nil {
		var baseline baselineState
		if err := json.Unmarshal(data, &baseline); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return &baseline, nil
	}

	baseline := &baselineState{
		Head:      head,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, err
	}
	_ = repoRoot
	return baseline, nil
}

func filterBaselineCommits(repoRoot string, commits []git.Commit, baseline *baselineState) []git.Commit {
	if baseline == nil || baseline.Head == "" {
		return commits
	}
	var filtered []git.Commit
	for _, commit := range commits {
		if commit.Hash == "" {
			continue
		}
		if commit.Hash == baseline.Head || git.IsAncestor(repoRoot, commit.Hash, baseline.Head) {
			continue
		}
		filtered = append(filtered, commit)
	}
	return filtered
}

func (a *App) loadConfig() (*config.Config, error) {
	cfgPath := filepath.Join(a.Workspace, ".daily-report-daemon", "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no config found; run 'init' first")
	}
	return config.Load(cfgPath)
}

func (a *App) openStore() (*store.Store, error) {
	dbPath := filepath.Join(a.Workspace, ".daily-report-daemon", "daemon.db")
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return st, nil
}

func (a *App) loadProjectMetadata(repoRoot, runDir string) (*scanner.ProjectMetadata, error) {
	if runDir != "" {
		metaPath := filepath.Join(runDir, "project-metadata.json")
		if meta, err := scanner.LoadMetadata(metaPath); err == nil {
			return meta, nil
		}
	}
	sc := scanner.NewScanner(repoRoot)
	meta, err := sc.Scan()
	if err != nil {
		return nil, fmt.Errorf("scan project metadata: %w", err)
	}
	return meta, nil
}

func marshalEvidence(items []evidence.Item) ([]byte, error) {
	var lines []string
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		lines = append(lines, string(data))
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func loadEvidenceItems(path string) ([]evidence.Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []evidence.Item
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item evidence.Item
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue // skip malformed lines
		}
		items = append(items, item)
	}
	return items, nil
}
