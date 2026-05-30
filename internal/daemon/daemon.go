package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/daily-report-daemon/internal/agent"
	"github.com/daily-report-daemon/internal/app"
	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/llm"
	"github.com/daily-report-daemon/internal/store"
)

// Daemon manages the background service lifecycle.
type Daemon struct {
	Workspaces []string
	Interval   time.Duration
	ReportTime string // "17:30" format
	PIDFile    string
	DBPath     string

	mu         sync.Mutex
	running    bool
	stopCh     chan struct{}
	lastScan   map[string]time.Time
	scanStates map[string]*ScanState // per-workspace incremental scan state
}

// New creates a Daemon with default settings.
func New(workspaces []string, outputDir string) *Daemon {
	return &Daemon{
		Workspaces: workspaces,
		Interval:   30 * time.Minute,
		ReportTime: "17:30",
		PIDFile:    filepath.Join(outputDir, "daemon.pid"),
		DBPath:     filepath.Join(outputDir, "daemon.db"),
		lastScan:   make(map[string]time.Time),
		scanStates: make(map[string]*ScanState),
		stopCh:     make(chan struct{}),
	}
}

// Start begins the daemon loop and writes a PID file.
func (d *Daemon) Start() error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon already running")
	}
	d.running = true
	d.mu.Unlock()

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(d.PIDFile), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Write PID file
	if err := os.WriteFile(d.PIDFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}

	// Restore scan state from SQLite for incremental scanning
	d.LoadState(d.DBPath)

	fmt.Println("daemon started")
	go d.loop()
	return nil
}

// LoadState restores per-workspace scan state from SQLite.
// Falls back silently if DB doesn't exist yet (first run).
func (d *Daemon) LoadState(dbPath string) {
	st, err := store.Open(dbPath)
	if err != nil {
		return // first run, no DB yet
	}
	defer st.Close()

	wsList, err := st.ListWorkspaces()
	if err != nil {
		return
	}

	for _, ws := range wsList {
		latest, err := st.LatestScanRun(ws.ID)
		if err != nil || latest == nil {
			continue
		}

		d.mu.Lock()
		if _, ok := d.scanStates[ws.Path]; !ok {
			d.scanStates[ws.Path] = NewScanState()
		}
		if t, err := time.Parse(time.RFC3339, latest.FinishedAt); err == nil {
			d.scanStates[ws.Path].LastScan = t
		}
		d.lastScan[ws.Path] = d.scanStates[ws.Path].LastScan
		d.mu.Unlock()

		fmt.Printf("[daemon] restored scan state for %s (last: %s, %d files)\n",
			ws.Path, latest.FinishedAt, latest.FilesScanned)
	}

	// Also ensure workspaces are registered in store
	for _, ws := range d.Workspaces {
		st.EnsureWorkspace(filepath.Base(ws), ws)
	}
}

// Stop signals the daemon to shut down and cleans up.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon not running")
	}
	d.running = false
	d.mu.Unlock()

	close(d.stopCh)
	platformCleanup(d)
	fmt.Println("daemon stopped")
	return nil
}

// Status returns the current daemon state.
func (d *Daemon) Status() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return "running"
	}
	if _, err := os.Stat(d.PIDFile); err == nil {
		return "stale (PID file exists but process may be dead)"
	}
	return "stopped"
}

// Restart stops and starts the daemon.
func (d *Daemon) Restart() error {
	if err := d.Stop(); err != nil {
		// Ignore "not running" error on restart
		if err.Error() != "daemon not running" {
			return err
		}
	}
	// Recreate stopCh since it was closed by Stop
	d.stopCh = make(chan struct{})
	time.Sleep(500 * time.Millisecond)
	return d.Start()
}

func (d *Daemon) loop() {
	scanTicker := time.NewTicker(d.Interval)
	defer scanTicker.Stop()

	reportTicker := time.NewTicker(1 * time.Minute) // check report time every minute
	defer reportTicker.Stop()

	// Initial scan on start
	d.runScan()

	// Platform-specific signal handling
	platformInit(d)

	for {
		select {
		case <-d.stopCh:
			platformCleanup(d)
			return
		case <-scanTicker.C:
			d.runScan()
		case <-reportTicker.C:
			if d.isReportTime() {
				d.runReport()
			}
		}
	}
}

