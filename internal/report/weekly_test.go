package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseWeeklyReportJSONWithFencedOutput(t *testing.T) {
	raw := "```json\n{\"week\":\"2026-W22\",\"summary\":\"完成管线修复\",\"completed\":[\"日报修复\"],\"quality_trend\":\"测试增加\",\"next_week_plan\":[\"Windows 兼容\"]}\n```"
	report, err := ParseWeeklyReportJSON(raw)
	if err != nil {
		t.Fatalf("ParseWeeklyReportJSON failed: %v", err)
	}
	if report.Week != "2026-W22" {
		t.Fatalf("unexpected week: %s", report.Week)
	}
	if len(report.Completed) != 1 || report.Completed[0] != "日报修复" {
		t.Fatalf("unexpected completed items: %+v", report.Completed)
	}
}

func TestWeeklyMarkdownFallbacks(t *testing.T) {
	md := WeeklyMarkdown(&WeeklyReportJSON{Week: "2026-W22"})
	for _, want := range []string{
		"本周信息不足",
		"未识别到明确完成事项",
		"暂无项目进展信号",
		"暂无足够信息判断质量趋势",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("weekly markdown missing fallback %q:\n%s", want, md)
		}
	}
}

func TestSaveWeeklyReport(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveWeeklyReport("# Weekly", dir, "2026-W22")
	if err != nil {
		t.Fatalf("SaveWeeklyReport failed: %v", err)
	}
	if path != filepath.Join(dir, "2026-W22.md") {
		t.Fatalf("unexpected path: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read weekly report: %v", err)
	}
	if string(data) != "# Weekly" {
		t.Fatalf("unexpected content: %s", string(data))
	}
}

func TestWeekRangeStartsOnMonday(t *testing.T) {
	start, end := WeekRange(time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC))
	if start != "2026-05-25" || end != "2026-05-31" {
		t.Fatalf("unexpected week range: %s to %s", start, end)
	}
}
