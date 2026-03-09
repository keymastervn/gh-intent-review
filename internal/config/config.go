package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	configFileName = ".gh-intent-review.yml"
)

// Config holds all configuration for the extension.
type Config struct {
	LLM     LLMConfig     `yaml:"llm"`
	Review  ReviewConfig  `yaml:"review"`
	Intents IntentsConfig `yaml:"intents"`
	Output  OutputConfig  `yaml:"output"`
}

// LLMConfig configures the AI provider used for review.
type LLMConfig struct {
	Provider string `yaml:"provider"` // "anthropic", "openai", "ollama", "agent"
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key,omitempty"` // can also use env vars
	BaseURL  string `yaml:"base_url,omitempty"`

	// Agent provider settings (provider: agent)
	AgentCommand string `yaml:"agent_command,omitempty"` // CLI binary to invoke (default: "claude")
}

// ReviewConfig controls the review process.
type ReviewConfig struct {
	Parallel     int      `yaml:"parallel"`
	IgnoreFiles  []string `yaml:"ignore_files,omitempty"`  // glob patterns
	FocusFiles   []string `yaml:"focus_files,omitempty"`   // glob patterns
	CustomPrompt string   `yaml:"custom_prompt,omitempty"` // appended to the system prompt
}

// IntentSymbol defines a single intent notation.
type IntentSymbol struct {
	Symbol      string `yaml:"symbol"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Enabled     bool   `yaml:"enabled"`
	Category    string `yaml:"category"` // "reliability" or "form"
}

// IntentsConfig controls which intent symbols are active.
type IntentsConfig struct {
	Symbols []IntentSymbol `yaml:"symbols"`
}

// OutputConfig controls where and how diffs are stored.
type OutputConfig struct {
	Dir    string `yaml:"dir"`    // base directory for storing focused diffs
	Format string `yaml:"format"` // "text" or "json"
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-6",
		},
		Review: ReviewConfig{
			Parallel: 4,
			IgnoreFiles: []string{
				"*.lock",
				"*.sum",
				"vendor/**",
				"node_modules/**",
			},
		},
		Intents: IntentsConfig{
			Symbols: DefaultIntentSymbols(),
		},
		Output: OutputConfig{
			Dir:    "", // empty = use ~/.gh-intent-review/
			Format: "text",
		},
	}
}

// DefaultIntentSymbols returns the built-in intent symbols.
func DefaultIntentSymbols() []IntentSymbol {
	return []IntentSymbol{
		// Reliability
		{Symbol: "!", Name: "Security Risk", Description: "Vulnerabilities: SQL injection, XSS, exposed secrets, etc.", Enabled: true, Category: "reliability"},
		{Symbol: "~", Name: "Performance Drag", Description: "Latency, slow execution, performance bottlenecks.", Enabled: true, Category: "reliability"},
		{Symbol: "$", Name: "Resource Cost", Description: "Expensive operations, memory leaks, compute waste.", Enabled: false, Category: "reliability"},
		// Form
		{Symbol: "&", Name: "Coupling Violation", Description: "Tight coupling, hardcoded dependencies.", Enabled: true, Category: "form"},
		{Symbol: "#", Name: "Cohesion / SOLID Issue", Description: "Low cohesion, single responsibility violations.", Enabled: true, Category: "form"},
		{Symbol: "=", Name: "DRY Violation", Description: "Code duplication, repeated logic.", Enabled: true, Category: "form"},
		{Symbol: "?", Name: "KISS Violation", Description: "Overly clever, deeply nested, hard to read code.", Enabled: true, Category: "form"},
	}
}

// EnabledSymbols returns only the enabled intent symbols.
func (c *Config) EnabledSymbols() []IntentSymbol {
	var enabled []IntentSymbol
	for _, s := range c.Intents.Symbols {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled
}

// Load reads config from the nearest .gh-intent-review.yml, falling back to defaults.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Search current dir, then home dir
	paths := []string{
		filepath.Join(".", configFileName),
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, configFileName))
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		break
	}

	// Env var overrides for API key
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = key
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" && cfg.LLM.Provider == "openai" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = key
	}

	return cfg, nil
}

// Init writes a default config file to the current directory.
func Init() (string, error) {
	cfg := DefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}

	path := filepath.Join(".", configFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// String returns the config as YAML.
func (c *Config) String() string {
	data, _ := yaml.Marshal(c)
	return string(data)
}
