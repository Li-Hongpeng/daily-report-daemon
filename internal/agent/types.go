package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/report"
)

// RuntimeConfig controls the Eino ADK agent runtime.
type RuntimeConfig struct {
	MaxIterations       int
	MaxToolCalls        int
	ToolTimeoutSeconds  time.Duration
	TotalTimeoutSeconds time.Duration
	TokenBudgetDaily    int
	TraceEnabled        bool
}

// RuntimeConfigFromConfig converts persisted config into runtime settings.
func RuntimeConfigFromConfig(cfg config.AgentConfig) RuntimeConfig {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 8
	}
	if cfg.MaxToolCalls <= 0 {
		cfg.MaxToolCalls = 12
	}
	if cfg.ToolTimeoutSeconds <= 0 {
		cfg.ToolTimeoutSeconds = 30
	}
	if cfg.TotalTimeoutSeconds <= 0 {
		cfg.TotalTimeoutSeconds = 300
	}
	if cfg.TokenBudgetDaily <= 0 {
		cfg.TokenBudgetDaily = 20000
	}
	traceEnabled := true
	if cfg.TraceEnabled != nil {
		traceEnabled = *cfg.TraceEnabled
	}
	return RuntimeConfig{
		MaxIterations:       cfg.MaxIterations,
		MaxToolCalls:        cfg.MaxToolCalls,
		ToolTimeoutSeconds:  time.Duration(cfg.ToolTimeoutSeconds) * time.Second,
		TotalTimeoutSeconds: time.Duration(cfg.TotalTimeoutSeconds) * time.Second,
		TokenBudgetDaily:    cfg.TokenBudgetDaily,
		TraceEnabled:        traceEnabled,
	}
}

// WorkspaceRef identifies one monitored workspace.
type WorkspaceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Date string `json:"date"`
}

// WorkspaceBrief is the structured handoff from a WorkspaceAgent to SupervisorAgent.
type WorkspaceBrief struct {
	Workspace          WorkspaceRef      `json:"workspace"`
	Summary            string            `json:"summary"`
	Completed          []string          `json:"completed"`
	Changes            []WorkspaceChange `json:"changes"`
	Risks              []string          `json:"risks"`
	Blockers           []string          `json:"blockers"`
	NextSteps          []string          `json:"next_steps"`
	EvidenceIDs        []string          `json:"evidence_ids"`
	ToolObservations   []ToolObservation `json:"tool_observations"`
	Confidence         string            `json:"confidence"`
	MemoryUpdates      []MemoryUpdate    `json:"memory_updates"`
	ContextSuggestions []string          `json:"context_suggestions"`
}

// WorkspaceChange describes a concrete workspace-local change.
type WorkspaceChange struct {
	Path        string   `json:"path"`
	Description string   `json:"description"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

// ToolObservation records a sanitized, truncated tool result.
type ToolObservation struct {
	Tool   string `json:"tool"`
	Args   string `json:"args,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// MemoryUpdate is a structured memory write requested by an agent.
type MemoryUpdate struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

// AgentRunResult is the common output envelope for agent-produced assets.
type AgentRunResult struct {
	AssetType       string                   `json:"asset_type"`
	DailyReport     *report.DailyReportJSON  `json:"daily_report,omitempty"`
	WeeklyReport    *report.WeeklyReportJSON `json:"weekly_report,omitempty"`
	ContextMarkdown string                   `json:"context_markdown,omitempty"`
	WorkspaceBriefs []WorkspaceBrief         `json:"workspace_briefs,omitempty"`
	TraceSessionID  string                   `json:"trace_session_id,omitempty"`
	TraceID         string                   `json:"trace_id,omitempty"`
	Warnings        []string                 `json:"warnings,omitempty"`
	FallbackReason  string                   `json:"fallback_reason,omitempty"`
	DryRun          bool                     `json:"dry_run,omitempty"`
}

func workspaceID(path string) string {
	base := filepath.Base(path)
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "workspace"
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", ".", "-")
	base = replacer.Replace(base)
	return fmt.Sprintf("%s-%x", base, fnv32(path))
}

func fnv32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
