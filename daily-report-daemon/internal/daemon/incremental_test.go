package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanStateNewFile(t *testing.T) {
	ss := NewScanState()
	dir := t.TempDir()

	// Create a file
	path := filepath.Join(dir, "new.go")
	os.WriteFile(path, []byte("package main"), 0644)
	info, _ := os.Stat(path)

	if !ss.HasChanged("new.go", info) {
		t.Error("new file should be detected as changed")
	}

	ss.Update("new.go", info)
	if ss.HasChanged("new.go", info) {
		t.Error("unchanged file should not be detected as changed")
	}
}

func TestScanStateModifiedFile(t *testing.T) {
	ss := NewScanState()
	dir := t.TempDir()

	path := filepath.Join(dir, "mod.go")
	os.WriteFile(path, []byte("v1"), 0644)
	info1, _ := os.Stat(path)
	ss.Update("mod.go", info1)

	// Modify
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("v2"), 0644)
	info2, _ := os.Stat(path)

	if !ss.HasChanged("mod.go", info2) {
		t.Error("modified file should be detected as changed")
	}
}

func TestChangedFiles(t *testing.T) {
	ss := NewScanState()
	dir := t.TempDir()

	// Initial state: one existing file
	path1 := filepath.Join(dir, "existing.go")
	os.WriteFile(path1, []byte("old"), 0644)
	info1, _ := os.Stat(path1)
	ss.Update("existing.go", info1)

	// Add new file, modify existing, don't delete (deletion check needs fs changes)
	path2 := filepath.Join(dir, "new.go")
	os.WriteFile(path2, []byte("new"), 0644)

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path1, []byte("modified"), 0644)

	added, modified, _ := ss.ChangedFiles(dir)

	if len(added) != 1 || added[0] != "new.go" {
		t.Errorf("expected 1 added file (new.go), got: %v", added)
	}
	if len(modified) != 1 || modified[0] != "existing.go" {
		t.Errorf("expected 1 modified file (existing.go), got: %v", modified)
	}
}

func TestScanStatePrune(t *testing.T) {
	ss := NewScanState()
	ss.Files["deleted.go"] = FileSnapshot{Path: "deleted.go", Size: 100}
	ss.Files["kept.go"] = FileSnapshot{Path: "kept.go", Size: 200}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "kept.go"), []byte("kept"), 0644)

	ss.Prune(dir)

	if _, ok := ss.Files["deleted.go"]; ok {
		t.Error("deleted file should be pruned")
	}
	if _, ok := ss.Files["kept.go"]; !ok {
		t.Error("kept file should remain")
	}
}

func TestScanStateSummary(t *testing.T) {
	ss := NewScanState()
	summary := ss.Summary()
	if len(summary) == 0 {
		t.Error("summary should not be empty")
	}
}
