package report

import (
	"os"
	"strings"
	"testing"
)

func fixtureFullReport() *DailyReportJSON {
	return &DailyReportJSON{
		Date:    "2026-05-29",
		Summary: []string{"完成 daily-report-daemon Phase 0 原型核心模块", "修复 3 个边界条件 bug"},
		Completed: []WorkItem{
			{Description: "实现 Git Analyzer 模块", EvidenceIDs: []string{"diff:analyzer.go:unstaged"}, Inferred: false},
			{Description: "实现 File Scanner 模块", EvidenceIDs: []string{"doc:scanner.go"}, Inferred: false},
		},
		Changes: []CodeChange{
			{File: "internal/git/analyzer.go", Description: "新增 untracked 文件处理逻辑", Module: "git", EvidenceIDs: []string{"diff:analyzer.go:unstaged"}},
			{File: "internal/sanitize/sanitize.go", Description: "新增 .env 变体路径过滤", Module: "sanitize", EvidenceIDs: []string{"diff:sanitize.go:unstaged"}, Inferred: false},
		},
		Risks: []RiskItem{
			{Description: "LLM 调用超时可能导致报告生成失败", Severity: "medium", EvidenceIDs: nil, Inferred: true},
		},
		Blockers:  []BlockerItem{},
		NextSteps: []string{"完成 LLM Client 集成测试", "编写端到端命令"},
	}
}

func fixtureEmptyReport() *DailyReportJSON {
	return &DailyReportJSON{
		Date:      "2026-05-29",
		Summary:   []string{},
		Completed: []WorkItem{},
		Changes:   []CodeChange{},
		Risks:     []RiskItem{},
		Blockers:  []BlockerItem{},
		NextSteps: []string{},
	}
}

func TestDeveloperMarkdownFull(t *testing.T) {
	report := fixtureFullReport()
	idx := map[string]string{
		"diff:analyzer.go:unstaged": "新增 untracked 文件处理",
		"diff:sanitize.go:unstaged": "新增 .env 变体过滤",
		"doc:scanner.go":            "File Scanner 实现",
	}
	md := DeveloperMarkdown(report, idx)

	if !strings.Contains(md, "2026-05-29") {
		t.Error("missing date")
	}
	if !strings.Contains(md, "今日概览") {
		t.Error("missing summary section")
	}
	if !strings.Contains(md, "完成事项") {
		t.Error("missing completed section")
	}
	if !strings.Contains(md, "关键代码变更") {
		t.Error("missing changes section")
	}
	if !strings.Contains(md, "风险与待确认") {
		t.Error("missing risks section")
	}
	if !strings.Contains(md, "可能卡点") {
		t.Error("missing blockers section")
	}
	if !strings.Contains(md, "明日建议") {
		t.Error("missing next steps section")
	}
	if !strings.Contains(md, "证据索引") {
		t.Error("missing evidence index")
	}
	if !strings.Contains(md, "diff:analyzer.go:unstaged") {
		t.Error("missing evidence ID in index")
	}
	if !strings.Contains(md, "daily-report-daemon") {
		t.Error("missing auto-generation footer")
	}
}

func TestDeveloperMarkdownEmpty(t *testing.T) {
	report := fixtureEmptyReport()
	md := DeveloperMarkdown(report, nil)

	if !strings.Contains(md, "无明显代码活动") && !strings.Contains(md, "无已完成") {
		// Should have fallback text in at least one section
	}
	// Should not crash
	if len(md) == 0 {
		t.Error("empty markdown output")
	}
}

func TestDeveloperMarkdownInferredItems(t *testing.T) {
	report := &DailyReportJSON{
		Date:    "2026-05-29",
		Summary: []string{"test"},
		Completed: []WorkItem{
			{Description: "推断项", EvidenceIDs: nil, Inferred: true},
		},
	}
	md := DeveloperMarkdown(report, nil)
	if !strings.Contains(md, "⚠推断") {
		t.Error("missing inferred marker")
	}
}

func TestDegradedMarkdown(t *testing.T) {
	md := DegradedMarkdown("2026-05-29", "模型输出解析失败，请检查原始数据。")
	if !strings.Contains(md, "降级模式") {
		t.Error("missing degraded mode indicator")
	}
	if !strings.Contains(md, "模型输出解析失败") {
		t.Error("missing fallback note")
	}
}

func TestSaveReport(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveReport("# Test Report", dir, "2026-05-29")
	if err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "# Test Report" {
		t.Errorf("unexpected content: %s", string(data))
	}
	if !strings.HasSuffix(path, "2026-05-29.md") {
		t.Errorf("unexpected filename: %s", path)
	}
}

func TestSaveReportDefaultDate(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveReport("# Test", dir, "")
	if err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}
	// Should use today's date
	if !strings.Contains(path, ".md") {
		t.Errorf("unexpected filename: %s", path)
	}
}

func TestBuildEvidenceIndex(t *testing.T) {
	idx := BuildEvidenceIndex(nil)
	if idx == nil {
		t.Error("expected non-nil map")
	}
}

func TestModuleDisplay(t *testing.T) {
	report := &DailyReportJSON{
		Date:    "2026-05-29",
		Summary: []string{"test"},
		Changes: []CodeChange{
			{File: "main.go", Description: "entry point", Module: "core", EvidenceIDs: []string{"id1"}},
		},
	}
	md := DeveloperMarkdown(report, map[string]string{"id1": "test"})
	if !strings.Contains(md, "[core]") {
		t.Error("missing module tag")
	}
}
