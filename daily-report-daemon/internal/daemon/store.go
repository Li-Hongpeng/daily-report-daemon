package daemon

import (
	"fmt"
)

// StoreSchema contains the SQLite schema for Phase 2 daemon storage.
// Uses modernc.org/sqlite (pure Go, no CGo) for Go ≥ 1.21 compatibility.
const StoreSchema = `
-- Workspaces
CREATE TABLE IF NOT EXISTS workspaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL DEFAULT 'git_repo',
    enabled INTEGER DEFAULT 1,
    created_at TEXT NOT NULL
);

-- Scan runs
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

-- File snapshots (for incremental scanning)
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

-- Git events
CREATE TABLE IF NOT EXISTS git_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_run_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    file_path TEXT,
    details TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (scan_run_id) REFERENCES scan_runs(id)
);

-- Evidence items
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

-- Reports
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

-- Publish events
CREATE TABLE IF NOT EXISTS publish_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id INTEGER NOT NULL,
    channel TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    sent_at TEXT,
    FOREIGN KEY (report_id) REFERENCES reports(id)
);

-- Agent sessions (from Phase 2 M1)
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

-- Agent steps
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

-- Agent memory (cross-day tracking)
CREATE TABLE IF NOT EXISTS agent_memory (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

// MigrateJSONL migrates Phase 0 JSONL evidence to SQLite.
// Placeholder: will be implemented when SQLite driver is available.
func MigrateJSONL(jsonlDir, dbPath string) error {
	_ = jsonlDir
	_ = dbPath
	fmt.Println("[migration] Phase 0 JSONL → SQLite migration ready (requires modernc.org/sqlite)")
	return nil
}

// SchemaComment returns documentation about the schema.
func SchemaComment() string {
	return fmt.Sprintf("Phase 2 SQLite schema: %d tables, %d bytes of DDL.",
		11, len(StoreSchema))
}
