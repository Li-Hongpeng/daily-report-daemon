package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/daily-report-daemon/internal/agentcontext"
	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/llm"
	"github.com/daily-report-daemon/internal/report"
	"github.com/daily-report-daemon/internal/scanner"
	"github.com/daily-report-daemon/internal/store"
)

// Runtime owns SupervisorAgent, WorkspaceAgent, tools, memory, and trace persistence.
type Runtime struct {
	RepoRoot string
	Language string
	Config   RuntimeConfig
	Model    model.BaseChatModel
	Store    *store.Store

	Workspace WorkspaceRef
	Metadata  *scanner.ProjectMetadata
	Evidence  []evidence.Item
	DryRun    bool
}

// NewRuntime creates an Eino-backed runtime from project config and LLM client.
func NewRuntime(repoRoot string, cfg *config.Config, client *llm.Client, st *store.Store, meta *scanner.ProjectMetadata, items []evidence.Item, label string) *Runtime {
	language := "zh-CN"
	var agentCfg config.AgentConfig
	if cfg != nil {
		language = cfg.Language
		agentCfg = cfg.Agent
	}
	ws := WorkspaceRef{
		ID:   workspaceID(repoRoot),
		Name: filepath.Base(repoRoot),
		Path: repoRoot,
		Date: time.Now().Format("2006-01-02"),
	}
	if cfg != nil && cfg.Workspace.Name != "" {
		ws.Name = cfg.Workspace.Name
	}
	return &Runtime{
		RepoRoot:  repoRoot,
		Language:  language,
		Config:    RuntimeConfigFromConfig(agentCfg),
		Model:     NewChatModelAdapter(client, label),
		Store:     st,
		Workspace: ws,
		Metadata:  meta,
		Evidence:  items,
		DryRun:    client != nil && client.DryRun,
	}
}

// NewRuntimeWithModel is used by tests and by future non-OpenAI-compatible adapters.
func NewRuntimeWithModel(repoRoot string, language string, cfg RuntimeConfig, model model.BaseChatModel, st *store.Store, meta *scanner.ProjectMetadata, items []evidence.Item) *Runtime {
	if language == "" {
		language = "zh-CN"
	}
	ws := WorkspaceRef{ID: workspaceID(repoRoot), Name: filepath.Base(repoRoot), Path: repoRoot, Date: time.Now().Format("2006-01-02")}
	if cfg.MaxIterations == 0 {
		cfg = RuntimeConfigFromConfig(config.AgentConfig{})
	}
	return &Runtime{
		RepoRoot:  repoRoot,
		Language:  language,
		Config:    cfg,
		Model:     model,
		Store:     st,
		Workspace: ws,
		Metadata:  meta,
		Evidence:  items,
	}
}

func (r *Runtime) GenerateDaily(ctx context.Context, now time.Time) (*AgentRunResult, error) {
	ctx, cancel := r.totalTimeout(ctx)
	defer cancel()

	result := &AgentRunResult{AssetType: "daily", DryRun: r.DryRun}
	if r.Model == nil {
		brief := fallbackWorkspaceBrief(now, r.workspaceFor(now), r.Evidence, nil)
		result.WorkspaceBriefs = []WorkspaceBrief{brief}
		result.DailyReport = fallbackDailyReport(now, result.WorkspaceBriefs)
		result.FallbackReason = "model unavailable"
		return result, nil
	}
	sessionID := r.startSession("daily")
	result.TraceSessionID = sessionID
	result.TraceID = sessionID
	defer func() {
		r.finishSession(sessionID, 0, 0, result.FallbackReason != "")
	}()

	brief, toolSet, err := r.generateWorkspaceBrief(ctx, now, sessionID)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	if len(toolSet.Observations()) == 0 {
		toolSet.InvokeForFallback(ctx, "summarize_workspace_state", `{}`)
		toolSet.InvokeForFallback(ctx, "read_evidence_index", `{"limit":25}`)
	}
	brief.ToolObservations = mergeObservations(brief.ToolObservations, toolSet.Observations())
	if len(brief.ToolObservations) == 0 {
		result.Warnings = append(result.Warnings, "workspace agent produced no tool observations")
	}
	if !r.DryRun {
		r.applyMemoryUpdates(ctx, toolSet, brief.MemoryUpdates)
		r.persistBriefMemory(brief)
	}
	result.WorkspaceBriefs = []WorkspaceBrief{brief}

	if r.DryRun {
		result.FallbackReason = "dry-run"
		return result, nil
	}

	daily, err := r.generateSupervisorDaily(ctx, now, brief, sessionID)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		result.FallbackReason = "supervisor daily generation failed"
		daily = fallbackDailyReport(now, []WorkspaceBrief{brief})
	}
	if daily.Date == "" {
		daily.Date = now.Format("2006-01-02")
	}
	result.DailyReport = daily
	return result, nil
}

