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
// Unlike API-based providers that see only the diff text, the agent has access to
// the full codebase at $PWD via its own tools (file reads, search, etc.), enabling
// deeper cross-file analysis.
//
// --append-system-prompt is auto-generated from the enabled intent symbols in config.
// The user prompt includes the diff and instructs the agent to use $PWD for context.
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

// ReviewFile runs the agent CLI to review a file diff and returns structured intents.
// The agent can traverse the workspace at $PWD for additional context.
func (p *AgentProvider) ReviewFile(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	prompt := buildAgentPrompt(fileDiff, symbols)
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
		return parseLLMResponse(string(out), fileDiff, symbols)
	}
	if envelope.IsError {
		return nil, fmt.Errorf("agent returned error: %s", envelope.Result)
	}

	return parseLLMResponse(envelope.Result, fileDiff, symbols)
}

// buildAgentSystemPrompt constructs the --append-system-prompt content from the
// enabled intent symbols. This tells the agent what categories of issues to look for.
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
  {"symbol": "!", "lines": ["exact code line"], "start_line": 12, "comment": "concise explanation"}

Rules:
- symbol must be one of the listed intent symbols
- lines contains exact code lines being flagged (no +/- prefix)
- An intent may span multiple consecutive lines
- Multiple intents may reference the same line
- If no issues found, return: []
- No markdown, no prose — raw JSON only
`)

	return b.String()
}

// buildAgentPrompt builds the user-facing prompt for the agent, including the diff
// and an instruction to use the workspace at $PWD for deeper context.
func buildAgentPrompt(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) string {
	var b strings.Builder

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "$PWD"
	}

	b.WriteString(fmt.Sprintf("Review the following diff for `%s`.\n\n", fileDiff.NewName))
	b.WriteString(fmt.Sprintf(
		"The full codebase is available at `%s` — use your tools to read related files, "+
			"trace call sites, or search for patterns when assessing impact.\n\n",
		cwd,
	))
	b.WriteString("Diff:\n```diff\n")
	b.WriteString(fileDiff.String())
	b.WriteString("```\n\n")
	b.WriteString("Return a JSON array of intents found (see system prompt for schema). ")
	b.WriteString("Return [] if no issues are found.")

	return b.String()
}
