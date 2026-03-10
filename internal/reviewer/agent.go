package reviewer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
func (p *AgentProvider) ReviewAll(fileDiffs []diff.FileDiff, symbols []config.IntentSymbol, severity, prURL string) ([]diff.Intent, error) {
	prompt := buildAgentPrompt(fileDiffs, symbols, prURL)
	systemPrompt := buildAgentSystemPrompt(symbols, severity)

	args := []string{"-p", prompt, "--output-format", "stream-json", "--verbose", "--append-system-prompt", systemPrompt}
	if p.model != "" {
		args = append(args, "--model", p.model)
	}

	cmd := exec.Command(p.command, args...)
	// Pipe agent stderr (tool use, thinking) live to our stderr so the user sees progress.
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting agent command: %w", err)
	}

	// stream-json emits one JSON object per line. The final result is the event with
	// "type":"result". Collect it; forward all other lines to stderr as activity hints.
	result, err := collectStreamResult(stdout)
	if waitErr := cmd.Wait(); waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return nil, fmt.Errorf("agent command failed (exit %d)", exitErr.ExitCode())
		}
		return nil, fmt.Errorf("agent command failed: %w", waitErr)
	}
	if err != nil {
		return nil, err
	}

	return parseLLMResponse(result, fileDiffs, symbols)
}

// collectStreamResult reads stream-json lines from the agent stdout.
// Each line is a JSON event; the one with "type":"result" contains the final model output.
// Non-result events are forwarded to stderr so the user sees live tool activity.
func collectStreamResult(r io.Reader) (string, error) {
	var resultText string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MiB line buffer for large diffs
	for scanner.Scan() {
		line := scanner.Text()
		var event struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
			// assistant message fields
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Not JSON (e.g. raw text fallback) — treat as result.
			resultText = line
			continue
		}
		switch event.Type {
		case "result":
			if event.IsError {
				return "", fmt.Errorf("agent returned error: %s", event.Result)
			}
			resultText = event.Result
		case "assistant":
			// Forward text content chunks to stderr so the user sees the agent thinking.
			for _, c := range event.Message.Content {
				if c.Type == "text" && c.Text != "" {
					fmt.Fprintf(os.Stderr, "%s", c.Text)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading agent output: %w", err)
	}
	return resultText, nil
}

// severityLevel maps a severity name to a numeric rank for comparison.
// Returns 0 for "" or "none" (no threshold).
func severityLevel(s string) int {
	switch s {
	case "trivial":
		return 1
	case "minor":
		return 2
	case "major":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

// buildAgentSystemPrompt constructs the --append-system-prompt content describing
// what intent categories to look for, the severity threshold, and the expected JSON schema.
//
// severity is the global minimum threshold ("", "none", "trivial", "minor", "major", "critical").
// Symbols whose per-symbol severity is below the global threshold are excluded from the prompt
// entirely — the agent won't be asked to look for them at all.
func buildAgentSystemPrompt(symbols []config.IntentSymbol, severity string) string {
	var b strings.Builder
	threshold := severityLevel(severity)

	// Filter to symbols at or above the global threshold.
	var applicable []config.IntentSymbol
	for _, s := range symbols {
		if threshold == 0 || severityLevel(s.Severity) >= threshold {
			applicable = append(applicable, s)
		}
	}

	b.WriteString("You are performing an intent-focused code review. ")
	b.WriteString("Your job is to identify only meaningful issues — not style nitpicks.\n\n")
	b.WriteString("Intent symbols you must use:\n")
	for _, s := range applicable {
		severityTag := ""
		if s.Severity != "" {
			severityTag = fmt.Sprintf(" [typical severity: %s]", s.Severity)
		}
		b.WriteString(fmt.Sprintf("  ¿%s  %s%s: %s\n", s.Symbol, s.Name, severityTag, s.Description))
	}

	if threshold > 0 {
		b.WriteString(fmt.Sprintf(`
Severity threshold: %s
Before flagging any issue, assess its real-world impact against this scale:
  trivial  — cosmetic or purely theoretical; negligible real-world effect
  minor    — modest quality or maintainability impact; unlikely to cause runtime problems
  major    — meaningful impact on reliability, correctness, or significant performance
  critical — high impact: security vulnerabilities, data loss, or production instability
Even within an enabled symbol category, only report findings whose specific instance
is "%s" or higher in practice. Silently skip anything below this threshold.
`, severity, severity))
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
func buildAgentPrompt(fileDiffs []diff.FileDiff, symbols []config.IntentSymbol, prURL string) string {
	var b strings.Builder

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "$PWD"
	}

	b.WriteString(fmt.Sprintf("Review the following PR diff (%d file(s)).\n\n", len(fileDiffs)))
	if prURL != "" {
		b.WriteString(fmt.Sprintf("Pull Request: %s\n\n", prURL))
	}
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