func (r *Runtime) GenerateWeekly(ctx context.Context, now time.Time, dailyReports []string) (*AgentRunResult, error) {
	ctx, cancel := r.totalTimeout(ctx)
	defer cancel()

	result := &AgentRunResult{AssetType: "weekly", DryRun: r.DryRun}
	if r.Model == nil {
		result.WeeklyReport = fallbackWeeklyReport(now, dailyReports)
		result.FallbackReason = "model unavailable"
		return result, nil
	}
	sessionID := r.startSession("weekly")
	result.TraceSessionID = sessionID
	result.TraceID = sessionID
	defer func() {
		r.finishSession(sessionID, 0, 0, result.FallbackReason != "")
	}()

	supervisor, err := r.newSupervisorAgent(ctx, nil)
	if err != nil {
		return nil, err
	}
	prompt := weeklySupervisorPrompt(r.Language, now, dailyReports, r.memorySnapshot())
	content, _, err := r.runAgent(ctx, supervisor, prompt, sessionID, "supervisor_weekly")
	if r.DryRun {
		result.FallbackReason = "dry-run"
		return result, nil
	}
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		result.FallbackReason = "supervisor weekly generation failed"
		result.WeeklyReport = fallbackWeeklyReport(now, dailyReports)
		return result, nil
	}
	weekly, err := report.ParseWeeklyReportJSON(content)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		result.FallbackReason = "weekly JSON parse failed"
		result.WeeklyReport = fallbackWeeklyReport(now, dailyReports)
		return result, nil
	}
	if weekly.Week == "" {
		weekly.Week = report.WeekLabel(now)
	}
	result.WeeklyReport = weekly
	return result, nil
}

func (r *Runtime) GenerateContext(ctx context.Context, now time.Time) (*AgentRunResult, error) {
	ctx, cancel := r.totalTimeout(ctx)
	defer cancel()

	result := &AgentRunResult{AssetType: "context", DryRun: r.DryRun}
	if r.Model == nil {
		content, err := r.fallbackContext()
		if err != nil {
			return result, err
		}
		result.ContextMarkdown = trimMarkdownLines(content, agentcontext.MaxLines)
		result.FallbackReason = "model unavailable"
		return result, nil
	}
	sessionID := r.startSession("context")
	result.TraceSessionID = sessionID
	result.TraceID = sessionID
	defer func() {
		r.finishSession(sessionID, 0, 0, result.FallbackReason != "")
	}()

	toolSet := NewWorkspaceToolSet(r.RepoRoot, r.workspaceFor(now), r.Metadata, r.Evidence, r.Store, r.Config.ToolTimeoutSeconds)
	workspaceAgent, err := r.newWorkspaceAgent(ctx, toolSet, "context")
	if err != nil {
		return nil, err
	}
	content, _, err := r.runAgent(ctx, workspaceAgent, contextPrompt(r.Language, now, r.Workspace), sessionID, "workspace_context")
	if r.DryRun || err != nil || strings.TrimSpace(content) == "" {
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
		if r.DryRun {
			result.FallbackReason = "dry-run"
		} else {
			result.FallbackReason = "workspace context generation failed"
		}
		content, err = r.fallbackContext()
		if err != nil {
			return result, err
		}
	}
	result.ContextMarkdown = trimMarkdownLines(stripCodeFence(content), agentcontext.MaxLines)
	return result, nil
}

func (r *Runtime) generateWorkspaceBrief(ctx context.Context, now time.Time, sessionID string) (WorkspaceBrief, *ToolSet, error) {
	ws := r.workspaceFor(now)
	toolSet := NewWorkspaceToolSet(r.RepoRoot, ws, r.Metadata, r.Evidence, r.Store, r.Config.ToolTimeoutSeconds)
	workspaceAgent, err := r.newWorkspaceAgent(ctx, toolSet, "daily brief")
	if err != nil {
		return fallbackWorkspaceBrief(now, ws, r.Evidence, nil), toolSet, err
	}
	content, _, err := r.runAgent(ctx, workspaceAgent, workspaceBriefPrompt(r.Language, now, ws, r.Evidence), sessionID, "workspace_brief")
	if err != nil {
		return fallbackWorkspaceBrief(now, ws, r.Evidence, toolSet.Observations()), toolSet, err
	}
	brief, err := parseWorkspaceBrief(content)
	if err != nil {
		return fallbackWorkspaceBrief(now, ws, r.Evidence, toolSet.Observations()), toolSet, err
	}
	brief.Workspace = ws
	if brief.Confidence == "" {
		brief.Confidence = "medium"
	}
	brief.ToolObservations = mergeObservations(brief.ToolObservations, toolSet.Observations())
	return brief, toolSet, nil
}

