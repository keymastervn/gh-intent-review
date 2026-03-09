package reviewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// OpenAIProvider implements LLMProvider using the OpenAI API.
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(cfg config.LLMConfig) (*OpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required (set via env var or config)")
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = openaiAPIURL
	}
	return &OpenAIProvider{apiKey: cfg.APIKey, model: model, baseURL: baseURL}, nil
}

// ReviewFile sends the file diff to OpenAI and returns structured intents.
func (p *OpenAIProvider) ReviewFile(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	prompt := buildReviewPrompt(fileDiff, symbols)

	reqBody := map[string]interface{}{
		"model":      p.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing API response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, nil
	}

	return parseLLMResponse(result.Choices[0].Message.Content, fileDiff, symbols)
}
