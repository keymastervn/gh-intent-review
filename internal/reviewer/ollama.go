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

const ollamaDefaultURL = "http://localhost:11434/api/generate"

// OllamaProvider implements LLMProvider using a local Ollama instance.
type OllamaProvider struct {
	model   string
	baseURL string
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(cfg config.LLMConfig) (*OllamaProvider, error) {
	model := cfg.Model
	if model == "" {
		model = "llama3"
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = ollamaDefaultURL
	}
	return &OllamaProvider{model: model, baseURL: baseURL}, nil
}

// ReviewFile sends the file diff to Ollama and returns structured intents.
func (p *OllamaProvider) ReviewFile(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error) {
	prompt := buildReviewPrompt(fileDiff, symbols)

	reqBody := map[string]interface{}{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
		"format": "json",
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama request failed (is Ollama running?): %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing Ollama response: %w", err)
	}

	return parseLLMResponse(result.Response, fileDiff, symbols)
}
