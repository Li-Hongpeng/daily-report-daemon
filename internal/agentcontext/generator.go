package agentcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/scanner"
)

const MaxLines = 200

// Generator creates AGENTS.generated.md from project metadata and evidence.
type Generator struct {
	Root           string
	OutputDir      string // .daily-report-daemon/context
	HasAgentMD     bool
	AgentMDPath    string // path to root AGENTS.md if exists
}

// NewGenerator creates a Generator for the given project root.
func NewGenerator(root string, outputDir string) *Generator {
	agentMDPath := filepath.Join(root, "AGENTS.md")
	hasAgentMD := false
	if _, err := os.Stat(agentMDPath); err == nil {
		hasAgentMD = true
	}
	return &Generator{
		Root:        root,
		OutputDir:   outputDir,
		HasAgentMD:  hasAgentMD,
		AgentMDPath: agentMDPath,
	}
}

// Generate creates the AGENTS.generated.md content from metadata and evidence.
func (g *Generator) Generate(meta *scanner.ProjectMetadata, evItems []evidence.Item) (string, error) {
	var b strings.Builder

	// Header
	b.WriteString("# AGENTS.generated.md\n\n")
	b.WriteString(fmt.Sprintf("> 自动生成于 %s。请勿手动编辑。\n", time.Now().Format("2006-01-02 15:04")))

	if g.HasAgentMD {
		b.WriteString("> ⚠ 项目根目录已存在 `AGENTS.md`（人工维护上下文），本文件为补充生成内容。\n")
	}
	b.WriteString("\n")

	// Project overview
	b.WriteString("## Project Overview\n\n")
	overview := extractOverview(meta)
	b.WriteString(overview)
	b.WriteString("\n\n")

	// Repository layout
	b.WriteString("## Repository Layout\n\n")
	if len(meta.Structure) > 0 {
		for _, entry := range meta.Structure {
			if entry.Type == "dir" {
				b.WriteString(fmt.Sprintf("- `%s/` — directory\n", entry.Name))
			}
		}
		// List notable files not in subdirs
		for _, entry := range meta.Structure {
			if entry.Type == "file" {
				b.WriteString(fmt.Sprintf("- `%s`\n", entry.Name))
			}
		}
	} else {
		b.WriteString("*未检测到项目结构。*\n")
	}
	b.WriteString("\n")

	// Language stats
	if len(meta.Languages) > 0 {
		b.WriteString("**主要语言：** ")
		for i, l := range meta.Languages {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("%s (%d files)", l.Language, l.Files))
		}
		b.WriteString("\n\n")
	}

	// Build, run, test commands
	b.WriteString("## Build, Run, Test Commands\n\n")
	if len(meta.BuildCommands) > 0 {
		b.WriteString("### Build / Run\n\n")
		for _, cmd := range meta.BuildCommands {
			marker := ""
			if strings.Contains(cmd.Command, "(needs confirmation)") {
				marker = " (needs confirmation)"
			}
			b.WriteString(fmt.Sprintf("- `%s`%s — from %s\n", cmd.Command, marker, cmd.Source))
		}
		b.WriteString("\n")
	}
	if len(meta.TestCommands) > 0 {
		b.WriteString("### Test / Lint\n\n")
		for _, cmd := range meta.TestCommands {
			b.WriteString(fmt.Sprintf("- `%s` — from %s\n", cmd.Command, cmd.Source))
		}
		b.WriteString("\n")
	}
	if len(meta.BuildCommands) == 0 && len(meta.TestCommands) == 0 {
		b.WriteString("*未检测到构建或测试命令。请手动确认。*\n\n")
	}

	// Coding conventions
	b.WriteString("## Coding Conventions\n\n")
	conventions := extractConventions(meta)
	if conventions != "" {
		b.WriteString(conventions)
	} else {
		b.WriteString("No clear conventions detected.\n")
	}
	b.WriteString("\n")

	// Important files
	b.WriteString("## Important Files\n\n")
	if len(meta.KeyFiles) > 0 {
		shown := 0
		for _, kf := range meta.KeyFiles {
			if shown >= 10 {
				break
			}
			b.WriteString(fmt.Sprintf("- `%s` — %s\n", kf.Path, kf.Name))
			shown++
		}
	} else {
		b.WriteString("*未检测到关键配置文件。*\n")
	}
	b.WriteString("\n")

	// Today's activity
	b.WriteString("## Today's Activity\n\n")
	activityItems := filterByDate(evItems, time.Now())
	if len(activityItems) > 0 {
		for _, item := range activityItems {
			b.WriteString(fmt.Sprintf("- %s (`%s`)\n", item.Summary, item.ID))
		}
	} else {
		b.WriteString("*今日无明显活动。*\n")
	}
	b.WriteString("\n")

	// Known risks and open questions
	b.WriteString("## Known Risks and Open Questions\n\n")
	risks := filterRisks(evItems)
	if len(risks) > 0 {
		for _, item := range risks {
			b.WriteString(fmt.Sprintf("- %s (`%s`)\n", item.Summary, item.ID))
		}
	} else {
		b.WriteString("*未检测到已知风险。*\n")
	}
	b.WriteString("\n")

	// Suggested prompts
	b.WriteString("## Suggested Prompts for Agents\n\n")
	b.WriteString(generateSuggestedPrompts(meta))
	b.WriteString("\n")

	// Line count check
	result := b.String()
	lines := strings.Count(result, "\n") + 1
	if lines > MaxLines {
		result += fmt.Sprintf("\n---\n⚠ 本文件已超过 %d 行（当前 %d 行）。建议拆分或精简。\n", MaxLines, lines)
	}

	return result, nil
}

