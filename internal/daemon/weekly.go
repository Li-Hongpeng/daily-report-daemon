package daemon

import (
	"fmt"
	"strings"
	"time"
)

// WeeklyReportJSON is the structured weekly report.
type WeeklyReportJSON struct {
	Week         string              `json:"week"`
	Summary      string              `json:"summary"`
	Completed    []string            `json:"completed"`
	ProjectProgress []ProjectProgress  `json:"project_progress"`
	RiskTrends   []RiskTrend          `json:"risk_trends"`
	QualityTrend string              `json:"quality_trend"`
	NextWeekPlan []string            `json:"next_week_plan"`
	Decisions    []string            `json:"decisions_needed,omitempty"`
}

// ProjectProgress tracks a project's progress this week.
type ProjectProgress struct {
	Project string   `json:"project"`
	Status  string   `json:"status"` // on_track/at_risk/blocked
	Details []string `json:"details"`
}

// RiskTrend tracks a risk that appeared across multiple days.
type RiskTrend struct {
	Description string   `json:"description"`
	DaysSeen    int      `json:"days_seen"`
	Severity    string   `json:"severity"` // increasing/stable/decreasing
	Days        []string `json:"days"`
}

// WeeklyReportSystemPrompt returns the system prompt for weekly report generation.
func WeeklyReportSystemPrompt(language string) string {
	return fmt.Sprintf(`你是一个技术周报生成器。基于本周每日开发活动，生成跨日叙事周报。

规则：
1. 不是每日日报的拼凑——识别跨日模式、持续风险、累积趋势
2. 本周完成事项按重要性排序，不是时间线
3. 项目进展标注状态：on_track / at_risk / blocked
4. 风险趋势：识别本周反复出现或逐渐恶化的信号
5. 质量趋势：基于本周变更模式、测试覆盖变化
6. 下周计划：基于未完成事项和风险的具体建议
7. 输出语言：%s
8. 输出结构化 JSON`, language)
}

// WeeklyReportUserPrompt builds the user prompt from daily evidence.
func WeeklyReportUserPrompt(dailyReports []string) string {
	weekStart, weekEnd := currentWeekRange()
	return fmt.Sprintf(`本周：%s 至 %s

本周各日报告：
%s

生成周报 JSON，结构：
{
  "week": "YYYY-WXX",
  "summary": "一句话概述本周",
  "completed": ["完成事项1", "完成事项2"],
  "project_progress": [{"project": "...", "status": "on_track|at_risk|blocked", "details": ["..."]}],
  "risk_trends": [{"description": "...", "days_seen": N, "severity": "increasing|stable|decreasing", "days": ["..."]}],
  "quality_trend": "质量趋势描述",
  "next_week_plan": ["建议1", "建议2"],
  "decisions_needed": ["需要组长决策的问题（可选）"]
}
`, weekStart, weekEnd, strings.Join(dailyReports, "\n\n---\n\n"))
}

// WeeklyMarkdown renders a WeeklyReportJSON to Markdown.
func WeeklyMarkdown(report *WeeklyReportJSON) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# 周报 — %s\n\n", report.Week))
	b.WriteString(fmt.Sprintf("## 本周概览\n\n%s\n\n", report.Summary))

	b.WriteString("## 本周完成\n\n")
	for _, item := range report.Completed {
		b.WriteString(fmt.Sprintf("- %s\n", item))
	}
	b.WriteString("\n")

	b.WriteString("## 重点项目进展\n\n")
	for _, pp := range report.ProjectProgress {
		statusIcon := "✅"
		if pp.Status == "at_risk" {
			statusIcon = "⚠️"
		} else if pp.Status == "blocked" {
			statusIcon = "🔴"
		}
		b.WriteString(fmt.Sprintf("### %s %s %s\n\n", statusIcon, pp.Project, pp.Status))
		for _, d := range pp.Details {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	b.WriteString("## 风险趋势\n\n")
	if len(report.RiskTrends) > 0 {
		for _, rt := range report.RiskTrends {
			icon := "→"
			if rt.Severity == "increasing" {
				icon = "📈"
			} else if rt.Severity == "decreasing" {
				icon = "📉"
			}
			b.WriteString(fmt.Sprintf("- %s %s（出现 %d 天：%s）\n", icon, rt.Description, rt.DaysSeen, strings.Join(rt.Days, ", ")))
		}
	} else {
		b.WriteString("*本周未发现明显的跨日风险模式。*\n")
	}
	b.WriteString("\n")

	b.WriteString("## 质量趋势\n\n")
	b.WriteString(report.QualityTrend)
	b.WriteString("\n\n")

	b.WriteString("## 下周计划\n\n")
	for _, plan := range report.NextWeekPlan {
		b.WriteString(fmt.Sprintf("- %s\n", plan))
	}
	b.WriteString("\n")

	if len(report.Decisions) > 0 {
		b.WriteString("## 需要决策\n\n")
		for _, d := range report.Decisions {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n*本报告由 daily-report-daemon Agent 引擎自动生成。*\n")

	return b.String()
}

func currentWeekRange() (string, string) {
	now := time.Now()
	weekday := now.Weekday()
	// Monday = start of week
	daysToMonday := int(weekday) - 1
	if daysToMonday < 0 {
		daysToMonday = 6 // Sunday
	}
	monday := now.AddDate(0, 0, -daysToMonday)
	sunday := monday.AddDate(0, 0, 6)
	return monday.Format("2006-01-02"), sunday.Format("2006-01-02")
}

// WeekLabel returns the ISO week label for the current week.
func WeekLabel() string {
	now := time.Now()
	year, week := now.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}
