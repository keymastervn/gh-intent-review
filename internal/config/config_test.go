package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", cfg.LLM.Provider)
	}
	if cfg.Review.Parallel != 4 {
		t.Errorf("expected parallel 4, got %d", cfg.Review.Parallel)
	}
	if cfg.Output.Dir != "" {
		t.Errorf("expected empty output dir (use ~), got %q", cfg.Output.Dir)
	}
}

func TestEnabledSymbols(t *testing.T) {
	cfg := DefaultConfig()
	enabled := cfg.EnabledSymbols()

	// $ is disabled by default, so 6 of 7 should be enabled
	if len(enabled) != 6 {
		t.Errorf("expected 6 enabled symbols, got %d", len(enabled))
	}

	// Check that $ is not in the enabled list
	for _, s := range enabled {
		if s.Symbol == "$" {
			t.Error("expected '$' to be disabled by default")
		}
	}
}

func TestDefaultIntentSymbols(t *testing.T) {
	symbols := DefaultIntentSymbols()
	if len(symbols) != 7 {
		t.Errorf("expected 7 default symbols, got %d", len(symbols))
	}

	// Verify categories
	reliabilityCount := 0
	formCount := 0
	for _, s := range symbols {
		switch s.Category {
		case "reliability":
			reliabilityCount++
		case "form":
			formCount++
		default:
			t.Errorf("unexpected category %q for symbol %q", s.Category, s.Symbol)
		}
	}
	if reliabilityCount != 3 {
		t.Errorf("expected 3 reliability symbols, got %d", reliabilityCount)
	}
	if formCount != 4 {
		t.Errorf("expected 4 form symbols, got %d", formCount)
	}
}

func TestLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".gh-intent-review.yml")

	content := `
llm:
  provider: openai
  model: gpt-4o
review:
  parallel: 8
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to tmpDir so Load() finds the config
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", cfg.LLM.Model)
	}
	if cfg.Review.Parallel != 8 {
		t.Errorf("expected parallel 8, got %d", cfg.Review.Parallel)
	}
}

func TestConfigString(t *testing.T) {
	cfg := DefaultConfig()
	s := cfg.String()
	if s == "" {
		t.Error("expected non-empty config string")
	}
}

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	path, err := Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected config file to exist")
	}
}

func TestEnvVarOverride(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	t.Setenv("ANTHROPIC_API_KEY", "test-key-123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLM.APIKey != "test-key-123" {
		t.Errorf("expected API key from env var, got %q", cfg.LLM.APIKey)
	}
}
