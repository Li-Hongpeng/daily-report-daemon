package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileSnapshot tracks a file's state for incremental scanning.
type FileSnapshot struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
	Hash    string    `json:"hash,omitempty"`
}

// ScanState holds the last known state of scanned files.
type ScanState struct {
	Files    map[string]FileSnapshot `json:"files"`
	LastScan time.Time               `json:"last_scan"`
}

// NewScanState creates an empty scan state.
func NewScanState() *ScanState {
	return &ScanState{
		Files: make(map[string]FileSnapshot),
	}
}

// HasChanged checks if a file has changed since the last scan.
func (ss *ScanState) HasChanged(path string, info os.FileInfo) bool {
	prev, exists := ss.Files[path]
	if !exists {
		return true // new file
	}
	return prev.Size != info.Size() || !prev.ModTime.Equal(info.ModTime())
}

// Update records a file's current state.
func (ss *ScanState) Update(path string, info os.FileInfo) {
	ss.Files[path] = FileSnapshot{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
}

// Prune removes files no longer on disk.
func (ss *ScanState) Prune(root string) {
	for path := range ss.Files {
		fullPath := filepath.Join(root, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			delete(ss.Files, path)
		}
	}
}

// ChangedFiles returns paths of files that changed since last scan.
func (ss *ScanState) ChangedFiles(root string) (added, modified, deleted []string) {
	// Check existing files
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		info, _ := d.Info()
		if info == nil {
			return nil
		}

		prev, exists := ss.Files[rel]
		if !exists {
			added = append(added, rel)
		} else if prev.Size != info.Size() || !prev.ModTime.Equal(info.ModTime()) {
			modified = append(modified, rel)
		}
		return nil
	})

	// Find deleted files
	for path := range ss.Files {
		fullPath := filepath.Join(root, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			deleted = append(deleted, path)
		}
	}

	return added, modified, deleted
}

// Summary returns a human-readable summary of changes.
func (ss *ScanState) Summary() string {
	added, modified, deleted := ss.ChangedFiles(".")
	return fmt.Sprintf("%d added, %d modified, %d deleted (%d total tracked)",
		len(added), len(modified), len(deleted), len(ss.Files))
}
