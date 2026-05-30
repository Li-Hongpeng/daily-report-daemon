package agent

import (
	"fmt"
)

// InitDB is a placeholder for SQLite initialization.
// Full implementation requires Go ≥ 1.21 + modernc.org/sqlite (pure Go driver).
// Schema is defined below for reference.
func InitDB(dbPath string) error {
	// Placeholder: return nil until SQLite driver is available
	_ = dbPath
	return nil
}

// SchemaSQL contains the CREATE TABLE statements for agent tables.
const SchemaSQL = `
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
`

// SchemaComment documents the SQLite dependency decision.
func SchemaComment() string {
	return fmt.Sprintf("SQLite schema: 3 tables (%d bytes). Use modernc.org/sqlite (pure Go).", len(SchemaSQL))
}
