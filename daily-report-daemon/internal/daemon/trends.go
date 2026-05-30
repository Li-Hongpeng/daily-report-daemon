package daemon

import (
	"fmt"
	"strings"
	"time"
)

// TrendPoint captures a metric at a point in time.
type TrendPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// CodeQualityTrend tracks code quality metrics over time.
type CodeQualityTrend struct {
	CommitsPerDay     []TrendPoint `json:"commits_per_day"`
	LinesChangedPerDay []TrendPoint `json:"lines_changed_per_day"`
	TODOCount          []TrendPoint `json:"todo_count"`
	FixmeCount         []TrendPoint `json:"fixme_count"`
	BugfixRatio        float64      `json:"bugfix_ratio"`
}

// TrendSignal tracks risk signals across days.
type TrendSignal struct {
	Pattern     string   `json:"pattern"`      // "increasing", "decreasing", "stable", "cyclic"
	Description string   `json:"description"`
	DaysSeen    int      `json:"days_seen"`
	FirstSeen   string   `json:"first_seen"`
	LastSeen    string   `json:"last_seen"`
	Evidence    []string `json:"evidence"`
}

// AnalyzeTrends computes trends from historical report data.
func AnalyzeTrends(reports []string) (*CodeQualityTrend, []TrendSignal, error) {
	qt := &CodeQualityTrend{}
	var risks []TrendSignal

	for _, report := range reports {
		// Count commits from report
		commits := countPattern(report, "commit:")
		qt.CommitsPerDay = append(qt.CommitsPerDay, TrendPoint{
			Date:  extractDate(report),
			Value: float64(commits),
		})

		// Count line changes
		changes := countPattern(report, "+") + countPattern(report, "-")
		qt.LinesChangedPerDay = append(qt.LinesChangedPerDay, TrendPoint{
			Date:  extractDate(report),
			Value: float64(changes),
		})

		// Count TODOs
		todos := countPattern(report, "TODO")
		qt.TODOCount = append(qt.TODOCount, TrendPoint{
			Date:  extractDate(report),
			Value: float64(todos),
		})
	}

	// Detect risk patterns
	if len(qt.CommitsPerDay) >= 3 {
		pattern := detectTrend(qt.CommitsPerDay)
		if pattern == "increasing" || pattern == "decreasing" {
			risks = append(risks, TrendSignal{
				Pattern:     pattern,
				Description: fmt.Sprintf("Commit 频率呈%s趋势", patternCN(pattern)),
				DaysSeen:    len(qt.CommitsPerDay),
				FirstSeen:   qt.CommitsPerDay[0].Date,
				LastSeen:    qt.CommitsPerDay[len(qt.CommitsPerDay)-1].Date,
			})
		}
	}

	if len(qt.TODOCount) >= 3 {
		pattern := detectTrend(qt.TODOCount)
		if pattern == "increasing" {
			risks = append(risks, TrendSignal{
				Pattern:     pattern,
				Description: "TODO 数量持续增长，技术债务可能在积累",
				DaysSeen:    len(qt.TODOCount),
				FirstSeen:   qt.TODOCount[0].Date,
				LastSeen:    qt.TODOCount[len(qt.TODOCount)-1].Date,
			})
		}
	}

	return qt, risks, nil
}

func countPattern(text, pattern string) int {
	return strings.Count(strings.ToLower(text), strings.ToLower(pattern))
}

func extractDate(report string) string {
	// Extract YYYY-MM-DD from report header
	idx := strings.Index(report, "# 日报 — ")
	if idx < 0 {
		return time.Now().Format("2006-01-02")
	}
	start := idx + len("# 日报 — ")
	end := strings.Index(report[start:], "\n")
	if end < 0 {
		end = 10
	}
	return strings.TrimSpace(report[start : start+end])
}

func detectTrend(points []TrendPoint) string {
	if len(points) < 2 {
		return "stable"
	}
	up := 0
	down := 0
	for i := 1; i < len(points); i++ {
		if points[i].Value > points[i-1].Value {
			up++
		} else if points[i].Value < points[i-1].Value {
			down++
		}
	}
	total := up + down
	if total == 0 {
		return "stable"
	}
	upRatio := float64(up) / float64(total)
	if upRatio > 0.7 {
		return "increasing"
	}
	if upRatio < 0.3 {
		return "decreasing"
	}
	return "stable"
}

func patternCN(p string) string {
	switch p {
	case "increasing":
		return "上升"
	case "decreasing":
		return "下降"
	case "cyclic":
		return "周期性"
	default:
		return "稳定"
	}
}

// TrendMarkdown renders trends as a markdown section.
func TrendMarkdown(qt *CodeQualityTrend, risks []TrendSignal) string {
	var b strings.Builder
	b.WriteString("## 质量趋势\n\n")

	if len(qt.CommitsPerDay) > 0 {
		b.WriteString("### 提交频率\n\n")
		for _, p := range qt.CommitsPerDay {
			b.WriteString(fmt.Sprintf("- %s: %.0f commits\n", p.Date, p.Value))
		}
		b.WriteString("\n")
	}

	if len(risks) > 0 {
		b.WriteString("### 风险趋势\n\n")
		for _, r := range risks {
			icon := "📈"
			if r.Pattern == "decreasing" {
				icon = "📉"
			} else if r.Pattern == "stable" {
				icon = "→"
			}
			b.WriteString(fmt.Sprintf("- %s %s（%d 天，%s ~ %s）\n",
				icon, r.Description, r.DaysSeen, r.FirstSeen, r.LastSeen))
		}
		b.WriteString("\n")
	}

	return b.String()
}
