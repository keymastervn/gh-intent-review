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

// LLMConfig configures the AI agent used for review.
type LLMConfig struct {
	Provider     string `yaml:"provider"`                // only "agent" is supported
	Model        string `yaml:"model,omitempty"`         // passed to the agent via --model
	AgentCommand string `yaml:"agent_command,omitempty"` // CLI binary to invoke (default: "claude")
}

// ReviewConfig controls the review process.
type ReviewConfig struct {
	IgnoreFiles   []string `yaml:"ignore_files,omitempty"`    // glob patterns
	FocusFiles    []string `yaml:"focus_files,omitempty"`     // glob patterns
	CustomPrompt  string   `yaml:"custom_prompt,omitempty"`   // appended to the system prompt
	CheckAndFetch bool     `yaml:"check_and_fetch,omitempty"` // auto-regenerate if PR head changed
}

// IntentSymbol defines a single intent notation.
type IntentSymbol struct {
	Symbol      string `yaml:"symbol"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Enabled     bool   `yaml:"enabled"`
	Category    string `yaml:"category"`           // "reliability" or "form"
	Severity    string `yaml:"severity,omitempty"` // trivial, minor, major, critical
}

// IntentsConfig controls which intent symbols are active and the minimum severity to report.
type IntentsConfig struct {
	Symbols  []IntentSymbol `yaml:"symbols"`
	Severity string         `yaml:"severity,omitempty"` // none (default), trivial, minor, major, critical
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
			Provider: "agent",
		},
		Review: ReviewConfig{
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
		{Symbol: "!", Name: "Security Risk", Description: "Vulnerabilities: SQL injection, XSS, exposed secrets, etc.", Enabled: true, Category: "reliability", Severity: "critical"},
		{Symbol: "~", Name: "Performance Drag", Description: "Latency, slow execution, performance bottlenecks.", Enabled: true, Category: "reliability", Severity: "major"},
		{Symbol: "$", Name: "Resource Cost", Description: "Expensive operations, memory leaks, compute waste.", Enabled: false, Category: "reliability", Severity: "major"},
		// Form
		{Symbol: "&", Name: "Coupling Violation", Description: "Tight coupling, hardcoded dependencies.", Enabled: true, Category: "form", Severity: "minor"},
		{Symbol: "#", Name: "Cohesion / SOLID Issue", Description: "Low cohesion, single responsibility violations.", Enabled: true, Category: "form", Severity: "minor"},
		{Symbol: "=", Name: "DRY Violation", Description: "Code duplication, repeated logic.", Enabled: true, Category: "form", Severity: "trivial"},
		{Symbol: "?", Name: "KISS Violation", Description: "Overly clever, deeply nested, hard to read code.", Enabled: true, Category: "form", Severity: "trivial"},
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

// LoadResult carries the loaded config and a diagnostic about where it came from.
type LoadResult struct {
	Config     *Config
	ConfigPath string // empty if defaults are being used
}

// Load reads config from the nearest .gh-intent-review.yml, falling back to defaults.
// Pass an explicit path to skip the search (e.g. from a --config flag).
func Load(explicitPath string) (*LoadResult, error) {
	cfg := DefaultConfig()
	result := &LoadResult{Config: cfg}

	var searchPaths []string
	if explicitPath != "" {
		searchPaths = []string{explicitPath}
	} else {
		searchPaths = []string{filepath.Join(".", configFileName)}
		if home, err := os.UserHomeDir(); err == nil {
			searchPaths = append(searchPaths, filepath.Join(home, configFileName))
		}
	}

	for _, p := range searchPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			if explicitPath != "" {
				return nil, fmt.Errorf("reading config %s: %w", p, err)
			}
			continue
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		result.ConfigPath = p
		break
	}

	return result, nil
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
