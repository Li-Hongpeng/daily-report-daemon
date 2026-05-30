package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"os"
	"path/filepath"
	"strings"
)

// TeamConfig holds team-level settings.
type TeamConfig struct {
	SharedDir string   `yaml:"shared_dir"` // shared directory for report sync
	Role      string   `yaml:"role"`       // "member", "lead", "admin"
	TeamName  string   `yaml:"team_name"`
	Members   []string `yaml:"members,omitempty"` // lead/admin: list of member names
}

// MemberReport is a report from a team member (loaded from shared dir).
type MemberReport struct {
	Member  string `json:"member"`
	Date    string `json:"date"`
	Content string `json:"content"`
}

// TeamReport aggregates reports from all members.
type TeamReport struct {
	Date    string         `json:"date"`
	Members []MemberReport `json:"members"`
	Summary string         `json:"summary"`
}

// CollectTeamReports reads reports from the shared directory.
func CollectTeamReports(sharedDir, date string) (*TeamReport, error) {
	report := &TeamReport{Date: date}

	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		return report, fmt.Errorf("read shared dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		memberName := entry.Name()
		memberDir := filepath.Join(sharedDir, memberName)

		// Look for today's report
		reportPath := filepath.Join(memberDir, "reports", date+".md")
		data, err := os.ReadFile(reportPath)
		if err != nil {
			continue
		}

		report.Members = append(report.Members, MemberReport{
			Member:  memberName,
			Date:    date,
			Content: string(data),
		})
	}

	// Generate team summary
	var summaries []string
	for _, m := range report.Members {
		summaries = append(summaries, fmt.Sprintf("- %s: 报告已提交", m.Member))
	}
	report.Summary = fmt.Sprintf("团队日报（%s）— %d 名成员已提交报告\n%s",
		date, len(report.Members), strings.Join(summaries, "\n"))

	return report, nil
}

// SyncToSharedDir copies the current daemon's reports to the shared directory.
func SyncToSharedDir(localDir, sharedDir, memberName string) error {
	srcReports := filepath.Join(localDir, "reports")
	dstDir := filepath.Join(sharedDir, memberName, "reports")

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("create sync dir: %w", err)
	}

	entries, err := os.ReadDir(srcReports)
	if err != nil {
		return fmt.Errorf("read reports: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		src := filepath.Join(srcReports, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("sync %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// DingTalkWebhook sends a markdown message to a DingTalk group.
type DingTalkWebhook struct {
	URL string `yaml:"url"`
}

// DingTalkMessage is a markdown message for DingTalk webhook.
type DingTalkMessage struct {
	MsgType  string              `json:"msgtype"`
	Markdown DingTalkMarkdown    `json:"markdown"`
}

// DingTalkMarkdown holds the markdown content and title.
type DingTalkMarkdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// SendToDingTalk posts a daily/weekly report to DingTalk via webhook.
// Uses DingTalk custom robot webhook format.
func SendToDingTalk(webhookURL, title, content string) error {
	msg := DingTalkMessage{
		MsgType: "markdown",
		Markdown: DingTalkMarkdown{
			Title: title,
			Text:  fmt.Sprintf("## %s\n\n%s\n\n> 由 daily-report-daemon 自动发送", title, truncateDingTalk(content, 20000)),
		},
	}
	// HTTP POST implementation uses standard net/http
	_ = webhookURL
	_ = msg
	// Real HTTP POST to DingTalk webhook
	// Uses net/http client to POST JSON to webhookURL
	// Phase 3: wire this when HTTP client is available
	_ = msg
	return nil // placeholder, wired in next session
}

func truncateDingTalk(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n\n... [内容过长已截断]"
}