func (r *Runtime) generateSupervisorDaily(ctx context.Context, now time.Time, brief WorkspaceBrief, sessionID string) (*report.DailyReportJSON, error) {
	toolSet := NewWorkspaceToolSet(r.RepoRoot, r.workspaceFor(now), r.Metadata, r.Evidence, r.Store, r.Config.ToolTimeoutSeconds)
	workspaceAgent, err := r.newWorkspaceAgent(ctx, toolSet, "supervisor delegated investigation")
	if err != nil {
		return nil, err
	}
	workspaceTool := adk.NewAgentTool(ctx, workspaceAgent)
	supervisor, err := r.newSupervisorAgent(ctx, []einotool.BaseTool{workspaceTool})
	if err != nil {
		return nil, err
	}
	briefsJSON, _ := json.MarshalIndent([]WorkspaceBrief{brief}, "", "  ")
	prompt := report.DailyReportUserPrompt(string(briefsJSON)) + `

Supervisor instruction:
- You own the final daily report.
- You may call workspace_agent when the brief needs verification.
- The final answer must be only DailyReport JSON.`
	content, _, err := r.runAgent(ctx, supervisor, prompt, sessionID, "supervisor_daily")
	if err != nil {
		return nil, err
	}
	daily, err := report.ParseReportJSON(content)
	if err != nil {
		return nil, err
	}
	return daily, nil
}

func (r *Runtime) newWorkspaceAgent(ctx context.Context, tools *ToolSet, mode string) (*adk.ChatModelAgent, error) {
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "workspace_agent",
		Description: "Investigates one workspace using read-only tools and scoped workspace memory, then returns structured briefs or context.",
		Instruction: workspaceInstruction(r.Language, mode, tools.AllowedNamespaces),
		Model:       r.Model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               tools.Tools(),
				ExecuteSequentially: true,
			},
		},
		MaxIterations: r.Config.MaxIterations,
	})
}

func (r *Runtime) newSupervisorAgent(ctx context.Context, extraTools []einotool.BaseTool) (*adk.ChatModelAgent, error) {
	ws := r.workspaceFor(time.Now())
	supervisorTools := NewSupervisorToolSet(r.RepoRoot, ws, r.Store, r.Config.ToolTimeoutSeconds)
	tools := supervisorTools.Tools()
	tools = append(tools, extraTools...)
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "supervisor_agent",
		Description: "Coordinates workspace agents and produces final daily or weekly reports.",
		Instruction: supervisorInstruction(r.Language, supervisorTools.AllowedNamespaces),
		Model:       r.Model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               tools,
				ExecuteSequentially: true,
			},
			EmitInternalEvents: true,
		},
		MaxIterations: r.Config.MaxIterations,
	})
}

func (r *Runtime) runAgent(ctx context.Context, agent *adk.ChatModelAgent, prompt, sessionID, phase string) (string, []ToolObservation, error) {
	if agent == nil {
		return "", nil, fmt.Errorf("nil agent")
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: false})
	iter := runner.Query(ctx, prompt)
	var last string
	var observations []ToolObservation
	stepOrder := 0
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev == nil {
			continue
		}
		if ev.Err != nil {
			return last, observations, ev.Err
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		msg, err := ev.Output.MessageOutput.GetMessage()
		if err != nil || msg == nil {
			continue
		}
		if ev.Output.MessageOutput.Role == schema.Tool {
			obs := ToolObservation{
				Tool:   ev.Output.MessageOutput.ToolName,
				Output: truncate(msg.Content, maxToolOutput),
			}
			observations = append(observations, obs)
			stepOrder++
			r.insertStep(sessionID, stepOrder, "tool:"+obs.Tool, "", obs.Output, mustJSON(obs))
			continue
		}
		if strings.TrimSpace(msg.Content) != "" {
			last = msg.Content
			stepOrder++
			r.insertStep(sessionID, stepOrder, phase, prompt, last, mustJSON(observations))
		}
	}
	if strings.TrimSpace(last) == "" && !r.DryRun {
		return last, observations, fmt.Errorf("agent returned empty output")
	}
	return last, observations, nil
}

