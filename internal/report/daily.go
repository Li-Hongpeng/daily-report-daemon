package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DailyReportJSON is the structured daily report output.
type DailyReportJSON struct {
	Date      string        `json:"date"`
	Summary   []string      `json:"summary"`
	Completed []WorkItem    `json:"completed"`
	Changes   []CodeChange  `json:"changes"`
	Risks     []RiskItem    `json:"risks"`
	Blockers  []BlockerItem `json:"blockers"`
	NextSteps []string      `json:"next_steps"`
}

// WorkItem is a completed task.
type WorkItem struct {
	Description string   `json:"description"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
}

// CodeChange describes a specific code modification.
type CodeChange struct {
	File        string   `json:"file"`
	Description string   `json:"description"`
	Module      string   `json:"module,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
}

// RiskItem flags a potential quality or security concern.
type RiskItem struct {
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
}

// BlockerItem flags a potential blocker.
type BlockerItem struct {
	Description string   `json:"description"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
}

// DailyReportSystemPrompt returns the system prompt for daily report generation.
func DailyReportSystemPrompt(language string) string {
	return fmt.Sprintf(`你是一个技术日报 Supervisor Agent。你会先要求 Workspace Agent 深入调查每个 workspace 的 evidence，再聚合成本日工作汇报。

规则：
1. 最终日报必须由 Supervisor 输出，不能直接复制 Workspace Agent 的原始 brief。
2. 只能使用 evidence、tool observation、workspace brief 和 agent memory 中的信息，不要编造事件、文件或上下文。
3. 每条关键结论必须引用 evidence_ids 或 tool_observations；缺证据的合理推断必须标记 inferred:true。
4. 清晰区分已完成工作、进行中的变更、风险、卡点和建议。
5. 按模块、功能区域或项目维度聚合相关变更。
6. 简洁扼要。summary 限制 3-5 条。
7. 输出语言：%s。
8. 输出必须是符合 DailyReport schema 的有效 JSON。`, language)
}

// DailyReportUserPrompt builds the user prompt from workspace briefs.
func DailyReportUserPrompt(workspaceBriefsJSON string) string {
	return fmt.Sprintf(`Today's date: %s

Workspace briefs and observations:
%s

Output ONLY a JSON object with this structure:
{
  "date": "YYYY-MM-DD",
  "summary": ["one-line overview 1", "one-line overview 2"],
  "completed": [{"description": "...", "evidence_ids": ["id1"], "inferred": false}],
  "changes": [{"file": "path", "description": "...", "module": "area", "evidence_ids": ["id1"], "inferred": false}],
  "risks": [{"description": "...", "severity": "low|medium|high", "evidence_ids": ["id1"], "inferred": false}],
  "blockers": [{"description": "...", "evidence_ids": ["id1"], "inferred": false}],
  "next_steps": ["suggestion 1", "suggestion 2"]
}

If there is little or no activity, generate a minimal but honest report stating no significant activity.
Do NOT include any text outside the JSON.`, time.Now().Format("2006-01-02"), workspaceBriefsJSON)
}

// ValidateReportJSON checks the structured report for required fields and evidence references.
func ValidateReportJSON(report *DailyReportJSON) []string {
	var issues []string
	if report.Date == "" {
		issues = append(issues, "missing date")
	}
	if len(report.Summary) == 0 {
		issues = append(issues, "missing summary — report may be empty or model output was malformed")
	}
	noEvidenceCount := 0
	for _, w := range report.Completed {
		if len(w.EvidenceIDs) == 0 && !w.Inferred {
			noEvidenceCount++
		}
	}
	for _, c := range report.Changes {
		if len(c.EvidenceIDs) == 0 && !c.Inferred {
			noEvidenceCount++
		}
	}
	if noEvidenceCount > 0 {
		issues = append(issues, fmt.Sprintf("%d items have no evidence and are not marked inferred", noEvidenceCount))
	}
	return issues
}

// ParseReportJSON attempts to parse model output into DailyReportJSON.
func ParseReportJSON(raw string) (*DailyReportJSON, error) {
	cleaned := ExtractJSON(raw)
	var report DailyReportJSON
	if err := json.Unmarshal([]byte(cleaned), &report); err != nil {
		return nil, fmt.Errorf("parse report JSON: %w\nraw output (first 500 chars): %s", err, truncForError(raw, 500))
	}
	return &report, nil
}

// ExtractJSON pulls the first JSON object from text that may have markdown fences.
func ExtractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) > 2 && strings.HasPrefix(lines[len(lines)-1], "```") {
			lines = lines[1 : len(lines)-1]
		}
		raw = strings.Join(lines, "\n")
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

func truncForError(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
