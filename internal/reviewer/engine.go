package reviewer

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// Engine orchestrates agentic review of all file diffs in a single session.
type Engine struct {
	cfg      *config.Config
	provider LLMProvider
}

// LLMProvider is the interface any AI backend must implement.
type LLMProvider interface {
	// ReviewAll reviews all file diffs in one agent session and returns all intents found.
	ReviewAll(fileDiffs []diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error)
}

// NewEngine creates a new review engine with the configured LLM provider.
func NewEngine(cfg *config.Config) (*Engine, error) {
	provider, err := NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Engine{cfg: cfg, provider: provider}, nil
}

// Review runs a single-session review across all file diffs and returns a FocusedDiff.
func (e *Engine) Review(fileDiffs []diff.FileDiff) (*diff.FocusedDiff, error) {
	symbols := e.cfg.EnabledSymbols()

	// Build raw diff string from all files.
	var rawDiffBuilder strings.Builder
	for _, fd := range fileDiffs {
		rawDiffBuilder.WriteString(fd.String())
	}

	// Filter binary and ignored files.
	var filtered []diff.FileDiff
	for _, fd := range fileDiffs {
		if !fd.IsBinary && !e.shouldIgnore(fd.NewName) {
			filtered = append(filtered, fd)
		}
	}
	if len(filtered) == 0 {
		return &diff.FocusedDiff{RawDiff: rawDiffBuilder.String()}, nil
	}

	fmt.Printf("Reviewing %d file(s) in a single agent session:\n", len(filtered))
	for _, fd := range filtered {
		fmt.Printf("  %s\n", fd.NewName)
	}

	intents, err := e.provider.ReviewAll(filtered, symbols)
	if err != nil {
		return nil, fmt.Errorf("agent review failed: %w", err)
	}

	// Group intents by file, preserving order.
	fileIntents := make(map[string]*diff.FocusedFile)
	var fileOrder []string
	for _, intent := range intents {
		path := intent.FilePath
		if _, ok := fileIntents[path]; !ok {
			fileIntents[path] = &diff.FocusedFile{Path: path}
			fileOrder = append(fileOrder, path)
		}
		fileIntents[path].Intents = append(fileIntents[path].Intents, intent)
	}

	var files []diff.FocusedFile
	for _, path := range fileOrder {
		files = append(files, *fileIntents[path])
	}

	return &diff.FocusedDiff{
		RawDiff: rawDiffBuilder.String(),
		Files:   files,
	}, nil
}

// shouldIgnore checks if a file matches any ignore patterns.
func (e *Engine) shouldIgnore(path string) bool {
	for _, pattern := range e.cfg.Review.IgnoreFiles {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}
