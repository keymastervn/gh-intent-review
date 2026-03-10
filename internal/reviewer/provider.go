package reviewer

import (
	"fmt"

	"github.com/keymastervn/gh-intent-review/internal/config"
)

// NewProvider creates an LLMProvider based on the config.
func NewProvider(cfg *config.Config) (LLMProvider, error) {
	switch cfg.LLM.Provider {
	case "agent", "":
		return NewAgentProvider(cfg.LLM)
	case "custom":
		return NewCustomAgentProvider(cfg.LLM)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q (supported: \"agent\", \"custom\")", cfg.LLM.Provider)
	}
}
