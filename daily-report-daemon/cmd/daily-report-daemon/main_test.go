package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCmdVersion(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "daily-report-daemon",
		Version: "0.1.0-dev",
	}
	cmd.SetVersionTemplate("daily-report-daemon {{.Version}}\n")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "0.1.0-dev") {
		t.Errorf("expected version in output, got: %s", got)
	}
}

func TestRootCmdHelp(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "daily-report-daemon",
		Long: "daily-report-daemon is a local-first tool.",
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "daily-report-daemon") {
		t.Errorf("expected help output to contain app name, got: %s", got)
	}
}
