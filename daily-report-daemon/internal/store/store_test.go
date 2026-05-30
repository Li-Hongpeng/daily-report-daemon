package store

import (
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := t.TempDir() + "/test.db"
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenClose(t *testing.T) {
	s := newTestStore(t)
	if err := s.db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestEnsureWorkspace(t *testing.T) {
	s := newTestStore(t)
	id1, err := s.EnsureWorkspace("test-repo", "/tmp/test-repo")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("expected positive id, got %d", id1)
	}
	id2, err := s.EnsureWorkspace("test-repo-dup", "/tmp/test-repo")
	if err != nil {
		t.Fatalf("EnsureWorkspace (dup): %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id %d, got %d", id1, id2)
	}
}

func TestListWorkspaces(t *testing.T) {
	s := newTestStore(t)
	s.EnsureWorkspace("repo-a", "/tmp/a")
	s.EnsureWorkspace("repo-b", "/tmp/b")
	ws, err := s.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(ws) < 2 {
		t.Fatalf("expected >=2 workspaces, got %d", len(ws))
	}
}

func TestScanRunLifecycle(t *testing.T) {
	s := newTestStore(t)
	wsID, _ := s.EnsureWorkspace("test", "/tmp/test-scan")
	runID := "run-001"
	if err := s.CreateScanRun(runID, wsID); err != nil {
		t.Fatalf("CreateScanRun: %v", err)
	}
	if err := s.FinishScanRun(runID, 42, 5, 2, "completed"); err != nil {
		t.Fatalf("FinishScanRun: %v", err)
	}
	latest, err := s.LatestScanRun(wsID)
	if err != nil {
		t.Fatalf("LatestScanRun: %v", err)
	}
	if latest == nil {
		t.Fatal("expected scan run, got nil")
	}
	if latest.FilesScanned != 42 || latest.DiffFiles != 5 || latest.Redactions != 2 {
		t.Fatalf("unexpected values: %+v", latest)
	}
}

func TestFileSnapshot(t *testing.T) {
	s := newTestStore(t)
	wsID, _ := s.EnsureWorkspace("test", "/tmp/test-snap")
	if err := s.UpsertFileSnapshot(wsID, "main.go", 1024, "2026-01-01T00:00:00Z", "abc123"); err != nil {
		t.Fatalf("UpsertFileSnapshot: %v", err)
	}
	snap, err := s.GetFileSnapshot(wsID, "main.go")
	if err != nil {
		t.Fatalf("GetFileSnapshot: %v", err)
	}
	if snap == nil || snap.Size != 1024 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	nosnap, _ := s.GetFileSnapshot(wsID, "nonexistent.go")
	if nosnap != nil {
		t.Fatal("expected nil for nonexistent file")
	}
}

func TestChangedFiles(t *testing.T) {
	s := newTestStore(t)
	wsID, _ := s.EnsureWorkspace("test", "/tmp/test-changed")
	s.UpsertFileSnapshot(wsID, "a.go", 100, "t1", "h1")
	s.UpsertFileSnapshot(wsID, "b.go", 200, "t2", "h2")

	changed, err := s.ChangedFiles(wsID, []FileCandidate{{Path: "a.go", Mtime: "t1", Hash: "newhash"}})
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed, got %d", len(changed))
	}

	unchanged, _ := s.ChangedFiles(wsID, []FileCandidate{{Path: "b.go", Mtime: "t2", Hash: "h2"}})
	if len(unchanged) != 0 {
		t.Fatalf("expected 0 changed, got %d", len(unchanged))
	}

	newFile, _ := s.ChangedFiles(wsID, []FileCandidate{{Path: "c.go", Mtime: "t3", Hash: "h3"}})
	if len(newFile) != 1 {
		t.Fatalf("expected new file as changed, got %d", len(newFile))
	}
}

func TestEvidenceCRUD(t *testing.T) {
	s := newTestStore(t)
	wsID, _ := s.EnsureWorkspace("test", "/tmp/test-ev")
	s.CreateScanRun("run-ev", wsID)
	e := EvidenceRow{ID: "ev-001", ScanRunID: "run-ev", Type: "git_commit", Workspace: "test", Summary: "added feature X", Sensitivity: "low", Source: "git", CreatedAt: "2026-05-29T00:00:00Z"}
	if err := s.InsertEvidence(e); err != nil {
		t.Fatalf("InsertEvidence: %v", err)
	}
	items, err := s.EvidenceByScanRun("run-ev")
	if err != nil || len(items) != 1 {
		t.Fatalf("EvidenceByScanRun: %v, %d items", err, len(items))
	}
}

func TestReportCRUD(t *testing.T) {
	s := newTestStore(t)
	wsID, _ := s.EnsureWorkspace("test", "/tmp/test-rpt")
	s.CreateScanRun("run-rpt", wsID)
	id, err := s.InsertReport("run-rpt", "daily", "2026-05-29", "# Report", "markdown")
	if err != nil || id <= 0 {
		t.Fatalf("InsertReport: %v, id=%d", err, id)
	}
	latest, err := s.LatestReport("daily", "2026-05-29")
	if err != nil || latest == nil {
		t.Fatalf("LatestReport: %v", err)
	}
}

func TestAgentSession(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateAgentSession("sess-001", "run-001"); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}
	if err := s.FinishAgentSession("sess-001", 5, 3, 12000, "completed", false); err != nil {
		t.Fatalf("FinishAgentSession: %v", err)
	}
}

func TestAgentMemory(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetAgentMemory("mem-001", "last_date", "2026-05-29"); err != nil {
		t.Fatalf("SetAgentMemory: %v", err)
	}
	mem, err := s.GetAgentMemory("last_date")
	if err != nil || mem == nil || mem.Value != "2026-05-29" {
		t.Fatalf("GetAgentMemory: %v, %+v", err, mem)
	}
	s.SetAgentMemory("mem-002", "last_date", "2026-05-30")
	mem2, _ := s.GetAgentMemory("last_date")
	if mem2.Value != "2026-05-30" {
		t.Fatalf("expected updated value, got %s", mem2.Value)
	}
}
