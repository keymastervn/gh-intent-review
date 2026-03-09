package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// ElaborateIntent calls the configured LLM/agent to provide a deeper explanation
// of an intent issue, guided by the user's prompt.
func ElaborateIntent(cfg *config.Config, intent diff.Intent, userPrompt string) (string, error) {
	fullPrompt := buildElaborationPrompt(intent, userPrompt)

	switch cfg.LLM.Provider {
	case "agent":
		return elaborateViaAgent(cfg.LLM, fullPrompt)
	case "anthropic":
		return elaborateViaAnthropic(cfg.LLM, fullPrompt)
	case "openai":
		return elaborateViaOpenAI(cfg.LLM, fullPrompt)
	default:
		return "", fmt.Errorf("elaboration not supported for provider %q (use: agent, anthropic, openai)", cfg.LLM.Provider)
	}
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

func elaborateViaAnthropic(cfg config.LLMConfig, prompt string) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	reqBody, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing Anthropic response: %w", err)
	}
	if result.Error.Message != "" {
		return "", fmt.Errorf("Anthropic error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from Anthropic")
	}
	return strings.TrimSpace(result.Content[0].Text), nil
}

func elaborateViaOpenAI(cfg config.LLMConfig, prompt string) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is required")
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	reqBody, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing OpenAI response: %w", err)
	}
	if result.Error.Message != "" {
		return "", fmt.Errorf("OpenAI error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from OpenAI")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
