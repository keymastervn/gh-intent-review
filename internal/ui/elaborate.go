package ui

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// ElaborateVerboseHint returns a dim command-line description of what ElaborateIntent will call.
// Printed before the spinner so the user knows what's running.
func ElaborateVerboseHint(cfg *config.Config, userPrompt string) string {
	truncated := userPrompt
	if len(truncated) > 60 {
		truncated = truncated[:57] + "..."
	}
	cmd := cfg.LLM.AgentCommand
	if cmd == "" {
		cmd = "claude"
	}
	model := ""
	if cfg.LLM.Model != "" {
		model = " --model " + cfg.LLM.Model
	}
	return fmt.Sprintf("%s -p %q --output-format json%s", cmd, truncated, model)
}

// ElaborateIntent calls the configured agent to provide a deeper explanation
// of an intent issue, guided by the user's prompt.
func ElaborateIntent(cfg *config.Config, intent diff.Intent, userPrompt string) (string, error) {
	return elaborateViaAgent(cfg.LLM, buildElaborationPrompt(intent, userPrompt))
}

func buildElaborationPrompt(intent diff.Intent, userPrompt string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Code review finding: ¿%s %s\n", intent.Symbol, intent.Name))
	b.WriteString(fmt.Sprintf("File: %s\n", intent.FilePath))
	if intent.HunkHeader != "" {
		b.WriteString(fmt.Sprintf("Location: %s\n", intent.HunkHeader))
	}
	if len(intent.AffectedLines) > 0 {
		b.WriteString("Flagged code:\n```\n")
		for _, line := range intent.AffectedLines {
			b.WriteString(line + "\n")
		}
		b.WriteString("```\n")
	}
	b.WriteString(fmt.Sprintf("AI finding: %s\n\n", intent.Explanation))
	b.WriteString(userPrompt)
	return b.String()
}

func elaborateViaAgent(cfg config.LLMConfig, prompt string) (string, error) {
	command := cfg.AgentCommand
	if command == "" {
		command = "claude"
	}
	args := []string{"-p", prompt, "--output-format", "json"}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	out, err := exec.Command(command, args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("agent failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("agent failed: %w", err)
	}

	// Parse Claude Code JSON envelope: {"type":"result","is_error":false,"result":"..."}
	var envelope struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return strings.TrimSpace(string(out)), nil
	}
	if envelope.IsError {
		return "", fmt.Errorf("agent error: %s", envelope.Result)
	}
	return strings.TrimSpace(envelope.Result), nil
}