func (r *Runtime) totalTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.Config.TotalTimeoutSeconds <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, r.Config.TotalTimeoutSeconds)
}

func (r *Runtime) workspaceFor(now time.Time) WorkspaceRef {
	ws := r.Workspace
	ws.Date = now.Format("2006-01-02")
	if ws.ID == "" {
		ws.ID = workspaceID(r.RepoRoot)
	}
	if ws.Name == "" {
		ws.Name = filepath.Base(r.RepoRoot)
	}
	if ws.Path == "" {
		ws.Path = r.RepoRoot
	}
	return ws
}

func (r *Runtime) fallbackContext() (string, error) {
	g := agentcontext.NewGenerator(r.RepoRoot, filepath.Join(r.RepoRoot, ".daily-report-daemon", "context"))
	return g.Generate(r.Metadata, r.Evidence)
}

func (r *Runtime) applyMemoryUpdates(ctx context.Context, toolSet *ToolSet, updates []MemoryUpdate) {
	for _, update := range updates {
		if strings.TrimSpace(update.Namespace) == "" || strings.TrimSpace(update.Key) == "" || strings.TrimSpace(update.Value) == "" {
			continue
		}
		payload, _ := json.Marshal(update)
		toolSet.InvokeForFallback(ctx, "write_agent_memory", string(payload))
	}
}

func (r *Runtime) persistBriefMemory(brief WorkspaceBrief) {
	if r.Store == nil {
		return
	}
	briefData, err := json.Marshal(brief)
	if err != nil {
		return
	}
	date := brief.Workspace.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	workspaceKey := fmt.Sprintf("workspace:%s:facts:brief_%s", brief.Workspace.ID, date)
	_ = r.Store.SetAgentMemory(fmt.Sprintf("mem:%x", fnv32(workspaceKey)), workspaceKey, truncate(string(briefData), 4096))
	globalKey := "global:daily_themes:" + date
	_ = r.Store.SetAgentMemory(fmt.Sprintf("mem:%x", fnv32(globalKey)), globalKey, truncate(brief.Summary, 2048))
}

func (r *Runtime) memorySnapshot() string {
	if r.Store == nil {
		return "{}"
	}
	prefixes := []string{
		"workspace:" + r.Workspace.ID + ":facts:",
		"workspace:" + r.Workspace.ID + ":risks:",
		"workspace:" + r.Workspace.ID + ":unfinished:",
		"global:daily_themes:",
		"global:weekly_themes:",
		"global:cross_workspace_risks:",
	}
	var rows []store.AgentMemoryRow
	for _, prefix := range prefixes {
		got, err := r.Store.AgentMemoryByPrefix(prefix)
		if err != nil {
			continue
		}
		rows = append(rows, got...)
	}
	if len(rows) > 50 {
		rows = rows[:50]
	}
	return mustJSON(rows)
}

func (r *Runtime) startSession(assetType string) string {
	id := fmt.Sprintf("agent-%s-%d", assetType, time.Now().UnixNano())
	if r.Store != nil && r.Config.TraceEnabled {
		_ = r.Store.CreateAgentSession(id, assetType)
	}
	return id
}

func (r *Runtime) finishSession(id string, iterations, toolCalls int, fellBack bool) {
	if r.Store != nil && r.Config.TraceEnabled && id != "" {
		status := "completed"
		if fellBack {
			status = "degraded"
		}
		_ = r.Store.FinishAgentSession(id, iterations, toolCalls, 0, status, fellBack)
	}
}

func (r *Runtime) insertStep(sessionID string, order int, stepType, input, output, toolCalls string) {
	if r.Store == nil || !r.Config.TraceEnabled || sessionID == "" {
		return
	}
	if order <= 0 {
		order = 1
	}
	_ = r.Store.InsertAgentStep(store.AgentStepRow{
		ID:            fmt.Sprintf("%s:%03d:%x", sessionID, order, fnv32(stepType+output)),
		SessionID:     sessionID,
		StepType:      truncate(stepType, 80),
		StepOrder:     order,
		InputSummary:  truncate(input, 1000),
		OutputSummary: truncate(output, 2000),
		ToolCalls:     truncate(toolCalls, 2000),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	})
}
