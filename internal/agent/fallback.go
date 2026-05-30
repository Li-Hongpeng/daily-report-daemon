package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/report"
)

func fallbackWorkspaceBrief(now time.Time, ws WorkspaceRef, items []evidence.Item, observations []ToolObservation) WorkspaceBrief {
	brief := WorkspaceBrief{
		Workspace:        ws,
		Summary:          "信息不足，WorkspaceAgent 基于脱敏 evidence 生成保守摘要。",
		Confidence:       "low",
		ToolObservations: observations,
	}
	seenEvidence := map[string]bool{}
	for _, item := range items {
		if item.ID != "" && !seenEvidence[item.ID] {
			brief.EvidenceIDs = append(brief.EvidenceIDs, item.ID)
			seenEvidence[item.ID] = true
		}
		switch item.Type {
		case evidence.TypeCommit:
			brief.Completed = appendLimit(brief.Completed, item.Summary, 8)
		case evidence.TypeDiff, evidence.TypeFileChange:
			brief.Changes = appendChangeLimit(brief.Changes, WorkspaceChange{
				Path:        item.Path,
				Description: item.Summary,
				EvidenceIDs: compactIDs(item.ID),
			}, 12)
		case evidence.TypeTodo:
			brief.NextSteps = appendLimit(brief.NextSteps, item.Summary, 6)
		}
		if item.Sensitivity == evidence.SensMedium || item.Sensitivity == evidence.SensHigh {
			brief.Risks = appendLimit(brief.Risks, item.Summary, 6)
		}
	}
	if len(brief.EvidenceIDs) == 0 {
		brief.Summary = "未发现可用于生成日报的有效 evidence。"
	}
	if len(brief.Completed) == 0 && len(brief.Changes) == 0 && len(brief.NextSteps) == 0 {
		brief.NextSteps = append(brief.NextSteps, "继续观察工作目录变化，等待更多可验证证据。")
	}
	brief.Workspace.Date = now.Format("2006-01-02")
	return brief
}

func fallbackDailyReport(now time.Time, briefs []WorkspaceBrief) *report.DailyReportJSON {
	daily := &report.DailyReportJSON{
		Date: now.Format("2006-01-02"),
		Summary: []string{
			"SupervisorAgent 未获得完整模型输出，已基于 WorkspaceBrief 生成保守日报。",
		},
	}
	for _, brief := range briefs {
		if strings.TrimSpace(brief.Summary) != "" && len(daily.Summary) < 5 {
			daily.Summary = append(daily.Summary, fmt.Sprintf("%s：%s", brief.Workspace.Name, brief.Summary))
		}
		for _, done := range brief.Completed {
			daily.Completed = append(daily.Completed, report.WorkItem{
				Description: done,
				EvidenceIDs: brief.EvidenceIDs,
				Inferred:    len(brief.EvidenceIDs) == 0,
			})
		}
		for _, change := range brief.Changes {
			ids := change.EvidenceIDs
			if len(ids) == 0 {
				ids = brief.EvidenceIDs
			}
			daily.Changes = append(daily.Changes, report.CodeChange{
				File:        change.Path,
				Module:      moduleFromPath(change.Path),
				Description: change.Description,
				EvidenceIDs: ids,
				Inferred:    len(ids) == 0,
			})
		}
		for _, risk := range brief.Risks {
			daily.Risks = append(daily.Risks, report.RiskItem{
				Description: risk,
				Severity:    "medium",
				EvidenceIDs: brief.EvidenceIDs,
				Inferred:    len(brief.EvidenceIDs) == 0,
			})
		}
		for _, blocker := range brief.Blockers {
			daily.Blockers = append(daily.Blockers, report.BlockerItem{
				Description: blocker,
				EvidenceIDs: brief.EvidenceIDs,
				Inferred:    len(brief.EvidenceIDs) == 0,
			})
		}
		daily.NextSteps = append(daily.NextSteps, brief.NextSteps...)
	}
	if len(daily.Completed) == 0 && len(daily.Changes) == 0 && len(daily.Risks) == 0 {
		daily.Summary = append(daily.Summary, "今日没有足够证据识别明确完成事项或代码变更。")
	}
	daily.NextSteps = uniqueStrings(limitStrings(daily.NextSteps, 8))
	return daily
}

func fallbackWeeklyReport(now time.Time, dailyReports []string) *report.WeeklyReportJSON {
	week := report.WeekLabel(now)
	summary := "本周日报数量不足或 SupervisorAgent 未返回有效 JSON，无法形成完整跨日趋势。"
	if len(dailyReports) > 1 {
		summary = fmt.Sprintf("本周有 %d 份日报可参考，但模型输出不可用；以下为保守周报。", len(dailyReports))
	}
	return &report.WeeklyReportJSON{
		Week:    week,
		Summary: summary,
		Completed: []string{
			fmt.Sprintf("已收集 %d 份本周日报作为周报输入。", len(dailyReports)),
		},
		ProjectProgress: []report.ProjectProgress{
			{Project: "workspace", Status: "at_risk", Details: []string{"周报生成降级，跨日趋势需要更多结构化日报和 memory 支撑。"}},
		},
		RiskTrends: []report.RiskTrend{
			{Description: "周报信息不足，无法可靠判断持续风险。", DaysSeen: len(dailyReports), Severity: "stable", Days: []string{week}},
		},
		QualityTrend: "信息不足，无法可靠判断质量趋势。",
		NextWeekPlan: []string{
			"继续生成结构化日报，积累跨日趋势输入。",
			"检查 Agent trace，确认 WorkspaceAgent 工具调查是否稳定执行。",
		},
	}
}

func mergeObservations(a, b []ToolObservation) []ToolObservation {
	out := append([]ToolObservation(nil), a...)
	for _, obs := range b {
		if strings.TrimSpace(obs.Tool) == "" {
			continue
		}
		out = append(out, obs)
	}
	if len(out) > 20 {
		return out[:20]
	}
	return out
}

func compactIDs(id string) []string {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return []string{id}
}

func appendLimit(items []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || len(items) >= limit {
		return items
	}
	return append(items, value)
}

func appendChangeLimit(items []WorkspaceChange, value WorkspaceChange, limit int) []WorkspaceChange {
	if strings.TrimSpace(value.Description) == "" || len(items) >= limit {
		return items
	}
	return append(items, value)
}

func moduleFromPath(path string) string {
	path = filepath.ToSlash(path)
	if path == "" || path == "." {
		return "workspace"
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return "root"
	}
	return parts[0]
}

func limitStrings(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
