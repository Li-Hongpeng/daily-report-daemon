package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/llm"
)

// DeveloperMarkdown renders a DailyReportJSON into a developer-oriented Markdown report.
func DeveloperMarkdown(report *llm.DailyReportJSON, evidenceIndex map[string]string) string {
	var b strings.Builder

	date := report.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	b.WriteString(fmt.Sprintf("# 日报 — %s\n\n", date))

	// Summary
	b.WriteString("## 今日概览\n\n")
	if len(report.Summary) > 0 {
		for _, s := range report.Summary {
			b.WriteString(fmt.Sprintf("- %s\n", s))
		}
	} else {
		b.WriteString("*今日无明显代码活动。*\n")
	}
	b.WriteString("\n")

	// Completed
	b.WriteString("## 完成事项\n\n")
	if len(report.Completed) > 0 {
		for _, item := range report.Completed {
			marker := ""
			if item.Inferred {
				marker = " ⚠推断"
			}
			b.WriteString(fmt.Sprintf("- %s%s\n", item.Description, marker))
			writeEvidenceRefs(&b, item.EvidenceIDs, evidenceIndex)
		}
	} else {
		b.WriteString("*无已完成的明确事项。*\n")
	}
	b.WriteString("\n")

	// Changes
	b.WriteString("## 关键代码变更\n\n")
	if len(report.Changes) > 0 {
		for _, c := range report.Changes {
			marker := ""
			if c.Inferred {
				marker = " ⚠推断"
			}
			module := ""
			if c.Module != "" {
				module = fmt.Sprintf(" [%s]", c.Module)
			}
			b.WriteString(fmt.Sprintf("- **%s**%s: %s%s\n", c.File, module, c.Description, marker))
			writeEvidenceRefs(&b, c.EvidenceIDs, evidenceIndex)
		}
	} else {
		b.WriteString("*今日无关键代码变更。*\n")
	}
	b.WriteString("\n")

	// Risks
	b.WriteString("## 风险与待确认\n\n")
	if len(report.Risks) > 0 {
		for _, r := range report.Risks {
			severity := fmt.Sprintf("[%s]", strings.ToUpper(r.Severity))
			marker := ""
			if r.Inferred {
				marker = " ⚠推断"
			}
			b.WriteString(fmt.Sprintf("- %s %s%s\n", severity, r.Description, marker))
			writeEvidenceRefs(&b, r.EvidenceIDs, evidenceIndex)
		}
	} else {
		b.WriteString("*未发现明显风险。*\n")
	}
	b.WriteString("\n")

	// Blockers
	b.WriteString("## 可能卡点\n\n")
	if len(report.Blockers) > 0 {
		for _, bl := range report.Blockers {
			marker := ""
			if bl.Inferred {
				marker = " ⚠推断"
			}
			b.WriteString(fmt.Sprintf("- %s%s\n", bl.Description, marker))
			writeEvidenceRefs(&b, bl.EvidenceIDs, evidenceIndex)
		}
	} else {
		b.WriteString("*未发现明显卡点。*\n")
	}
	b.WriteString("\n")

	// Next steps
	b.WriteString("## 明日建议\n\n")
	if len(report.NextSteps) > 0 {
		for _, ns := range report.NextSteps {
			b.WriteString(fmt.Sprintf("- %s\n", ns))
		}
	} else {
		b.WriteString("*无具体建议。*\n")
	}
	b.WriteString("\n")

	// Evidence index
	if len(evidenceIndex) > 0 {
		b.WriteString("## 证据索引\n\n")
		for id, summary := range evidenceIndex {
			b.WriteString(fmt.Sprintf("- `%s`: %s\n", id, summary))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n*本报告由 daily-report-daemon 自动生成。*\n")

	return b.String()
}

func writeEvidenceRefs(b *strings.Builder, ids []string, index map[string]string) {
	if len(ids) == 0 {
		return
	}
	b.WriteString("  - 证据: ")
	for i, id := range ids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("`%s`", id))
	}
	b.WriteString("\n")
}

// DegradedMarkdown generates a minimal report when the model output is missing.
func DegradedMarkdown(date string, note string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# 日报 — %s\n\n", date))
	b.WriteString("## 今日概览\n\n")
	b.WriteString(fmt.Sprintf("*%s*\n\n", note))
	b.WriteString("---\n*本报告由 daily-report-daemon 自动生成（降级模式）。*\n")
	return b.String()
}

// SaveReport writes a rendered report to the output directory.
func SaveReport(content string, outputDir, date string) (string, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}
	filename := filepath.Join(outputDir, fmt.Sprintf("%s.md", date))
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return filename, nil
}

// BuildEvidenceIndex creates a lookup map from evidence item summaries (for report indexing).
func BuildEvidenceIndex(items interface{}) map[string]string {
	// In Phase 0, we use a simplified approach: the LLM response carries evidence IDs,
	// and the rendered report references them. The index in the report is built from IDs
	// the LLM returns. For now, return an empty map — the LLM will populate evidence_ids.
	return make(map[string]string)
}
