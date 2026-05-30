package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonLifecycle(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, ".daily-report-daemon")
	os.MkdirAll(outputDir, 0755)

	d := New([]string{dir}, outputDir)

	// Start
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Check status
	status := d.Status()
	if status != "running" {
		t.Fatalf("expected running, got %s", status)
	}

	// PID file should exist
	if _, err := os.Stat(filepath.Join(outputDir, "daemon.pid")); err != nil {
		t.Fatalf("PID file: %v", err)
	}

	// Stop
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Should be stopped
	status = d.Status()
	if status == "running" {
		t.Fatal("expected stopped after Stop()")
	}
}

func TestDaemonRestart(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, ".daily-report-daemon")
	os.MkdirAll(outputDir, 0755)
	d := New([]string{dir}, outputDir)

	d.Start()
	time.Sleep(100 * time.Millisecond)

	if err := d.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}

	// After restart, should be running
	if d.Status() != "running" {
		t.Fatal("expected running after restart")
	}

	d.Stop()
}

func TestDaemonAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	d := New([]string{dir}, filepath.Join(dir, ".daily-report-daemon"))

	d.Start()
	defer d.Stop()

	// Second Start should fail
	if err := d.Start(); err == nil {
		t.Fatal("expected error on double start")
	}
}

func TestDaemonNotRunningStop(t *testing.T) {
	dir := t.TempDir()
	d := New([]string{dir}, filepath.Join(dir, ".daily-report-daemon"))

	if err := d.Stop(); err == nil {
		t.Fatal("expected error stopping non-running daemon")
	}
}

func TestReportTimeDetection(t *testing.T) {
	dir := t.TempDir()
	d := New([]string{dir}, filepath.Join(dir, ".daily-report-daemon"))

	// Set report time to current time
	now := time.Now()
	d.ReportTime = now.Format("15:04")

	if !d.isReportTime() {
		t.Fatal("expected isReportTime to be true")
	}

	// Set to 1 minute ago
	past := now.Add(-1 * time.Minute)
	d.ReportTime = past.Format("15:04")
	if d.isReportTime() {
		t.Fatal("expected isReportTime to be false for past time")
	}
}

func TestLastScan(t *testing.T) {
	dir := t.TempDir()
	d := New([]string{dir, "/tmp/other"}, filepath.Join(dir, ".daily-report-daemon"))

	// Before any scan, no last scan times
	_, ok := d.LastScan(dir)
	if ok {
		t.Fatal("expected no last scan before first scan")
	}

	// Simulate a scan
	d.mu.Lock()
	d.lastScan[dir] = time.Now()
	d.mu.Unlock()

	tm, ok := d.LastScan(dir)
	if !ok {
		t.Fatal("expected last scan time after scan")
	}
	if tm.IsZero() {
		t.Fatal("expected non-zero time")
	}
}
