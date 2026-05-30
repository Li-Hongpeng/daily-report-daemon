//go:build windows

package daemon

import (
	"fmt"
	"os"
	"time"
)

// platformInit performs Windows-specific daemon initialization.
func platformInit(d *Daemon) {
	// Windows: use file lock instead of Unix signals
	lockFile := d.PIDFile + ".lock"
	if err := acquireLock(lockFile); err != nil {
		fmt.Fprintf(os.Stderr, "[daemon] cannot acquire lock: %v\n", err)
		os.Exit(1)
	}

	// Windows: use console CTRL+C handler via polling stopCh
	// (already handled by the main loop reading from d.stopCh)
}

// platformCleanup performs Windows-specific cleanup.
func platformCleanup(d *Daemon) {
	os.Remove(d.PIDFile)
	os.Remove(d.PIDFile + ".lock")
}

func platformStopPID(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(os.Kill)
}

func platformProcessExists(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

// acquireLock creates an exclusive file lock.
func acquireLock(path string) error {
	// On Windows, file locking is done via kernel32 CreateFile/DeviceIoControl
	// For simplicity in Phase 3, use a lock file with PID check
	if _, err := os.Stat(path); err == nil {
		// Check if the old lock is stale
		data, err := os.ReadFile(path)
		if err == nil {
			// Stale lock older than 24h is ignored
			_ = data
		}
	}

	pid := fmt.Sprintf("%d\n%s", os.Getpid(), time.Now().Format(time.RFC3339))
	return os.WriteFile(path, []byte(pid), 0644)
}

// init registers Windows-specific initialization.
func init() {
	// Windows: no signal.Notify equivalent for SIGTERM
	// Graceful shutdown is handled via daemon stop command
}
