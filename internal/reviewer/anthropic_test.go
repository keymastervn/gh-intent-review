package reviewer

import (
	"strings"
	"testing"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

func TestBuildReviewPrompt(t *testing.T) {
	fileDiff := &diff.FileDiff{
		OldName: "handler.go",
		NewName: "handler.go",
		Hunks: []diff.Hunk{
			{
				Header: "@@ -1,3 +1,5 @@",
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, Content: "package handler"},
					{Type: diff.LineAdded, Content: `db.Exec("SELECT * FROM users WHERE id = " + id)`},
					{Type: diff.LineContext, Content: "}"},
				},
			},
		},
	}

	symbols := []config.IntentSymbol{
		{Symbol: "!", Name: "Security Risk", Description: "SQL injection, XSS, etc."},
		{Symbol: "~", Name: "Performance Drag", Description: "Slow execution"},
	}

	prompt := buildReviewPrompt(fileDiff, symbols)

	if len(prompt) == 0 {
		t.Fatal("expected non-empty prompt")
	}

	expected := []string{
		"¿!",               // symbol reference
		"¿~",               // symbol reference
		"Security Risk",    // symbol name
		"Performance Drag", // symbol name
		"handler.go",       // file content in diff
		"JSON",             // asks for JSON response
		"start_line",       // JSON field
	}
	for _, s := range expected {
		if !strings.Contains(prompt, s) {
			t.Errorf("expected prompt to contain %q", s)
		}
	}
}

func TestParseLLMResponse_Valid(t *testing.T) {
	response := `[
		{
			"symbol": "!",
			"lines": ["db.Exec(query + id)"],
			"start_line": 5,
			"comment": "SQL injection vulnerability"
		}
	]`

	fileDiff := &diff.FileDiff{NewName: "handler.go"}
	symbols := []config.IntentSymbol{
		{Symbol: "!", Name: "Security Risk"},
		{Symbol: "~", Name: "Performance Drag"},
	}

	intents, err := parseLLMResponse(response, fileDiff, symbols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}
	if intents[0].Symbol != "!" {
		t.Errorf("expected symbol '!', got %q", intents[0].Symbol)
	}
	if intents[0].FilePath != "handler.go" {
		t.Errorf("expected file path 'handler.go', got %q", intents[0].FilePath)
	}
	if intents[0].HunkHeader != "@@ +5,1 @@" {
		t.Errorf("expected hunk '@@ +5,1 @@', got %q", intents[0].HunkHeader)
	}
	if len(intents[0].AffectedLines) != 1 {
		t.Errorf("expected 1 affected line, got %d", len(intents[0].AffectedLines))
	}
}

func TestParseLLMResponse_MultiLine(t *testing.T) {
	response := `[
		{
			"symbol": "~",
			"lines": ["for (let i = 0; i < arr.length; i++) {", "  await fetch(arr[i]);", "}"],
			"start_line": 10,
			"comment": "N+1 query in loop"
		}
	]`

	fileDiff := &diff.FileDiff{NewName: "app.js"}
	symbols := []config.IntentSymbol{{Symbol: "~"}}

	intents, err := parseLLMResponse(response, fileDiff, symbols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}
	if len(intents[0].AffectedLines) != 3 {
		t.Errorf("expected 3 affected lines, got %d", len(intents[0].AffectedLines))
	}
	if intents[0].HunkHeader != "@@ +10,3 @@" {
		t.Errorf("expected hunk '@@ +10,3 @@', got %q", intents[0].HunkHeader)
	}
}

func TestParseLLMResponse_Empty(t *testing.T) {
	fileDiff := &diff.FileDiff{NewName: "clean.go"}
	symbols := []config.IntentSymbol{{Symbol: "!"}}

	intents, err := parseLLMResponse("[]", fileDiff, symbols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents, got %d", len(intents))
	}
}

func TestParseLLMResponse_WithCodeFences(t *testing.T) {
	response := "```json\n[{\"symbol\": \"~\", \"lines\": [\"x\"], \"start_line\": 1, \"comment\": \"slow\"}]\n```"

	fileDiff := &diff.FileDiff{NewName: "f.go"}
	symbols := []config.IntentSymbol{{Symbol: "~"}}
	intents, err := parseLLMResponse(response, fileDiff, symbols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Errorf("expected 1 intent, got %d", len(intents))
	}
}

func TestParseLLMResponse_FiltersInvalidSymbols(t *testing.T) {
	response := `[
		{"symbol": "!", "lines": ["x"], "start_line": 1, "comment": "valid"},
		{"symbol": "X", "lines": ["y"], "start_line": 2, "comment": "invalid symbol"}
	]`

	fileDiff := &diff.FileDiff{NewName: "f.go"}
	symbols := []config.IntentSymbol{{Symbol: "!"}}
	intents, err := parseLLMResponse(response, fileDiff, symbols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Errorf("expected 1 intent (filtered invalid symbol), got %d", len(intents))
	}
}

func TestParseLLMResponse_InvalidJSON(t *testing.T) {
	fileDiff := &diff.FileDiff{NewName: "f.go"}
	symbols := []config.IntentSymbol{{Symbol: "!"}}
	_, err := parseLLMResponse("not json", fileDiff, symbols)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
