package reviewer

import (
	"fmt"

	"github.com/keymastervn/gh-intent-review/internal/config"
)

// NewProvider creates an LLMProvider based on the config.
// Only the "agent" provider is supported.
func NewProvider(cfg *config.Config) (LLMProvider, error) {
	switch cfg.LLM.Provider {
	case "agent", "":
		return NewAgentProvider(cfg.LLM)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q (only \"agent\" is supported)", cfg.LLM.Provider)
	}
}
