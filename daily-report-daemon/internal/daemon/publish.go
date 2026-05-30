package daemon

import (
	"fmt"
	"net/smtp"
	"strings"
)

// LeadReport renders a lead/manager-oriented version of the daily report.
// Strips technical details, emphasizes progress, risks, and help needed.
func LeadReport(developerReport string) string {
	var b strings.Builder

	// Extract sections minimally — in Phase 2, this is driven by agent prompt.
	// For now, provide a simplified transformation.
	b.WriteString("# 团队日报（组长版）\n\n")
	b.WriteString("## 今日进展\n\n")
	b.WriteString(extractSection(developerReport, "今日概览"))
	b.WriteString("\n")

	b.WriteString("## 需要关注的风险\n\n")
	b.WriteString(extractSection(developerReport, "风险与待确认"))
	b.WriteString("\n")

	b.WriteString("## 可能需要协助\n\n")
	b.WriteString(extractSection(developerReport, "可能卡点"))
	b.WriteString("\n")

	b.WriteString("## 明日计划\n\n")
	b.WriteString(extractSection(developerReport, "明日建议"))
	b.WriteString("\n")

	b.WriteString("---\n*由 daily-report-daemon 自动生成。技术细节已精简。*\n")

	return b.String()
}

// LeadReportSystemPrompt returns the system prompt for lead-oriented report generation.
func LeadReportSystemPrompt(language string) string {
	return fmt.Sprintf(`你是一个团队报告生成器。将开发者日报转换为组长版本。

规则：
1. 保留进展和风险，精简技术细节（不展示文件路径、commit hash、代码片段）
2. 突出需要组长关注或协助的问题
3. 使用非技术语言，让非技术管理者也能理解
4. 保留证据索引（组长可以深挖）
5. 输出语言：%s
6. 输出 Markdown`, language)
}

func extractSection(md, heading string) string {
	lines := strings.Split(md, "\n")
	inSection := false
	var result []string

	for _, line := range lines {
		if strings.HasPrefix(line, "## "+heading) {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			break
		}
		if inSection && strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}

	if len(result) == 0 {
		return "*暂无信息。*\n"
	}
	return strings.Join(result, "\n") + "\n"
}

// EmailConfig holds SMTP settings.
type EmailConfig struct {
	SMTPServer string `yaml:"smtp_server"`
	SMTPPort   string `yaml:"smtp_port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	From       string `yaml:"from"`
	To         []string `yaml:"to"`
}

// SendEmail sends a report via SMTP.
// Phase 2: wire this to Phase 0 Publisher config. Note: uses plain SMTP (port 25/587 without TLS). Phase 3 should upgrade to STARTTLS via crypto/tls + smtp.SendMail with TLS dial.
func SendEmail(cfg EmailConfig, subject, body string) error {
	if cfg.SMTPServer == "" {
		return fmt.Errorf("SMTP server not configured")
	}

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPServer)

	msg := fmt.Sprintf("From: %s\r\n", cfg.From)
	msg += fmt.Sprintf("To: %s\r\n", strings.Join(cfg.To, ", "))
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "Content-Type: text/plain; charset=UTF-8\r\n"
	msg += "\r\n"
	msg += body

	addr := cfg.SMTPServer + ":" + cfg.SMTPPort
	return smtp.SendMail(addr, auth, cfg.From, cfg.To, []byte(msg))
}

// PublishStatus tracks publication state.
type PublishStatus struct {
	Sent      bool   `json:"sent"`
	Channel   string `json:"channel"`
	Timestamp string `json:"timestamp,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Publish sends the report through configured channels.
// Supports manual confirmation mode (default) and auto-send.
func Publish(cfg EmailConfig, report string, autoSend bool, dryRun bool) (*PublishStatus, error) {
	status := &PublishStatus{Channel: "email"}

	if dryRun {
		fmt.Println("[dry-run] Email would be sent:")
		fmt.Printf("  To: %s\n", strings.Join(cfg.To, ", "))
		fmt.Printf("  Subject: 日报 %s\n", "YYYY-MM-DD")
		fmt.Printf("  Body: %d bytes\n", len(report))
		return status, nil
	}

	if !autoSend {
		fmt.Println("[manual] Report ready for review. Use --auto-send to publish immediately.")
		fmt.Printf("Preview: %d bytes\n", len(report))
		return status, nil
	}

	if err := SendEmail(cfg, "日报", report); err != nil {
		status.Error = err.Error()
		return status, err
	}

	status.Sent = true
	return status, nil
}
