package app

import (
	"encoding/json"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/agentcontext"
	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/git"
	"github.com/daily-report-daemon/internal/llm"
	"github.com/daily-report-daemon/internal/report"
	"github.com/daily-report-daemon/internal/sanitize"
	"github.com/daily-report-daemon/internal/scanner"
)

// App orchestrates the full pipeline.
type App struct {
	Workspace string
	DryRun    bool
	NoLLM     bool

	// Internal state populated by Scan, consumed by Report/AgentContext in Run()
	evItems    []evidence.Item
	projectMeta *scanner.ProjectMetadata
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
	Errors       []string
}

// Scan runs Git Analyzer + File Scanner + sanitization → evidence.
func (a *App) Scan() (*RunResult, error) {
	result := &RunResult{RunDir: a.runDir()}

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
	result.DiffFiles = len(act.Diffs)

	// Save git activity
	gitPath := filepath.Join(result.RunDir, "git-activity.json")
	if err := git.SaveActivity(act, gitPath); err != nil {
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

	return result, nil
}

// Report generates a daily report from previously scanned evidence.
func (a *App) Report() (*RunResult, error) {
	result := &RunResult{RunDir: a.runDir()}

	// Load evidence (prefer in-memory from Scan, fall back to disk)
	var evBytes []byte
	if len(a.evItems) > 0 {
		data, err := marshalEvidence(a.evItems)
		if err != nil {
			return result, fmt.Errorf("marshal evidence: %w", err)
		}
		evBytes = data
	} else {
		evPath := filepath.Join(result.RunDir, "evidence.jsonl")
		if _, err := os.Stat(evPath); os.IsNotExist(err) {
			return result, fmt.Errorf("no evidence found; run 'scan' first")
		}
		data, err := os.ReadFile(evPath)
		if err != nil {
			return result, fmt.Errorf("read evidence: %w", err)
		}
		evBytes = data
	}

	// Load config
	cfg, err := a.loadConfig()
	if err != nil {
		return result, err
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
	if a.NoLLM {
		fmt.Fprintln(os.Stderr, "[no-llm] Skipping LLM call and report generation")
		return result, nil
	}

	// Generate report via LLM
	sysPrompt := llm.DailyReportSystemPrompt(cfg.Language)
	userPrompt := llm.DailyReportUserPrompt(string(evBytes))

	resp, err := client.Chat(sysPrompt, userPrompt, "daily-report")
	if err != nil {
		return result, fmt.Errorf("LLM call failed: %w", err)
	}

	if resp.DryRun {
		return result, nil
	}

	// Parse and render
	reportJSON, err := llm.ParseReportJSON(resp.Content)
	if err != nil {
		// Degradation: save the raw output and generate a fallback
		dateStr := time.Now().Format("2006-01-02")
		reportDir := filepath.Join(outputDir, "reports")
		content := report.DegradedMarkdown(dateStr, fmt.Sprintf("模型输出解析失败：%v。请查看原始输出。", err))
		path, saveErr := report.SaveReport(content, reportDir, dateStr)
		if saveErr != nil {
			return result, fmt.Errorf("parse failed and save degraded report also failed: %w", saveErr)
		}
		result.Reports = append(result.Reports, path)
		result.Errors = append(result.Errors, fmt.Sprintf("JSON parse degraded: %v", err))
		return result, nil
	}

	issues := llm.ValidateReportJSON(reportJSON)
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
	reportDir := filepath.Join(outputDir, "reports")
	path, err := report.SaveReport(md, reportDir, dateStr)
	if err != nil {
		return result, fmt.Errorf("save report: %w", err)
	}
	result.Reports = append(result.Reports, path)

	return result, nil
}

// AgentContext generates AGENTS.generated.md from scanned data.
func (a *App) AgentContext() (*RunResult, error) {
	result := &RunResult{RunDir: a.runDir()}

	// Use in-memory evidence and metadata from Scan if available
	var evItems []evidence.Item
	var meta *scanner.ProjectMetadata
	if len(a.evItems) > 0 && a.projectMeta != nil {
		evItems = a.evItems
		meta = a.projectMeta
	} else {
		// Fall back to disk
		evPath := filepath.Join(result.RunDir, "evidence.jsonl")
		if _, err := os.Stat(evPath); os.IsNotExist(err) {
			return result, fmt.Errorf("no evidence found; run 'scan' first")
		}
		var err error
		evItems, err = loadEvidenceItems(evPath)
		if err != nil {
			return result, fmt.Errorf("load evidence: %w", err)
		}

		repoRoot, err := git.FindRepoRoot(a.Workspace)
		if err != nil {
			return result, fmt.Errorf("not a git repository: %w", err)
		}
		sc := scanner.NewScanner(repoRoot)
		meta, err = sc.Scan()
		if err != nil {
			return result, fmt.Errorf("scan for context: %w", err)
		}
	}

	repoRoot, err := git.FindRepoRoot(a.Workspace)
	if err != nil {
		return result, fmt.Errorf("not a git repository: %w", err)
	}

	// Generate
	contextDir := filepath.Join(a.Workspace, ".daily-report-daemon", "context")
	g := agentcontext.NewGenerator(repoRoot, contextDir)
	content, err := g.Generate(meta, evItems)
	if err != nil {
		return result, fmt.Errorf("generate context: %w", err)
	}
	path, err := g.Save(content)
	if err != nil {
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
	a.publishReports(result)

	return result, nil
}

func (a *App) publishReports(result *RunResult) {
	cfgPath := filepath.Join(a.Workspace, ".daily-report-daemon", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil || cfg.Publisher.Enabled == false {
		return
	}

	for _, rpt := range result.Reports {
		data, err := os.ReadFile(rpt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[publisher] read report error: %v\n", err)
			continue
		}
		content := string(data)

		// DingTalk
		if cfg.Publisher.PrimaryChannel == "dingtalk" && cfg.Publisher.DingTalk.WebhookURL != "" {
			fmt.Fprintf(os.Stderr, "[publisher] sending to DingTalk...\n")
			if err := a.sendDingTalk(cfg.Publisher.DingTalk.WebhookURL, "日报", content); err != nil {
				fmt.Fprintf(os.Stderr, "[publisher] DingTalk error: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "[publisher] DingTalk sent ✓\n")
			}
		}
	}
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
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
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
	if len(r.Errors) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, e := range r.Errors {
			b.WriteString(fmt.Sprintf("  ⚠ %s\n", e))
		}
	}
	b.WriteString("====================\n")
	return b.String()
}

func (a *App) runDir() string {
	ts := time.Now().Format("2006-01-02-150405")
	return filepath.Join(a.Workspace, ".daily-report-daemon", "runs", ts)
}

func (a *App) loadConfig() (*config.Config, error) {
	cfgPath := filepath.Join(a.Workspace, ".daily-report-daemon", "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no config found; run 'init' first")
	}
	return config.Load(cfgPath)
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
	dec := json.NewDecoder(strings.NewReader(string(data)))
	for dec.More() {
		var item evidence.Item
		if err := dec.Decode(&item); err != nil {
			continue // skip malformed lines
		}
		items = append(items, item)
	}
	return items, nil
}
