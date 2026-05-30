package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("/tmp/test-repo")
	if cfg.Version != "1" {
		t.Errorf("expected version 1, got %s", cfg.Version)
	}
	if cfg.Language != "zh-CN" {
		t.Errorf("expected zh-CN, got %s", cfg.Language)
	}
	if cfg.Workspace.Path != "/tmp/test-repo" {
		t.Errorf("expected workspace path /tmp/test-repo, got %s", cfg.Workspace.Path)
	}
	if cfg.Workspace.Type != "git_repo" {
		t.Errorf("expected git_repo, got %s", cfg.Workspace.Type)
	}
	if !cfg.Workspace.GitEnabled {
		t.Error("expected git_enabled true")
	}
	if cfg.Agent.MaxIterations != 8 {
		t.Errorf("expected agent max_iterations 8, got %d", cfg.Agent.MaxIterations)
	}
	if cfg.Agent.MaxToolCalls != 12 {
		t.Errorf("expected agent max_tool_calls 12, got %d", cfg.Agent.MaxToolCalls)
	}
	if cfg.Agent.TraceEnabled == nil || !*cfg.Agent.TraceEnabled {
		t.Error("expected agent trace enabled by default")
	}
}

func TestSaveAndLoad(t *testing.T) {
	cfg := DefaultConfig("/tmp/test-load")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Workspace.Path != cfg.Workspace.Path {
		t.Errorf("workspace path mismatch: got %s, want %s", loaded.Workspace.Path, cfg.Workspace.Path)
	}
	if loaded.LLM.Model != cfg.LLM.Model {
		t.Errorf("model mismatch: got %s, want %s", loaded.LLM.Model, cfg.LLM.Model)
	}
}

func TestDefaultLLMConfigDeepSeek(t *testing.T) {
	os.Setenv("DEEPSEEK_API_KEY", "sk-test-ds")
	defer os.Unsetenv("DEEPSEEK_API_KEY")
	cfg := defaultLLMConfig()
	if cfg.BaseURL != "https://api.deepseek.com" {
		t.Errorf("expected DeepSeek base URL, got %s", cfg.BaseURL)
	}
	if cfg.Model != "deepseek-chat" {
		t.Errorf("expected deepseek-chat, got %s", cfg.Model)
	}
	if cfg.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Errorf("expected DEEPSEEK_API_KEY, got %s", cfg.APIKeyEnv)
	}
}

func TestDefaultLLMConfigOpenAI(t *testing.T) {
	os.Unsetenv("DEEPSEEK_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	cfg := defaultLLMConfig()
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected OpenAI base URL, got %s", cfg.BaseURL)
	}
}

func TestCheckAPIKeyMissing(t *testing.T) {
	cfg := DefaultConfig("/tmp/test")
	// Ensure env var is unset
	os.Unsetenv("OPENAI_API_KEY")
	if err := cfg.CheckAPIKey(); err == nil {
		t.Error("expected error when API key not set")
	}
}

func TestCheckAPIKeyPresent(t *testing.T) {
	cfg := DefaultConfig("/tmp/test")
	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	if err := cfg.CheckAPIKey(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
