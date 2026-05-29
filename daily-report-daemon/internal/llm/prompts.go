package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DailyReportJSON is the structured daily report output.
type DailyReportJSON struct {
	Date      string          `json:"date"`
	Summary   []string        `json:"summary"`
	Completed []WorkItem      `json:"completed"`
	Changes   []CodeChange    `json:"changes"`
	Risks     []RiskItem      `json:"risks"`
	Blockers  []BlockerItem   `json:"blockers"`
	NextSteps []string        `json:"next_steps"`
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
	Severity    string   `json:"severity"` // "low", "medium", "high"
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
	return fmt.Sprintf(`你是一个技术报告生成器。你的任务是根据开发者的 Git 活动和项目文件证据生成开发者日报。

规则：
1. 只能使用提供的证据中的信息。不要编造事件、文件或上下文。
2. 每条关键结论必须在 "evidence_ids" 字段中引用至少一个证据 ID。
3. 如果你必须根据证据做合理推断，将该条目标记为 "inferred": true。
4. 清晰区分已完成工作、进行中的变更、风险、卡点和建议。
5. 按模块或功能区域对相关变更进行分组。
6. 简洁扼要。每条摘要 1-2 句话。
7. 输出语言：%s
8. Tone：专业但有帮助。聚焦进展，不做评判。不评估开发者生产力。
9. summary 限制 3-5 条。
10. 输出必须是符合 DailyReport schema 的有效 JSON。`, language)
}

// DailyReportUserPrompt builds the user prompt from evidence JSON.
func DailyReportUserPrompt(evidenceJSON string) string {
	return fmt.Sprintf(`Generate a daily report from the following evidence.
Today's date: %s

Evidence (JSONL format, one object per line):
%s

Output a JSON object with this structure:
{
  "date": "YYYY-MM-DD",
  "summary": ["one-line overview 1", "one-line overview 2", ...],
  "completed": [{"description": "...", "evidence_ids": ["id1"], "inferred": false}],
  "changes": [{"file": "path", "description": "...", "module": "area", "evidence_ids": ["id1"], "inferred": false}],
  "risks": [{"description": "...", "severity": "low|medium|high", "evidence_ids": ["id1"]}],
  "blockers": [{"description": "...", "evidence_ids": ["id1"]}],
  "next_steps": ["suggestion 1", "suggestion 2"]
}

If there is little or no evidence, generate a minimal but honest report stating no significant activity.
Do NOT include any text outside the JSON. Output ONLY the JSON object.`, time.Now().Format("2006-01-02"), evidenceJSON)
}

// AgentContextPrompt builds the prompt for AGENTS.generated.md generation.
func AgentContextPrompt(projectMeta, gitActivity string) string {
	return fmt.Sprintf(`You are generating a project context file for a coding agent (like Claude Code, Codex, Cursor).
The output is AGENTS.generated.md — a reference that helps coding agents understand this project.

Rules:
1. Extract STABLE information about the project, not daily activity details.
2. Today's activity goes in a short "Today's Activity" section and should reference specific files.
3. If you are uncertain about a command or convention, mark it with "(needs confirmation)".
4. Keep the total output under 200 lines of Markdown.
5. Do NOT write marketing copy — write for a technical audience (coding agents).
6. Do not invent project details not present in the evidence.
7. In Coding Conventions: if no clear conventions detected, state "No clear conventions detected" rather than guessing.

Project metadata:
%s

Recent git activity:
%s

Generate the AGENTS.generated.md content using this structure:

## Project Overview
Brief 2-3 line description of the project.

## Repository Layout
Key directories and their purposes.

## Build, Run, Test Commands
List detected commands. Mark uncertain ones with (needs confirmation).

## Coding Conventions
Any detectable patterns: file naming, package structure, error handling style, etc.

## Important Files
Key configuration and entry point files.

## Today's Activity
Today's changes (brief).

## Known Risks and Open Questions
Any technical debt or unresolved issues visible in the codebase.

## Suggested Prompts for Agents
2-3 sample prompts that a developer might use with a coding agent for this project.

Output ONLY the Markdown content. No preamble, no "here is the file" wrapping.`, projectMeta, gitActivity)
}

// ValidateReportJSON checks the structured report for required fields and evidence references.
// Returns a list of issues; empty slice means valid.
func ValidateReportJSON(report *DailyReportJSON) []string {
	var issues []string
	if report.Date == "" {
		issues = append(issues, "missing date")
	}
	if len(report.Summary) == 0 {
		issues = append(issues, "missing summary — report may be empty or model output was malformed")
	}
	// Count items without evidence
	inferredCount := 0
	noEvidenceCount := 0
	for _, w := range report.Completed {
		if w.Inferred {
			inferredCount++
		}
		if len(w.EvidenceIDs) == 0 && !w.Inferred {
			noEvidenceCount++
		}
	}
	for _, c := range report.Changes {
		if c.Inferred {
			inferredCount++
		}
		if len(c.EvidenceIDs) == 0 && !c.Inferred {
			noEvidenceCount++
		}
	}
	if noEvidenceCount > 0 {
		issues = append(issues, fmt.Sprintf("%d items have no evidence and are not marked inferred", noEvidenceCount))
	}
	if inferredCount > 0 {
		// Not an error, but worth noting
	}
	return issues
}

// ParseReportJSON attempts to parse LLM output into DailyReportJSON.
// It tries to extract JSON from markdown code fences if present.
func ParseReportJSON(raw string) (*DailyReportJSON, error) {
	cleaned := extractJSON(raw)
	var report DailyReportJSON
	if err := json.Unmarshal([]byte(cleaned), &report); err != nil {
		return nil, fmt.Errorf("parse report JSON: %w\nraw output (first 500 chars): %s", err, truncateStr(raw, 500))
	}
	return &report, nil
}

// extractJSON pulls the first JSON object from text that may have markdown fences.
func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip markdown code fences
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) > 2 && strings.HasPrefix(lines[len(lines)-1], "```") {
			lines = lines[1 : len(lines)-1]
		}
		raw = strings.Join(lines, "\n")
	}
	// Find first { and last }
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

// EvidenceToJSONL converts evidence items to a compact JSONL string for the prompt.
func EvidenceToJSONL(items interface{}) string {
	data, err := json.Marshal(items)
	if err != nil {
		return fmt.Sprintf("Error serializing evidence: %v", err)
	}
	return string(data)
}
