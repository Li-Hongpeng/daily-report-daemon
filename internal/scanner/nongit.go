package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DirectoryActivity captures changes in a non-Git directory.
type DirectoryActivity struct {
	Root        string         `json:"root"`
	FilesAdded    []FileChange `json:"files_added"`
	FilesModified []FileChange `json:"files_modified"`
	FilesDeleted  []FileChange `json:"files_deleted"`
	TotalFiles  int           `json:"total_files"`
	TotalTextFiles int        `json:"total_text_files"`
	TextSummaries  []string   `json:"text_summaries,omitempty"`
}

// FileChange records a file change event.
type FileChange struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mtime"`
	IsText  bool   `json:"is_text"`
	Summary string `json:"summary,omitempty"`
}

// ScanDirectory scans a non-Git directory for file activity.
func ScanDirectory(root string) (*DirectoryActivity, error) {
	act := &DirectoryActivity{Root: root}

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		info, _ := d.Info()
		if info == nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		ext := strings.ToLower(filepath.Ext(d.Name()))
		isText := !IsBinaryExt(ext) && info.Size() < MaxKeyFileBytes*2

		change := FileChange{
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			IsText:  isText,
		}

		act.TotalFiles++
		if isText {
			act.TotalTextFiles++
			data, err := os.ReadFile(path)
			if err == nil {
				content := string(data)
				if len(content) > 2048 {
					content = content[:2048] + "..."
				}
				content = redactPrivacy(content)
				change.Summary = content
				act.TextSummaries = append(act.TextSummaries, fmt.Sprintf("%s: %s", rel, truncateSummary(content, 100)))
			}
		}

		act.FilesAdded = append(act.FilesAdded, change)
		return nil
	})

	return act, nil
}

// Privacy patterns for enhanced filtering per lee's Phase 3 decision.
var (
	idCardPattern   = regexp.MustCompile(`\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)
	bankCardPattern = regexp.MustCompile(`\b[1-9]\d{15,18}\b`)
	phonePattern    = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
)

// redactPrivacy replaces detected PII with [REDACTED].
func redactPrivacy(content string) string {
	content = idCardPattern.ReplaceAllString(content, "[REDACTED:身份证号]")
	content = bankCardPattern.ReplaceAllString(content, "[REDACTED:银行卡号]")
	content = phonePattern.ReplaceAllString(content, "[REDACTED:手机号]")
	return content
}

func truncateSummary(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
