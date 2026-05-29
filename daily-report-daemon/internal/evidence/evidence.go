package evidence

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Type categorizes an evidence item.
type Type string

const (
	TypeCommit      Type = "commit"
	TypeDiff        Type = "diff"
	TypeFileChange  Type = "file_change"
	TypeFileMetadata Type = "file_metadata"
	TypeDocSnippet  Type = "doc_snippet"
	TypeTodo        Type = "todo"
	TypeCommand     Type = "command_result"
)

// Sensitivity level for an evidence item.
type Sensitivity string

const (
	SensLow    Sensitivity = "low"
	SensMedium Sensitivity = "medium"
	SensHigh   Sensitivity = "high"
)

// Item is a single piece of evidence.
type Item struct {
	ID          string      `json:"id"`
	Type        Type        `json:"type"`
	Workspace   string      `json:"workspace"`
	Path        string      `json:"path"`
	Summary     string      `json:"summary"`
	RawRef      string      `json:"raw_ref"`
	Sensitivity Sensitivity `json:"sensitivity"`
	Source      string      `json:"source"` // "git", "scanner"
}

// GenerateID creates a stable evidence ID from type + path + discriminator.
func GenerateID(typ Type, path string, discriminator string) string {
	input := string(typ) + ":" + path
	if discriminator != "" {
		input += ":" + discriminator
	}
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%s:%s:%x", typ, sanitizePath(path), hash[:4])
}

func sanitizePath(p string) string {
	return strings.ReplaceAll(p, "/", ":")
}

// SaveJSONL writes evidence items as JSONL to the given path.
func SaveJSONL(items []Item, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return fmt.Errorf("encode item: %w", err)
		}
	}
	return nil
}
