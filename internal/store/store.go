package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daily-report-daemon/internal/evidence"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

const Schema = `
CREATE TABLE IF NOT EXISTS workspaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL DEFAULT 'git_repo',
    enabled INTEGER DEFAULT 1,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS scan_runs (
    id TEXT PRIMARY KEY,
    workspace_id INTEGER NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    files_scanned INTEGER DEFAULT 0,
    diff_files INTEGER DEFAULT 0,
    redactions INTEGER DEFAULT 0,
    status TEXT DEFAULT 'running',
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
);
CREATE TABLE IF NOT EXISTS file_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    size INTEGER,
    mtime TEXT,
    hash TEXT,
    last_seen_at TEXT NOT NULL,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
);
CREATE TABLE IF NOT EXISTS git_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_run_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    file_path TEXT,
    details TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (scan_run_id) REFERENCES scan_runs(id)
);
CREATE TABLE IF NOT EXISTS evidence (
    id TEXT PRIMARY KEY,
    scan_run_id TEXT NOT NULL,
    type TEXT NOT NULL,
    workspace TEXT NOT NULL,
    path TEXT,
    summary TEXT,
    sensitivity TEXT DEFAULT 'low',
    source TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (scan_run_id) REFERENCES scan_runs(id)
);
CREATE TABLE IF NOT EXISTS reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_run_id TEXT,
    report_type TEXT NOT NULL,
    date TEXT NOT NULL,
    content TEXT NOT NULL,
    format TEXT DEFAULT 'markdown',
    created_at TEXT NOT NULL,
    FOREIGN KEY (scan_run_id) REFERENCES scan_runs(id)
);
CREATE TABLE IF NOT EXISTS publish_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id INTEGER NOT NULL,
    channel TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    sent_at TEXT,
    FOREIGN KEY (report_id) REFERENCES reports(id)
);
CREATE TABLE IF NOT EXISTS agent_sessions (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    iterations INTEGER DEFAULT 0,
    tool_calls INTEGER DEFAULT 0,
    tokens_used INTEGER DEFAULT 0,
    status TEXT DEFAULT 'running',
    fell_back INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS agent_steps (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    step_type TEXT NOT NULL,
    step_order INTEGER NOT NULL,
    input_summary TEXT,
    output_summary TEXT,
    tool_calls TEXT,
    token_used INTEGER DEFAULT 0,
    duration_ms INTEGER DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_memory (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_scan_runs_workspace ON scan_runs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_file_snapshots_workspace ON file_snapshots(workspace_id);
CREATE INDEX IF NOT EXISTS idx_file_snapshots_path ON file_snapshots(workspace_id, path);
CREATE INDEX IF NOT EXISTS idx_evidence_scan_run ON evidence(scan_run_id);
CREATE INDEX IF NOT EXISTS idx_reports_date ON reports(date);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_run ON agent_sessions(run_id);
`

func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }

func (s *Store) migrate() error {
	_, err := s.db.Exec(Schema)
	return err
}

// Workspace types
type WorkspaceRow struct {
	ID        int64
	Name      string
	Path      string
	Type      string
	Enabled   bool
	CreatedAt string
}

