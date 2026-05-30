package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WeeklyReportJSON is the structured weekly report output.
type WeeklyReportJSON struct {
	Week            string            `json:"week"`
	Summary         string            `json:"summary"`
	Completed       []string          `json:"completed"`
	ProjectProgress []ProjectProgress `json:"project_progress"`
	RiskTrends      []RiskTrend       `json:"risk_trends"`
	QualityTrend    string            `json:"quality_trend"`
	NextWeekPlan    []string          `json:"next_week_plan"`
	Decisions       []string          `json:"decisions_needed,omitempty"`
}

// ProjectProgress tracks a project's progress this week.
type ProjectProgress struct {
	Project string   `json:"project"`
	Status  string   `json:"status"`
	Details []string `json:"details"`
}

// RiskTrend tracks a risk that appeared across multiple days.
type RiskTrend struct {
	Description string   `json:"description"`
	DaysSeen    int      `json:"days_seen"`
	Severity    string   `json:"severity"`
	Days        []string `json:"days"`
}

// WeeklyReportSystemPrompt returns the system prompt for weekly report generation.
func WeeklyReportSystemPrompt(language string) string {
	return fmt.Sprintf(`你是一个技术周报生成器。基于本周每日开发活动，生成跨日叙事周报。

规则：
1. 不是每日日报的拼凑，要识别跨日模式、持续风险、累积趋势。
2. 本周完成事项按重要性排序，不按时间线罗列。
3. 项目进展标注状态：on_track / at_risk / blocked。
4. 风险趋势识别本周反复出现或逐渐恶化的信号。
5. 质量趋势基于本周变更模式、测试覆盖变化、风险收敛情况。
6. 下周计划要具体，可执行。
7. 输出语言：%s。
8. 输出必须是符合 WeeklyReport schema 的有效 JSON。`, language)
}

// WeeklyReportUserPrompt builds the user prompt from daily report markdown.
func WeeklyReportUserPrompt(dailyReports []string, now time.Time) string {
	weekStart, weekEnd := WeekRange(now)
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
  "decisions_needed": ["需要决策的问题（可选）"]
}

如果本周日报不足，也要诚实说明信息有限，不要编造。`, weekStart, weekEnd, strings.Join(dailyReports, "\n\n---\n\n"))
}

// ParseWeeklyReportJSON parses model output into WeeklyReportJSON.
func ParseWeeklyReportJSON(raw string) (*WeeklyReportJSON, error) {
	cleaned := extractJSONObject(raw)
	var report WeeklyReportJSON
	if err := json.Unmarshal([]byte(cleaned), &report); err != nil {
		return nil, fmt.Errorf("parse weekly report JSON: %w", err)
	}
	return &report, nil
}

// WeeklyMarkdown renders a weekly report to Markdown.
func WeeklyMarkdown(report *WeeklyReportJSON) string {
	var b strings.Builder
	if report.Week == "" {
		report.Week = WeekLabel(time.Now())
	}
	b.WriteString(fmt.Sprintf("# 周报 — %s\n\n", report.Week))
	b.WriteString(fmt.Sprintf("## 本周概览\n\n%s\n\n", fallbackText(report.Summary, "本周信息不足，无法形成完整概览。")))

	b.WriteString("## 本周完成\n\n")
	writeStringList(&b, report.Completed, "*未识别到明确完成事项。*")

	b.WriteString("## 重点项目进展\n\n")
	if len(report.ProjectProgress) == 0 {
		b.WriteString("*暂无项目进展信号。*\n\n")
	} else {
		for _, pp := range report.ProjectProgress {
			b.WriteString(fmt.Sprintf("### %s %s\n\n", pp.Project, pp.Status))
			writeStringList(&b, pp.Details, "*暂无细节。*")
		}
	}

	b.WriteString("## 风险趋势\n\n")
	if len(report.RiskTrends) == 0 {
		b.WriteString("*本周未发现明显跨日风险模式。*\n\n")
	} else {
		for _, rt := range report.RiskTrends {
			b.WriteString(fmt.Sprintf("- %s（出现 %d 天，趋势：%s，日期：%s）\n",
				rt.Description, rt.DaysSeen, rt.Severity, strings.Join(rt.Days, ", ")))
		}
		b.WriteString("\n")
	}

	b.WriteString("## 质量趋势\n\n")
	b.WriteString(fallbackText(report.QualityTrend, "暂无足够信息判断质量趋势。"))
	b.WriteString("\n\n")

	b.WriteString("## 下周计划\n\n")
	writeStringList(&b, report.NextWeekPlan, "*暂无明确下周计划。*")

	if len(report.Decisions) > 0 {
		b.WriteString("## 需要决策\n\n")
		writeStringList(&b, report.Decisions, "")
	}

	b.WriteString("---\n*本报告由 daily-report-daemon 自动生成。*\n")
	return b.String()
}

// SaveWeeklyReport writes weekly markdown into outputDir/YYYY-WXX.md.
func SaveWeeklyReport(content, outputDir, week string) (string, error) {
	if week == "" {
		week = WeekLabel(time.Now())
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create weekly report dir: %w", err)
	}
	path := filepath.Join(outputDir, week+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write weekly report: %w", err)
	}
	return path, nil
}

// WeekRange returns the Monday-Sunday range for t.
func WeekRange(t time.Time) (string, string) {
	weekday := t.Weekday()
	daysToMonday := int(weekday) - 1
	if daysToMonday < 0 {
		daysToMonday = 6
	}
	monday := t.AddDate(0, 0, -daysToMonday)
	sunday := monday.AddDate(0, 0, 6)
	return monday.Format("2006-01-02"), sunday.Format("2006-01-02")
}

// WeekLabel returns the ISO week label for t.
func WeekLabel(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

func extractJSONObject(raw string) string {
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

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writeStringList(b *strings.Builder, items []string, empty string) {
	if len(items) == 0 {
		if empty != "" {
			b.WriteString(empty)
			b.WriteString("\n\n")
		}
		return
	}
	for _, item := range items {
		b.WriteString(fmt.Sprintf("- %s\n", item))
	}
	b.WriteString("\n")
}
