package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daily-report-daemon/internal/app"
	"github.com/daily-report-daemon/internal/config"
	"github.com/daily-report-daemon/internal/daemon"
)

var version = "0.1.0-dev"

func main() {
	var workspacePath string
	var dryRun bool
	var noLLM bool

	rootCmd := &cobra.Command{
		Use:   "daily-report-daemon",
		Short: "daily-report-daemon – local dev activity observer & report generator",
		Long: `daily-report-daemon is a local-first tool that collects code and document
change signals from authorized workspaces and uses LLMs to generate
daily reports, weekly reports, code change analysis, risk alerts, and
agent context files (AGENTS.generated.md) for coding agents.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVarP(&workspacePath, "workspace", "w", "", "workspace path (default: current directory)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "save model input without calling LLM")
	rootCmd.PersistentFlags().BoolVar(&noLLM, "no-llm", false, "skip LLM and report generation entirely")

	rootCmd.AddCommand(newInitCmd(&workspacePath))
	rootCmd.AddCommand(newScanCmd(&workspacePath, &dryRun, &noLLM))
	rootCmd.AddCommand(newReportCmd(&workspacePath, &dryRun, &noLLM))
	rootCmd.AddCommand(newAgentContextCmd(&workspacePath, &dryRun, &noLLM))
	rootCmd.AddCommand(newRunCmd(&workspacePath, &dryRun, &noLLM))
	rootCmd.AddCommand(newDaemonCmd(&workspacePath))

	rootCmd.SetVersionTemplate("daily-report-daemon {{.Version}}\n")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveWorkspace(wp string) (string, error) {
	if wp == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot determine current directory (use --workspace): %w", err)
		}
		return cwd, nil
	}
	return filepath.Abs(wp)
}

func newInitCmd(wp *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize daily-report-daemon in a workspace",
		Long: `Initialize daily-report-daemon by creating a .daily-report-daemon directory
with a config.yaml and the required subdirectories (runs, reports, context).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := resolveWorkspace(*wp)
			if err != nil {
				return err
			}

			info, err := os.Stat(abs)
			if err != nil {
				return fmt.Errorf("workspace does not exist or is not accessible: %s\n  %w", abs, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("workspace path is not a directory: %s", abs)
			}

			key := os.Getenv("OPENAI_API_KEY")
			dsKey := os.Getenv("DEEPSEEK_API_KEY")
			if key == "" && dsKey == "" {
				fmt.Fprintln(os.Stderr, "⚠  No API key found. Set one before generating reports:")
				fmt.Fprintln(os.Stderr, "   export OPENAI_API_KEY=sk-...")
				fmt.Fprintln(os.Stderr, "   export DEEPSEEK_API_KEY=sk-...")
			}
			if dsKey != "" {
				fmt.Fprintln(os.Stderr, "✓  DEEPSEEK_API_KEY detected — using DeepSeek API (deepseek-chat)")
			}

			cfg := config.DefaultConfig(abs)

			daemonDir := filepath.Join(abs, ".daily-report-daemon")
			for _, sub := range []string{"runs", "reports", "context"} {
				if err := os.MkdirAll(filepath.Join(daemonDir, sub), 0755); err != nil {
					return fmt.Errorf("create %s: %w", sub, err)
				}
			}

			configPath := filepath.Join(daemonDir, "config.yaml")
			if err := cfg.Save(configPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Printf("Initialized daily-report-daemon in %s\n", abs)
			fmt.Printf("  config:  %s\n", configPath)
			fmt.Printf("  runs:    %s/runs\n", daemonDir)
			fmt.Printf("  reports: %s/reports\n", daemonDir)
			fmt.Printf("  context: %s/context\n", daemonDir)

			return nil
		},
	}
	return cmd
}

func newScanCmd(wp *string, dryRun, noLLM *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan workspace and collect evidence (no LLM call)",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := resolveWorkspace(*wp)
			if err != nil {
				return err
			}
			a := &app.App{Workspace: abs, DryRun: *dryRun, NoLLM: true}
			result, err := a.Scan()
			fmt.Print(result.Summary())
			return err
		},
	}
	return cmd
}

func newReportCmd(wp *string, dryRun, noLLM *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate daily report from scanned evidence",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := resolveWorkspace(*wp)
			if err != nil {
				return err
			}
			a := &app.App{Workspace: abs, DryRun: *dryRun, NoLLM: *noLLM}
			result, err := a.Report()
			fmt.Print(result.Summary())
			return err
		},
	}
	return cmd
}

func newAgentContextCmd(wp *string, dryRun, noLLM *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent-context",
		Short: "Generate AGENTS.generated.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := resolveWorkspace(*wp)
			if err != nil {
				return err
			}
			a := &app.App{Workspace: abs, DryRun: *dryRun, NoLLM: *noLLM}
			result, err := a.AgentContext()
			fmt.Print(result.Summary())
			return err
		},
	}
	return cmd
}

func newRunCmd(wp *string, dryRun, noLLM *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "One-shot: scan + report + agent-context",
		Long: `Run the full pipeline: scan the workspace, generate a daily report,
and produce AGENTS.generated.md. Use --dry-run to skip LLM calls,
or --no-llm to skip LLM and report generation entirely.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := resolveWorkspace(*wp)
			if err != nil {
				return err
			}
			a := &app.App{Workspace: abs, DryRun: *dryRun, NoLLM: *noLLM}
			result, err := a.Run()
			fmt.Print(result.Summary())
			return err
		},
	}
	return cmd
}

func newDaemonCmd(wp *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the background service (start/stop/status/restart)",
	}

	var outputDir string
	cmd.PersistentFlags().StringVar(&outputDir, "output-dir", ".daily-report-daemon", "daemon data directory")

	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, _ := resolveWorkspace(*wp)
			d := daemon.New([]string{abs}, filepath.Join(abs, outputDir))
			return d.Start()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, _ := resolveWorkspace(*wp)
			d := daemon.New([]string{abs}, filepath.Join(abs, outputDir))
			return d.Stop()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, _ := resolveWorkspace(*wp)
			d := daemon.New([]string{abs}, filepath.Join(abs, outputDir))
			fmt.Println(d.Status())
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "restart",
		Short: "Restart the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, _ := resolveWorkspace(*wp)
			d := daemon.New([]string{abs}, filepath.Join(abs, outputDir))
			return d.Restart()
		},
	})
	return cmd
}