func (s *Store) EnsureWorkspace(name, wpath string) (int64, error) {
	var id int64
	err := s.db.QueryRow("SELECT id FROM workspaces WHERE path = ?", wpath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	result, err := s.db.Exec(
		"INSERT INTO workspaces (name, path, type, enabled, created_at) VALUES (?, ?, 'git_repo', 1, ?)",
		name, wpath, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListWorkspaces() ([]WorkspaceRow, error) {
	rows, err := s.db.Query("SELECT id, name, path, type, enabled, created_at FROM workspaces WHERE enabled = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkspaceRow
	for rows.Next() {
		var w WorkspaceRow
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.Type, &w.Enabled, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ScanRun types
type ScanRunRow struct {
	ID           string
	WorkspaceID  int64
	StartedAt    string
	FinishedAt   string
	FilesScanned int
	DiffFiles    int
	Redactions   int
	Status       string
}

func (s *Store) CreateScanRun(id string, workspaceID int64) error {
	_, err := s.db.Exec("INSERT INTO scan_runs (id, workspace_id, started_at, status) VALUES (?, ?, ?, 'running')",
		id, workspaceID, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) FinishScanRun(id string, filesScanned, diffFiles, redactions int, status string) error {
	_, err := s.db.Exec("UPDATE scan_runs SET finished_at=?, files_scanned=?, diff_files=?, redactions=?, status=? WHERE id=?",
		time.Now().UTC().Format(time.RFC3339), filesScanned, diffFiles, redactions, status, id)
	return err
}

func (s *Store) LatestScanRun(workspaceID int64) (*ScanRunRow, error) {
	row := s.db.QueryRow(
		"SELECT id, workspace_id, started_at, finished_at, files_scanned, diff_files, redactions, status FROM scan_runs WHERE workspace_id = ? ORDER BY started_at DESC LIMIT 1", workspaceID)
	var r ScanRunRow
	var finishedAt sql.NullString
	err := row.Scan(&r.ID, &r.WorkspaceID, &r.StartedAt, &finishedAt, &r.FilesScanned, &r.DiffFiles, &r.Redactions, &r.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		r.FinishedAt = finishedAt.String
	}
	return &r, nil
}

// FileSnapshot types
type FileSnapshotRow struct {
	ID          int64
	WorkspaceID int64
	Path        string
	Size        int64
	Mtime       string
	Hash        string
	LastSeenAt  string
}

func (s *Store) UpsertFileSnapshot(workspaceID int64, fpath string, size int64, mtime, hash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("INSERT INTO file_snapshots (workspace_id, path, size, mtime, hash, last_seen_at) VALUES (?, ?, ?, ?, ?, ?)",
		workspaceID, fpath, size, mtime, hash, now)
	if err != nil {
		_, err = s.db.Exec("UPDATE file_snapshots SET size=?, mtime=?, hash=?, last_seen_at=? WHERE workspace_id=? AND path=?",
			size, mtime, hash, now, workspaceID, fpath)
	}
	return err
}

func (s *Store) GetFileSnapshot(workspaceID int64, fpath string) (*FileSnapshotRow, error) {
	row := s.db.QueryRow("SELECT id, workspace_id, path, size, mtime, hash, last_seen_at FROM file_snapshots WHERE workspace_id=? AND path=?", workspaceID, fpath)
	var fs FileSnapshotRow
	var mtime, hash sql.NullString
	err := row.Scan(&fs.ID, &fs.WorkspaceID, &fs.Path, &fs.Size, &mtime, &hash, &fs.LastSeenAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fs.Mtime = mtime.String
	fs.Hash = hash.String
	return &fs, nil
}

type FileCandidate struct {
	Path  string
	Mtime string
	Hash  string
	Size  int64
}

func (s *Store) ChangedFiles(workspaceID int64, candidates []FileCandidate) ([]string, error) {
	var changed []string
	for _, c := range candidates {
		snap, err := s.GetFileSnapshot(workspaceID, c.Path)
		if err != nil {
			return nil, err
		}
		if snap == nil || snap.Mtime != c.Mtime || snap.Hash != c.Hash {
			changed = append(changed, c.Path)
		}
	}
	return changed, nil
}

func (s *Store) PruneFileSnapshots(workspaceID int64, cutoff string) error {
	_, err := s.db.Exec("DELETE FROM file_snapshots WHERE workspace_id = ? AND last_seen_at < ?", workspaceID, cutoff)
	return err
}

// Evidence types
type EvidenceRow struct {
	ID          string
	ScanRunID   string
	Type        string
	Workspace   string
	Path        string
	Summary     string
	Sensitivity string
	Source      string
	CreatedAt   string
}

func (s *Store) InsertEvidence(e EvidenceRow) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO evidence (id, scan_run_id, type, workspace, path, summary, sensitivity, source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		e.ID, e.ScanRunID, e.Type, e.Workspace, e.Path, e.Summary, e.Sensitivity, e.Source, e.CreatedAt)
	return err
}

func (s *Store) EvidenceByScanRun(scanRunID string) ([]EvidenceRow, error) {
	rows, err := s.db.Query("SELECT id, scan_run_id, type, workspace, path, summary, sensitivity, source, created_at FROM evidence WHERE scan_run_id = ?", scanRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvidenceRow
	for rows.Next() {
		var e EvidenceRow
		var path, summary, source sql.NullString
		if err := rows.Scan(&e.ID, &e.ScanRunID, &e.Type, &e.Workspace, &path, &summary, &e.Sensitivity, &source, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Path = path.String
		e.Summary = summary.String
		e.Source = source.String
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) EvidenceByDateRange(from, to string) ([]EvidenceRow, error) {
	rows, err := s.db.Query("SELECT id, scan_run_id, type, workspace, path, summary, sensitivity, source, created_at FROM evidence WHERE created_at >= ? AND created_at <= ? ORDER BY created_at", from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvidenceRow
	for rows.Next() {
		var e EvidenceRow
		var path, summary, source sql.NullString
		if err := rows.Scan(&e.ID, &e.ScanRunID, &e.Type, &e.Workspace, &path, &summary, &e.Sensitivity, &source, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Path = path.String
		e.Summary = summary.String
		e.Source = source.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// Report types
type ReportRow struct {
	ID         int64
	ScanRunID  string
	ReportType string
	Date       string
	Content    string
	Format     string
	CreatedAt  string
}

func (s *Store) InsertReport(scanRunID, reportType, date, content, format string) (int64, error) {
	var runRef any
	if scanRunID != "" {
		runRef = scanRunID
	}
	result, err := s.db.Exec("INSERT INTO reports (scan_run_id, report_type, date, content, format, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		runRef, reportType, date, content, format, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ReportsByDate(date string) ([]ReportRow, error) {
	rows, err := s.db.Query("SELECT id, scan_run_id, report_type, date, content, format, created_at FROM reports WHERE date = ? ORDER BY created_at", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRow
	for rows.Next() {
		var r ReportRow
		var scanRunID sql.NullString
		if err := rows.Scan(&r.ID, &scanRunID, &r.ReportType, &r.Date, &r.Content, &r.Format, &r.CreatedAt); err != nil {
			return nil, err
		}
		if scanRunID.Valid {
			r.ScanRunID = scanRunID.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) LatestReport(reportType, date string) (*ReportRow, error) {
	row := s.db.QueryRow("SELECT id, scan_run_id, report_type, date, content, format, created_at FROM reports WHERE report_type = ? AND date = ? ORDER BY created_at DESC LIMIT 1", reportType, date)
	var r ReportRow
	var scanRunID sql.NullString
	err := row.Scan(&r.ID, &scanRunID, &r.ReportType, &r.Date, &r.Content, &r.Format, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scanRunID.Valid {
		r.ScanRunID = scanRunID.String
	}
	return &r, nil
}

// Agent session types
func (s *Store) CreateAgentSession(id, runID string) error {
	_, err := s.db.Exec("INSERT INTO agent_sessions (id, run_id, started_at, status) VALUES (?, ?, ?, 'running')",
		id, runID, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) FinishAgentSession(id string, iterations, toolCalls, tokensUsed int, status string, fellBack bool) error {
	fb := 0
	if fellBack {
		fb = 1
	}
	_, err := s.db.Exec("UPDATE agent_sessions SET finished_at=?, iterations=?, tool_calls=?, tokens_used=?, status=?, fell_back=? WHERE id=?",
		time.Now().UTC().Format(time.RFC3339), iterations, toolCalls, tokensUsed, status, fb, id)
	return err
}

// Agent steps
type AgentStepRow struct {
	ID            string
	SessionID     string
	StepType      string
	StepOrder     int
	InputSummary  string
	OutputSummary string
	ToolCalls     string
	TokenUsed     int
	DurationMs    int
	CreatedAt     string
}

func (s *Store) InsertAgentStep(step AgentStepRow) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO agent_steps (id, session_id, step_type, step_order, input_summary, output_summary, tool_calls, token_used, duration_ms, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
		step.ID, step.SessionID, step.StepType, step.StepOrder, step.InputSummary, step.OutputSummary, step.ToolCalls, step.TokenUsed, step.DurationMs, step.CreatedAt)
	return err
}

// Agent memory
type AgentMemoryRow struct {
	ID        string
	Key       string
	Value     string
	UpdatedAt string
}

func (s *Store) GetAgentMemory(key string) (*AgentMemoryRow, error) {
	row := s.db.QueryRow("SELECT id, key, value, updated_at FROM agent_memory WHERE key = ?", key)
	var m AgentMemoryRow
	err := row.Scan(&m.ID, &m.Key, &m.Value, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) SetAgentMemory(id, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("INSERT OR REPLACE INTO agent_memory (id, key, value, updated_at) VALUES (?, ?, ?, ?)", id, key, value, now)
	return err
}

func (s *Store) AgentMemoryByPrefix(prefix string) ([]AgentMemoryRow, error) {
	rows, err := s.db.Query("SELECT id, key, value, updated_at FROM agent_memory WHERE key LIKE ? ORDER BY updated_at DESC", prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentMemoryRow
	for rows.Next() {
		var m AgentMemoryRow
		if err := rows.Scan(&m.ID, &m.Key, &m.Value, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MigrateJSONL reads Phase 0 evidence JSONL and inserts into store.
func (s *Store) MigrateJSONL(jsonlPath, workspaceName string, workspaceID int64) (int, error) {
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return 0, fmt.Errorf("read JSONL: %w", err)
	}
	scanRunID := fmt.Sprintf("migration-%s", time.Now().Format("20060102-150405"))
	if err := s.CreateScanRun(scanRunID, workspaceID); err != nil {
		return 0, fmt.Errorf("create migration scan run: %w", err)
	}
	count := 0
	for _, line := range splitJSONL(string(data)) {
		if line == "" {
			continue
		}
		e := EvidenceRow{
			ID: fmt.Sprintf("mig-%d", count), ScanRunID: scanRunID, Workspace: workspaceName,
			Sensitivity: "low", Source: "jsonl-migration", CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Summary: truncStr(line, 500), Type: "migrated",
		}
		if err := s.InsertEvidence(e); err != nil {
			continue
		}
		count++
	}
	if err := s.FinishScanRun(scanRunID, count, 0, 0, "completed"); err != nil {
		return count, fmt.Errorf("finish migration run: %w", err)
	}
	return count, nil
}

// InsertEvidenceJSONL inserts evidence items from JSONL into an existing scan run.
func (s *Store) InsertEvidenceJSONL(jsonlPath, workspaceName, scanRunID string) (int, error) {
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return 0, fmt.Errorf("read JSONL: %w", err)
	}
	count := 0
	for _, line := range splitJSONL(string(data)) {
		if line == "" {
			continue
		}
		var item evidence.Item
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		createdAt := item.CreatedAt
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}
		workspace := item.Workspace
		if workspace == "" {
			workspace = workspaceName
		}
		row := EvidenceRow{
			ID:          item.ID,
			ScanRunID:   scanRunID,
			Type:        string(item.Type),
			Workspace:   workspace,
			Path:        item.Path,
			Summary:     item.Summary,
			Sensitivity: string(item.Sensitivity),
			Source:      item.Source,
			CreatedAt:   createdAt,
		}
		if row.ID == "" {
			row.ID = fmt.Sprintf("%s-%d", scanRunID, count)
		} else {
			row.ID = fmt.Sprintf("%s:%s", scanRunID, row.ID)
		}
		if row.Sensitivity == "" {
			row.Sensitivity = "low"
		}
		if err := s.InsertEvidence(row); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

func splitJSONL(data string) []string {
	var lines []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
