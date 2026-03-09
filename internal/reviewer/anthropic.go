package reviewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// AnthropicProvider implements LLMProvider using the Anthropic API.
type AnthropicProvider struct {
	apiKey string
	model  string
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(cfg config.LLMConfig) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required (set via env var or config)")
	}
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &AnthropicProvider{apiKey: cfg.APIKey, model: model}, nil
}

// ReviewFile sends the file diff to Claude and returns structured intents.
func (p *AnthropicProvider) ReviewFile(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	prompt := buildReviewPrompt(fileDiff, symbols)

	resp, err := p.callAPI(prompt)
	if err != nil {
		return nil, err
	}

	return parseLLMResponse(resp, fileDiff, symbols)
}

func buildReviewPrompt(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) string {
	var b strings.Builder

	b.WriteString("You are an expert code reviewer. Analyze the diff below and identify issues.\n\n")
	b.WriteString("Available intent symbols:\n")

	for _, s := range symbols {
		b.WriteString(fmt.Sprintf("  ¿%s — %s: %s\n", s.Symbol, s.Name, s.Description))
	}

	b.WriteString(`
For each issue, return a JSON array of objects:
- "symbol": the intent symbol character (e.g. "!", "~", "#")
- "lines": array of the exact code lines being flagged (without +/- prefix)
- "start_line": the line number in the new file where the flagged range starts
- "comment": concise explanation of the issue

RULES:
- Only flag genuine issues — no trivial style nitpicks
- An intent can span multiple consecutive lines (e.g. a for-loop with its body)
- Multiple intents can flag the same line
- If no issues found, return: []
- Respond ONLY with valid JSON, no markdown

`)

	b.WriteString("File: " + fileDiff.NewName + "\n\n")
	b.WriteString(fileDiff.String())

	return b.String()
}

func (p *AnthropicProvider) callAPI(prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model":      p.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing API response: %w", err)
	}

	if len(result.Content) == 0 {
		return "[]", nil
	}

	return result.Content[0].Text, nil
}
