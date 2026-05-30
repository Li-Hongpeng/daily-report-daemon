package evidence

import (
	"fmt"
	"strings"

	"github.com/daily-report-daemon/internal/git"
	"github.com/daily-report-daemon/internal/sanitize"
	"github.com/daily-report-daemon/internal/scanner"
)

const MaxEvidenceContent = 12000

// Builder converts raw scan data into sanitized evidence.
type Builder struct {
	Workspace string
	Sanitizer *sanitize.Sanitizer
}

// NewBuilder creates an evidence Builder.
func NewBuilder(workspace string) *Builder {
	return &Builder{
		Workspace: workspace,
		Sanitizer: sanitize.New(),
	}
}

// BuildFromGit converts git.Activity into evidence items.
func (b *Builder) BuildFromGit(act *git.Activity) []Item {
	var items []Item

	// Commits
	for _, c := range act.Commits {
		subject := b.Sanitizer.Redact(c.Hash, c.Subject)
		items = append(items, Item{
			ID:          GenerateID(TypeCommit, c.Hash, ""),
			Type:        TypeCommit,
			Workspace:   b.Workspace,
			Path:        "",
			Summary:     fmt.Sprintf("[%s] %s: %s", c.Hash[:8], c.Author, subject),
			RawRef:      c.Hash,
			Sensitivity: SensLow,
			Source:      "git",
		})
	}

	// Diffs
	for _, d := range act.Diffs {
		if b.Sanitizer.CheckPath(d.File) {
			continue
		}

		patch := b.Sanitizer.Redact(d.File, d.Patch)
		patch, _ = b.Sanitizer.Truncate(d.File, patch, MaxEvidenceContent)

		summary := fmt.Sprintf("%s %s (%s): +%d -%d",
			d.Scope, d.File, d.ChangeType, d.Additions, d.Deletions)
		if d.Truncated {
			summary += " [truncated]"
		}

		items = append(items, Item{
			ID:          GenerateID(TypeDiff, d.File, d.Scope),
			Type:        TypeDiff,
			Workspace:   b.Workspace,
			Path:        d.File,
			Summary:     summary,
			Content:     patch,
			RawRef:      fmt.Sprintf("git diff (%s)", d.Scope),
			Sensitivity: sensForDiff(patch),
			Source:      "git",
		})
	}

	// File changes from status
	for _, se := range act.Status {
		if se.XY == "??" {
			continue // untracked files are already in diffs
		}
		if b.Sanitizer.CheckPath(se.Path) {
			continue
		}
		changeType := statusToChangeType(se.XY)
		items = append(items, Item{
			ID:          GenerateID(TypeFileChange, se.Path, se.XY),
			Type:        TypeFileChange,
			Workspace:   b.Workspace,
			Path:        se.Path,
			Summary:     fmt.Sprintf("%s: %s", changeType, se.Path),
			RawRef:      "git status --porcelain",
			Sensitivity: SensLow,
			Source:      "git",
		})
	}

	return items
}

// BuildFromScanner converts scanner.ProjectMetadata into evidence items.
func (b *Builder) BuildFromScanner(meta *scanner.ProjectMetadata) []Item {
	var items []Item

	// Key files as doc snippets
	for _, kf := range meta.KeyFiles {
		if b.Sanitizer.CheckPath(kf.Path) {
			continue
		}

		content := b.Sanitizer.Redact(kf.Path, kf.Content)
		content, truncated := b.Sanitizer.Truncate(kf.Path, content, MaxEvidenceContent)

		summary := fmt.Sprintf("Key file: %s (%d bytes)", kf.Name, len(kf.Content))
		if truncated || kf.Truncated {
			summary += " [truncated]"
		}

		items = append(items, Item{
			ID:          GenerateID(TypeDocSnippet, kf.Path, ""),
			Type:        TypeDocSnippet,
			Workspace:   b.Workspace,
			Path:        kf.Path,
			Summary:     summary,
			Content:     content,
			RawRef:      fmt.Sprintf("scanner:%s", kf.Path),
			Sensitivity: sensForContent(content),
			Source:      "scanner",
		})
	}

	// Build/test commands
	for _, cmd := range meta.BuildCommands {
		items = append(items, Item{
			ID:          GenerateID(TypeCommand, cmd.Command, "build"),
			Type:        TypeCommand,
			Workspace:   b.Workspace,
			Path:        cmd.Source,
			Summary:     fmt.Sprintf("Build command: %s (from %s)", cmd.Command, cmd.Source),
			RawRef:      cmd.Source,
			Sensitivity: SensLow,
			Source:      "scanner",
		})
	}
	for _, cmd := range meta.TestCommands {
		items = append(items, Item{
			ID:          GenerateID(TypeCommand, cmd.Command, "test"),
			Type:        TypeCommand,
			Workspace:   b.Workspace,
			Path:        cmd.Source,
			Summary:     fmt.Sprintf("Test command: %s (from %s)", cmd.Command, cmd.Source),
			RawRef:      cmd.Source,
			Sensitivity: SensLow,
			Source:      "scanner",
		})
	}

	return items
}

func statusToChangeType(xy string) string {
	switch {
	case strings.Contains(xy, "A"):
		return "added"
	case strings.Contains(xy, "D"):
		return "deleted"
	case strings.Contains(xy, "R"):
		return "renamed"
	case strings.Contains(xy, "M"):
		return "modified"
	default:
		return "changed"
	}
}

func sensForDiff(patch string) Sensitivity {
	lower := strings.ToLower(patch)
	if strings.Contains(lower, "secret") || strings.Contains(lower, "token") ||
		strings.Contains(lower, "password") || strings.Contains(lower, "key") {
		return SensMedium
	}
	return SensLow
}

func sensForContent(content string) Sensitivity {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "secret") || strings.Contains(lower, "token") ||
		strings.Contains(lower, "password") || strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "credential") {
		return SensMedium
	}
	return SensLow
}
