package reviewer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// llmIntent is the JSON structure the LLM returns for each flagged issue.
type llmIntent struct {
	Symbol    string   `json:"symbol"`
	Lines     []string `json:"lines"`
	StartLine int      `json:"start_line"`
	Comment   string   `json:"comment"`
}

// parseLLMResponse converts the LLM's JSON response into diff.Intent blocks.
func parseLLMResponse(response string, fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var raw []llmIntent
	if err := json.Unmarshal([]byte(response), &raw); err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w\nRaw: %s", err, response)
	}

	symbolSet := make(map[string]bool)
	for _, s := range symbols {
		symbolSet[s.Symbol] = true
	}

	var intents []diff.Intent
	for _, r := range raw {
		if !symbolSet[r.Symbol] {
			continue
		}
		lineCount := len(r.Lines)
		if lineCount == 0 {
			lineCount = 1
		}
		intents = append(intents, diff.Intent{
			Symbol:        r.Symbol,
			Name:          diff.SymbolToName(r.Symbol),
			FilePath:      fileDiff.NewName,
			HunkHeader:    fmt.Sprintf("@@ +%d,%d @@", r.StartLine, lineCount),
			AffectedLines: r.Lines,
			Explanation:   r.Comment,
			Severity:      diff.SymbolToSeverity(r.Symbol),
		})
	}
	return intents, nil
}
