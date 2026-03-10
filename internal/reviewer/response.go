package reviewer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// llmIntent is the JSON structure the agent returns for each flagged issue.
type llmIntent struct {
	Symbol    string   `json:"symbol"`
	File      string   `json:"file"`
	Lines     []string `json:"lines"`
	StartLine int      `json:"start_line"`
	Comment   string   `json:"comment"`
}

// parseLLMResponse converts the agent's JSON response into diff.Intent blocks.
// fileDiffs is used to validate file paths returned by the agent.
func parseLLMResponse(response string, fileDiffs []diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Agents sometimes output code containing regex/escape sequences like \s, \d, \w
	// directly in JSON strings. These are invalid JSON escapes and must be fixed before parsing.
	response = sanitizeJSONEscapes(response)

	var raw []llmIntent
	if err := json.Unmarshal([]byte(response), &raw); err != nil {
		return nil, fmt.Errorf("parsing agent response: %w\nRaw: %s", err, response)
	}

	symbolSet := make(map[string]bool)
	for _, s := range symbols {
		symbolSet[s.Symbol] = true
	}

	// Build a set of known file paths for validation.
	knownFiles := make(map[string]bool)
	for _, fd := range fileDiffs {
		knownFiles[fd.NewName] = true
	}

	var intents []diff.Intent
	for _, r := range raw {
		if !symbolSet[r.Symbol] {
			continue
		}
		filePath := r.File
		// Fall back to first file if agent omitted the file field.
		if filePath == "" && len(fileDiffs) > 0 {
			filePath = fileDiffs[0].NewName
		}
		lineCount := len(r.Lines)
		if lineCount == 0 {
			lineCount = 1
		}
		intents = append(intents, diff.Intent{
			Symbol:        r.Symbol,
			Name:          diff.SymbolToName(r.Symbol),
			FilePath:      filePath,
			HunkHeader:    fmt.Sprintf("@@ +%d,%d @@", r.StartLine, lineCount),
			AffectedLines: r.Lines,
			Explanation:   r.Comment,
			Severity:      diff.SymbolToSeverity(r.Symbol),
		})
	}
	return intents, nil
}

// sanitizeJSONEscapes fixes invalid escape sequences in JSON string values.
//
// Agents output code verbatim (e.g. regex like /^\s+$/, paths like C:\Users\) which
// contain bare backslash sequences that are not valid JSON escapes. JSON only allows:
//
//	\" \\ \/ \b \f \n \r \t \uXXXX
//
// Any other \X is invalid. This function walks the JSON byte-by-byte, tracking
// whether it is inside a string, and doubles any backslash that introduces an
// invalid escape — turning \s into \\s, \. into \\., etc.
func sanitizeJSONEscapes(raw string) string {
	validEscapeByte := func(c byte) bool {
		return c == '"' || c == '\\' || c == '/' ||
			c == 'b' || c == 'f' || c == 'n' || c == 'r' || c == 't' || c == 'u'
	}

	buf := make([]byte, 0, len(raw))
	inStr := false
	i := 0
	for i < len(raw) {
		b := raw[i]
		if inStr {
			if b == '\\' && i+1 < len(raw) {
				next := raw[i+1]
				if !validEscapeByte(next) {
					// Invalid escape: double the backslash so \s becomes \\s.
					buf = append(buf, '\\', '\\', next)
					i += 2
					continue
				}
				// Valid escape: copy both bytes and skip past the escaped char.
				buf = append(buf, b, next)
				i += 2
				continue
			} else if b == '"' {
				inStr = false
			}
		} else if b == '"' {
			inStr = true
		}
		buf = append(buf, b)
		i++
	}
	return string(buf)
}
