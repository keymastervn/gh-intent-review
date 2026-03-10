package reviewer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// AgentProvider implements LLMProvider by invoking a locally installed CLI agent
// (e.g. Claude Code: `claude -p <prompt> --output-format json`).
//
// All file diffs are passed in a single agent session, allowing cross-file analysis.
// The agent has access to the full codebase at $PWD via its own tools.
type AgentProvider struct {
	command string
	model   string
}

// NewAgentProvider creates a new agent-based provider.
func NewAgentProvider(cfg config.LLMConfig) (*AgentProvider, error) {
	command := cfg.AgentCommand
	if command == "" {
		command = "claude"
	}
	if _, err := exec.LookPath(command); err != nil {
		return nil, fmt.Errorf("agent command %q not found in PATH: %w", command, err)
	}
	return &AgentProvider{
		command: command,
		model:   cfg.Model,
	}, nil
}

// ReviewAll runs the agent CLI to review all file diffs in a single session.
func (p *AgentProvider) ReviewAll(fileDiffs []diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	prompt := buildAgentPrompt(fileDiffs, symbols)
	systemPrompt := buildAgentSystemPrompt(symbols)

	args := []string{"-p", prompt, "--output-format", "json", "--append-system-prompt", systemPrompt}
	if p.model != "" {
		args = append(args, "--model", p.model)
	}

	cmd := exec.Command(p.command, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("agent command failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("agent command failed: %w", err)
	}

	// Claude Code --output-format json wraps the model output in an envelope:
	// {"type":"result","subtype":"success","is_error":false,"result":"...","session_id":"..."}
	var envelope struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		// Not wrapped — treat raw output as the intent JSON directly.
		return parseLLMResponse(string(out), fileDiffs, symbols)
	}
	if envelope.IsError {
		return nil, fmt.Errorf("agent returned error: %s", envelope.Result)
	}

	return parseLLMResponse(envelope.Result, fileDiffs, symbols)
}

// buildAgentSystemPrompt constructs the --append-system-prompt content describing
// what intent categories to look for and the expected JSON output schema.
func buildAgentSystemPrompt(symbols []config.IntentSymbol) string {
	var b strings.Builder

	b.WriteString("You are performing an intent-focused code review. ")
	b.WriteString("Your job is to identify only meaningful issues — not style nitpicks.\n\n")
	b.WriteString("Intent symbols you must use:\n")
	for _, s := range symbols {
		b.WriteString(fmt.Sprintf("  ¿%s  %s: %s\n", s.Symbol, s.Name, s.Description))
	}
	b.WriteString(`
Output ONLY a JSON array. Each element:
  {"symbol": "!", "file": "path/to/file.ext", "lines": ["exact code line"], "start_line": 12, "comment": "concise explanation"}

Rules:
- symbol must be one of the listed intent symbols
- file must be the exact file path as provided in the diff header
- lines contains exact code lines being flagged (no +/- prefix)
- An intent may span multiple consecutive lines
- Multiple intents may reference the same line
- Backslashes in code lines must be JSON-escaped: \s → \\s, \. → \\., \n → \\n, etc.
- If no issues found, return: []
- No markdown, no prose — raw JSON only
`)

	return b.String()
}

// buildAgentPrompt builds the prompt for the agent with all file diffs concatenated.
func buildAgentPrompt(fileDiffs []diff.FileDiff, symbols []config.IntentSymbol) string {
	var b strings.Builder

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "$PWD"
	}

	b.WriteString(fmt.Sprintf("Review the following PR diff (%d file(s)).\n\n", len(fileDiffs)))
	b.WriteString(fmt.Sprintf(
		"The full codebase is available at `%s` — use your tools to read related files, "+
			"trace call sites, or search for patterns when assessing impact.\n\n",
		cwd,
	))
	b.WriteString("Diff:\n```diff\n")
	for _, fd := range fileDiffs {
		b.WriteString(fd.String())
	}
	b.WriteString("```\n\n")
	b.WriteString("Return a JSON array of intents found (see system prompt for schema). ")
	b.WriteString("Return [] if no issues are found.")

	return b.String()
}
