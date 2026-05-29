package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for daily-report-daemon.
type Config struct {
	Version   string              `yaml:"version"`
	Language  string              `yaml:"language"`
	Workspace WorkspaceConfig     `yaml:"workspace"`  // single workspace in Phase 0
	LLM       LLMConfig           `yaml:"llm"`
	Reports   ReportsConfig       `yaml:"reports"`
	Publisher PublisherConfig     `yaml:"publisher"`
}

// WorkspaceConfig defines a single workspace in Phase 0.
type WorkspaceConfig struct {
	Name        string   `yaml:"name"`
	Path        string   `yaml:"path"`
	Type        string   `yaml:"type"` // "git_repo" in Phase 0
	Include     []string `yaml:"include"`
	Exclude     []string `yaml:"exclude"`
	MaxFileBytes int64   `yaml:"max_file_bytes"`
	GitEnabled  bool     `yaml:"git_enabled"`
	DocsEnabled bool     `yaml:"docs_enabled"`
}

// LLMConfig holds model provider settings.
type LLMConfig struct {
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// ReportsConfig holds report generation settings.
type ReportsConfig struct {
	OutputDir     string `yaml:"output_dir"`
	EvidenceLevel string `yaml:"evidence_level"`
}

// PublisherConfig holds upstream publish settings.
type PublisherConfig struct {
	Enabled        bool   `yaml:"enabled"`
	PrimaryChannel string `yaml:"primary_channel"`
}

// DefaultConfig returns a Config with sensible defaults for the given workspace path.
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
			MaxFileBytes: 262144, // 256KB
			GitEnabled:   true,
			DocsEnabled:  true,
		},
		LLM: LLMConfig{
			Provider:  "openai-compatible",
			BaseURL:   "https://api.openai.com/v1",
			Model:     "gpt-4.1-mini",
			APIKeyEnv: "OPENAI_API_KEY",
		},
		Reports: ReportsConfig{
			OutputDir:     ".daily-report-daemon/reports",
			EvidenceLevel: "normal",
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
	return &cfg, nil
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