func (d *Daemon) isReportTime() bool {
	now := time.Now().Format("15:04")
	return now == d.ReportTime
}

func (d *Daemon) runScan() {
	for _, ws := range d.Workspaces {
		fmt.Printf("[daemon] scanning %s...\n", ws)

		// Track incremental state
		d.mu.Lock()
		state, ok := d.scanStates[ws]
		if !ok {
			state = NewScanState()
			d.scanStates[ws] = state
		}
		d.mu.Unlock()

		// Report what changed since last scan
		if state.LastScan.IsZero() {
			fmt.Printf("[daemon] %s: initial full scan\n", ws)
		} else {
			fmt.Printf("[daemon] %s: incremental scan (last: %s) — %s\n",
				ws, state.LastScan.Format("15:04:05"), state.Summary())
		}

		// Run the actual scanner
		a := &app.App{Workspace: ws, DryRun: false, NoLLM: false}
		result, err := a.Scan()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] scan error for %s: %v\n", ws, err)
			continue
		}

		fmt.Printf("[daemon] %s: %d files, %d diffs, %d redactions\n",
			ws, result.FilesScanned, result.DiffFiles, result.Redactions)

		// Persist scan run to SQLite store
		d.persistScan(ws, result)

		d.mu.Lock()
		state.LastScan = time.Now()
		d.lastScan[ws] = state.LastScan
		d.mu.Unlock()
	}
}

// persistScan saves scan results to SQLite.
func (d *Daemon) persistScan(workspace string, result *app.RunResult) {
	st, err := store.Open(d.DBPath)
	if err != nil {
		return // silently skip if DB unavailable
	}
	defer st.Close()

	wsID, err := st.EnsureWorkspace(filepath.Base(workspace), workspace)
	if err != nil {
		return
	}

	runID := fmt.Sprintf("scan-%s", time.Now().Format("20060102-150405"))
	if err := st.CreateScanRun(runID, wsID); err != nil {
		return
	}

	st.FinishScanRun(runID, result.FilesScanned, result.DiffFiles, result.Redactions, "completed")

	// Persist evidence if available
	if result.EvidencePath != "" {
		st.MigrateJSONL(result.EvidencePath, filepath.Base(workspace), wsID)
	}
}

func (d *Daemon) runReport() {
	fmt.Println("[daemon] generating scheduled report...")
	for _, ws := range d.Workspaces {
		// Load config for LLM settings
		cfgPath := filepath.Join(ws, ".daily-report-daemon", "config.yaml")
		cfg, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] config error for %s: %v\n", ws, err)
			continue
		}

		// Create LLM client
		outputDir := filepath.Join(ws, ".daily-report-daemon")
		llmClient, err := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.LLM.APIKeyEnv, false, outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] LLM client error for %s: %v\n", ws, err)
			continue
		}

		// Run scan first to get fresh evidence
		a := &app.App{Workspace: ws, DryRun: false, NoLLM: false}
		scanResult, err := a.Scan()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] pre-report scan error: %v\n", err)
			continue
		}

		// Run agent engine
		ag := agent.NewAgent(ws, llmClient)
		var evidenceData []byte; if scanResult.EvidencePath != "" { evidenceData, _ = os.ReadFile(scanResult.EvidencePath) }; if len(evidenceData) == 0 { evidenceData = []byte(fmt.Sprintf(`{"files":%d,"diffs":%d}`, scanResult.FilesScanned, scanResult.DiffFiles)) }
		_, err = ag.Run(nil, string(evidenceData))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[daemon] agent error for %s: %v\n", ws, err)
			continue
		}

		fmt.Printf("[daemon] report generated for %s\n", ws)
	}
}

// LastScan returns the time of the last scan for a workspace.
func (d *Daemon) LastScan(workspace string) (time.Time, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.lastScan[workspace]
	return t, ok
}
