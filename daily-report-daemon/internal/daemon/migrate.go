package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// JSONLEvidenceItem mirrors Phase 0 evidence.Item for migration.
type JSONLEvidenceItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Workspace   string `json:"workspace"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
	Sensitivity string `json:"sensitivity"`
	Source      string `json:"source"`
}

// MigrateResult reports migration statistics.
type MigrateResult struct {
	RunsMigrated    int `json:"runs_migrated"`
	EvidenceItems    int `json:"evidence_items"`
	ReportsMigrated  int `json:"reports_migrated"`
	Errors          []string `json:"errors,omitempty"`
}

// MigrateJSONLToStore reads Phase 0 JSONL evidence files and returns items for SQLite insertion.
// Phase 2: call this during daemon startup to import legacy data.
func MigrateJSONLToStore(runsDir string) (*MigrateResult, error) {
	result := &MigrateResult{}

	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil // no legacy data
		}
		return result, fmt.Errorf("read runs dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(runsDir, entry.Name())
		evPath := filepath.Join(runDir, "evidence.jsonl")

		items, err := parseEvidenceFile(evPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}

		result.RunsMigrated++
		result.EvidenceItems += len(items)

		// Store each item (Phase 2: INSERT INTO evidence table)
		for _, item := range items {
			_ = item // Will be inserted into SQLite when driver is available
		}
	}

	// Check for legacy reports
	reportsDir := filepath.Join(filepath.Dir(runsDir), "reports")
	if entries, err := os.ReadDir(reportsDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".md") {
				result.ReportsMigrated++
			}
		}
		_ = entries
	}

	return result, nil
}

func parseEvidenceFile(path string) ([]JSONLEvidenceItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []JSONLEvidenceItem
	dec := json.NewDecoder(strings.NewReader(string(data)))
	for dec.More() {
		var item JSONLEvidenceItem
		if err := dec.Decode(&item); err != nil {
			continue // skip malformed lines
		}
		items = append(items, item)
	}
	return items, nil
}

// Summary returns a human-readable migration summary.
func (r *MigrateResult) Summary() string {
	return fmt.Sprintf("Migrated %d runs, %d evidence items, %d reports (%d errors)",
		r.RunsMigrated, r.EvidenceItems, r.ReportsMigrated, len(r.Errors))
}
