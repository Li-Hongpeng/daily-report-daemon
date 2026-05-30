package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for daily-report-daemon.
type Config struct {
	Version   string          `yaml:"version"`
	Language  string          `yaml:"language"`
	Workspace WorkspaceConfig `yaml:"workspace"` // single workspace in Phase 0
	LLM       LLMConfig       `yaml:"llm"`
	Agent     AgentConfig     `yaml:"agent"`
	Reports   ReportsConfig   `yaml:"reports"`
	Daemon    DaemonConfig    `yaml:"daemon"`
	Publisher PublisherConfig `yaml:"publisher"`
}

// WorkspaceConfig defines a single workspace in Phase 0.
type WorkspaceConfig struct {
	Name         string   `yaml:"name"`
	Path         string   `yaml:"path"`
	Type         string   `yaml:"type"` // "git_repo" in Phase 0
	Include      []string `yaml:"include"`
	Exclude      []string `yaml:"exclude"`
	MaxFileBytes int64    `yaml:"max_file_bytes"`
	GitEnabled   bool     `yaml:"git_enabled"`
	DocsEnabled  bool     `yaml:"docs_enabled"`
}

// LLMConfig holds model provider settings.
type LLMConfig struct {
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// AgentConfig controls the Eino ADK runtime.
type AgentConfig struct {
	MaxIterations       int   `yaml:"max_iterations"`
	MaxToolCalls        int   `yaml:"max_tool_calls"`
	ToolTimeoutSeconds  int   `yaml:"tool_timeout_seconds"`
	TotalTimeoutSeconds int   `yaml:"total_timeout_seconds"`
	TokenBudgetDaily    int   `yaml:"token_budget_daily"`
	TraceEnabled        *bool `yaml:"trace_enabled"`
}

// ReportsConfig holds report generation settings.
type ReportsConfig struct {
	OutputDir       string `yaml:"output_dir"`
	WeeklyOutputDir string `yaml:"weekly_output_dir"`
	EvidenceLevel   string `yaml:"evidence_level"`
	IncludeBaseline bool   `yaml:"include_baseline"`
}

// DaemonConfig controls background scanning and report schedules.
type DaemonConfig struct {
	ScanIntervalMinutes int    `yaml:"scan_interval_minutes"`
	DailyReportTime     string `yaml:"daily_report_time"`
	WeeklyReportDay     string `yaml:"weekly_report_day"`
	WeeklyReportTime    string `yaml:"weekly_report_time"`
}

// PublisherConfig holds upstream publish settings.
type PublisherConfig struct {
	Enabled        bool           `yaml:"enabled"`
	PrimaryChannel string         `yaml:"primary_channel"`
	DingTalk       DingTalkConfig `yaml:"dingtalk,omitempty"`
}

// DingTalkConfig holds DingTalk webhook settings.
type DingTalkConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	AutoSend   bool   `yaml:"auto_send"`
}

// defaultLLMConfig detects DeepSeek vs OpenAI based on available env vars.
func defaultLLMConfig() LLMConfig {
	if os.Getenv("DEEPSEEK_API_KEY") != "" {
		return LLMConfig{
			Provider:  "openai-compatible",
			BaseURL:   "https://api.deepseek.com",
			Model:     "deepseek-chat",
			APIKeyEnv: "DEEPSEEK_API_KEY",
		}
	}
	return LLMConfig{
		Provider:  "openai-compatible",
		BaseURL:   "https://api.openai.com/v1",
		Model:     "gpt-4.1-mini",
		APIKeyEnv: "OPENAI_API_KEY",
	}
}

// DefaultConfig returns a Config with sensible defaults for the given workspace path.
// Auto-detects DeepSeek vs OpenAI based on available environment variables.
func DefaultConfig(workspacePath string) Config {
	return Config{
		Version:  "1",
		Language: "zh-CN",
		Workspace: WorkspaceConfig{
			Name:         filepath.Base(workspacePath),
			Path:         workspacePath,
			Type:         "git_repo",
			Include:      []string{"**/*"},
			Exclude:      defaultExcludes(),
			MaxFileBytes: 262144,
			GitEnabled:   true,
			DocsEnabled:  true,
		},
		LLM: defaultLLMConfig(),
		Agent: AgentConfig{
			MaxIterations:       8,
			MaxToolCalls:        12,
			ToolTimeoutSeconds:  30,
			TotalTimeoutSeconds: 300,
			TokenBudgetDaily:    20000,
			TraceEnabled:        boolPtr(true),
		},
		Reports: ReportsConfig{
			OutputDir:       ".daily-report-daemon/reports",
			WeeklyOutputDir: ".daily-report-daemon/reports/weekly",
			EvidenceLevel:   "normal",
			IncludeBaseline: false,
		},
		Daemon: DaemonConfig{
			ScanIntervalMinutes: 30,
			DailyReportTime:     "17:30",
			WeeklyReportDay:     "friday",
			WeeklyReportTime:    "17:45",
		},
		Publisher: PublisherConfig{
			Enabled:        false,
			PrimaryChannel: "email",
		},
	}
}

func defaultExcludes() []string {
	return []string{
		"**/node_modules/**",
		"**/.venv/**",
		"**/dist/**",
		"**/build/**",
		"**/.git/**",
		"**/.daily-report-daemon/**",
		"**/.env*",
		"**/*.pem",
		"**/*secret*",
		"**/*token*",
	}
}

// Load reads and parses a config YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.ApplyDefaults()
	return &cfg, nil
}

// ApplyDefaults fills fields added after older config files were generated.
func (c *Config) ApplyDefaults() {
	if c.Language == "" {
		c.Language = "zh-CN"
	}
	if c.Agent.MaxIterations <= 0 {
		c.Agent.MaxIterations = 8
	}
	if c.Agent.MaxToolCalls <= 0 {
		c.Agent.MaxToolCalls = 12
	}
	if c.Agent.ToolTimeoutSeconds <= 0 {
		c.Agent.ToolTimeoutSeconds = 30
	}
	if c.Agent.TotalTimeoutSeconds <= 0 {
		c.Agent.TotalTimeoutSeconds = 300
	}
	if c.Agent.TokenBudgetDaily <= 0 {
		c.Agent.TokenBudgetDaily = 20000
	}
	if c.Agent.TraceEnabled == nil {
		c.Agent.TraceEnabled = boolPtr(true)
	}
	if c.Reports.OutputDir == "" {
		c.Reports.OutputDir = ".daily-report-daemon/reports"
	}
	if c.Reports.WeeklyOutputDir == "" {
		c.Reports.WeeklyOutputDir = ".daily-report-daemon/reports/weekly"
	}
}

func boolPtr(v bool) *bool {
	return &v
}

// Save writes the config to a YAML file.
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Validate checks the config for obvious problems.
func (c *Config) Validate() error {
	if c.Workspace.Path == "" {
		return fmt.Errorf("workspace.path is required")
	}
	info, err := os.Stat(c.Workspace.Path)
	if err != nil {
		return fmt.Errorf("workspace.path not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace.path is not a directory: %s", c.Workspace.Path)
	}
	return nil
}

// CheckAPIKey verifies the configured API key env var is set.
func (c *Config) CheckAPIKey() error {
	key := os.Getenv(c.LLM.APIKeyEnv)
	if key == "" {
		return fmt.Errorf("API key not set: environment variable %s is empty; set it before generating reports", c.LLM.APIKeyEnv)
	}
	return nil
}
