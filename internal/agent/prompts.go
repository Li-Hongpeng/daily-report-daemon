package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/report"
)

func workspaceInstruction(language, mode string, namespaces []string) string {
	return fmt.Sprintf(`You are WorkspaceAgent for exactly one workspace.

Mode: %s
Output language: %s

Rules:
- Investigate with tools before making conclusions. Prefer read_evidence_index and summarize_workspace_state first.
- Tools are read-only except write_agent_memory.
- Never read .daily-report-daemon, .git internals, secrets, token files, webhook files, or paths outside the workspace.
- Keep memory writes scoped to these namespaces only: %s.
- Cite evidence_ids and tool_observations for important conclusions.
- If information is insufficient, say so and lower confidence. Do not invent project facts.`, mode, language, strings.Join(namespaces, ", "))
}

func supervisorInstruction(language string, namespaces []string) string {
	return fmt.Sprintf(`You are SupervisorAgent for daily-report-daemon.

Output language: %s

Responsibilities:
- Coordinate WorkspaceAgent outputs and own the final daily/weekly narrative.
- Daily reports must be synthesized by you from WorkspaceBriefs, evidence, tool observations, and memory.
- Weekly reports must identify cross-day trends; if data is thin, explicitly say information is insufficient.
- Use only global memory namespaces: %s.
- Do not expose secrets, webhooks, tokens, or local daemon internals.
- Output exactly the requested JSON schema with no prose outside JSON.`, language, strings.Join(namespaces, ", "))
}

func workspaceBriefPrompt(language string, now time.Time, ws WorkspaceRef, items []evidence.Item) string {
	index := evidenceIndexForPrompt(items, 80)
	return fmt.Sprintf(`Date: %s
Workspace:
%s

Sanitized evidence index:
%s

Task:
1. Call read_evidence_index and summarize_workspace_state.
2. Use git_diff_detail/read_file/search_pattern/list_directory only when needed to clarify evidence.
3. Return ONLY WorkspaceBrief JSON:
{
  "workspace": {"id":"...","name":"...","path":"...","date":"YYYY-MM-DD"},
  "summary": "concise workspace-local summary",
  "completed": ["done item"],
  "changes": [{"path":"relative/file","description":"what changed","evidence_ids":["id"]}],
  "risks": ["risk"],
  "blockers": ["blocker"],
  "next_steps": ["next step"],
  "evidence_ids": ["id"],
  "tool_observations": [{"tool":"read_evidence_index","args":"...","output":"short observation"}],
  "confidence": "high|medium|low",
  "memory_updates": [{"namespace":"workspace:%s:facts","key":"short-key","value":"JSON or concise text"}],
  "context_suggestions": ["things AGENTS.generated.md should remember"]
}

Output language: %s.`, now.Format("2006-01-02"), mustJSON(ws), index, ws.ID, language)
}

func weeklySupervisorPrompt(language string, now time.Time, dailyReports []string, memorySnapshot string) string {
	return report.WeeklyReportSystemPrompt(language) + "\n\n" + report.WeeklyReportUserPrompt(dailyReports, now) + `

Agent memory snapshot:
` + memorySnapshot + `

Supervisor instruction:
- This weekly report must not be a concatenation of daily reports.
- Include cross-day trends, repeated risks, or an explicit information-insufficient statement.
- Output ONLY WeeklyReport JSON.`
}

func contextPrompt(language string, now time.Time, ws WorkspaceRef) string {
	return fmt.Sprintf(`Date: %s
Workspace:
%s

Generate AGENTS.generated.md for this workspace only.

Required behavior:
1. Call summarize_workspace_state and read_evidence_index first.
2. Use read_file only for project configuration or README-like files when needed.
3. Keep the markdown under 200 lines.
4. Include current project overview, repo layout, build/test commands, coding conventions, important files, today's activity, risks/open questions, and suggested prompts.
5. Do not include secrets, webhooks, tokens, .git, or .daily-report-daemon content.
6. Output markdown only, no code fences.

Output language: %s.`, now.Format("2006-01-02"), mustJSON(ws), language)
}

func parseWorkspaceBrief(raw string) (WorkspaceBrief, error) {
	var brief WorkspaceBrief
	clean := report.ExtractJSON(raw)
	if err := json.Unmarshal([]byte(clean), &brief); err != nil {
		return brief, fmt.Errorf("parse workspace brief JSON: %w", err)
	}
	return brief, nil
}

func evidenceIndexForPrompt(items []evidence.Item, limit int) string {
	if limit <= 0 || limit > 100 {
		limit = 80
	}
	type ev struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Path    string `json:"path,omitempty"`
		Summary string `json:"summary"`
	}
	out := make([]ev, 0, minInt(len(items), limit))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		out = append(out, ev{
			ID:      item.ID,
			Type:    string(item.Type),
			Path:    item.Path,
			Summary: item.Summary,
		})
	}
	return mustJSON(out)
}

func mustJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return s
}

func trimMarkdownLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return strings.TrimSpace(s)
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n") + "\n"
	}
	return strings.Join(lines[:maxLines], "\n") + fmt.Sprintf("\n\n---\n本文件已按 %d 行上限截断。\n", maxLines)
}
