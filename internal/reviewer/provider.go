package reviewer

import (
	"fmt"

	"github.com/keymastervn/gh-intent-review/internal/config"
)

// NewProvider creates an LLMProvider based on the config.
func NewProvider(cfg *config.Config) (LLMProvider, error) {
	switch cfg.LLM.Provider {
	case "anthropic":
		return NewAnthropicProvider(cfg.LLM)
	case "openai":
		return NewOpenAIProvider(cfg.LLM)
	case "ollama":
		return NewOllamaProvider(cfg.LLM)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s (supported: anthropic, openai, ollama)", cfg.LLM.Provider)
	}
}