// Save writes AGENTS.generated.md to the output directory.
func (g *Generator) Save(content string) (string, error) {
	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return "", fmt.Errorf("create context dir: %w", err)
	}
	path := filepath.Join(g.OutputDir, "AGENTS.generated.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return path, nil
}

func extractOverview(meta *scanner.ProjectMetadata) string {
	// Try to get first paragraph from README
	for _, kf := range meta.KeyFiles {
		if kf.Name == "README.md" || kf.Name == "README" {
			lines := strings.Split(kf.Content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				return line
			}
		}
	}
	// Fallback: project name + language stats
	name := filepath.Base(meta.Root)
	langs := []string{}
	for _, l := range meta.Languages {
		langs = append(langs, l.Language)
	}
	if len(langs) > 0 {
		return fmt.Sprintf("%s — 一个 %s 项目。", name, strings.Join(langs, "/"))
	}
	return fmt.Sprintf("%s 项目的自动生成上下文。", name)
}

func extractConventions(meta *scanner.ProjectMetadata) string {
	// Look for convention hints in key files
	var hints []string
	for _, kf := range meta.KeyFiles {
		switch kf.Name {
		case ".gitignore":
			if strings.Contains(kf.Content, "dist") || strings.Contains(kf.Content, "build") {
				hints = append(hints, "- 构建产物不入库（`.gitignore` 中排除了 build/dist 目录）")
			}
		case "Makefile", "makefile":
			if strings.Contains(kf.Content, "lint") {
				hints = append(hints, "- 使用 `make lint` 进行代码检查")
			}
		}
	}

	// Look at file extensions for style hints
	hasGo := false
	hasTS := false
	for _, l := range meta.Languages {
		if l.Language == "Go" {
			hasGo = true
		}
		if l.Language == "TypeScript" {
			hasTS = true
		}
	}
	if hasGo {
		hints = append(hints, "- 使用 `go fmt` 格式化代码")
	}
	if hasTS {
		hints = append(hints, "- 可能使用 ESLint/Prettier 进行代码风格检查")
	}

	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "\n")
}

func filterByDate(items []evidence.Item, date time.Time) []evidence.Item {
	dateStr := date.Format("2006-01-02")
	var result []evidence.Item
	for _, item := range items {
		// Only include items whose summary contains today's date
		if strings.Contains(item.Summary, dateStr) {
			result = append(result, item)
		}
	}
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

func filterRisks(items []evidence.Item) []evidence.Item {
	var result []evidence.Item
	for _, item := range items {
		if item.Sensitivity == evidence.SensMedium || item.Sensitivity == evidence.SensHigh {
			result = append(result, item)
		}
	}
	if len(result) > 5 {
		result = result[:5]
	}
	return result
}

func generateSuggestedPrompts(meta *scanner.ProjectMetadata) string {
	var prompts []string
	name := filepath.Base(meta.Root)

	prompts = append(prompts, fmt.Sprintf("- \"帮我理解 %s 项目的目录结构\"", name))

	// Find a test command
	for _, cmd := range meta.TestCommands {
		prompts = append(prompts, fmt.Sprintf("- \"运行测试：`%s`，修复所有失败的测试\"", cmd.Command))
		break
	}

	// Find a build command
	for _, cmd := range meta.BuildCommands {
		prompts = append(prompts, fmt.Sprintf("- \"构建项目：`%s`，修复编译错误\"", cmd.Command))
		break
	}

	prompts = append(prompts, fmt.Sprintf("- \"根据 AGENTS.generated.md 的代码规范，帮我实现一个新功能\""))

	return strings.Join(prompts, "\n")
}
