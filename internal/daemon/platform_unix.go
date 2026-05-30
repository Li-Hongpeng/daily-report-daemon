//go:build !windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"
)

// platformInit performs Unix-specific daemon initialization.
func platformInit(d *Daemon) {
	// Signal handling for graceful shutdown (Unix: SIGINT, SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		d.Stop()
	}()
}

// platformCleanup performs Unix-specific cleanup.
func platformCleanup(d *Daemon) {
	os.Remove(d.PIDFile)
}
