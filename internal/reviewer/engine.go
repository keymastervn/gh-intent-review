package reviewer

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
)

// Engine orchestrates parallel agentic reviews of file diffs.
type Engine struct {
	cfg      *config.Config
	provider LLMProvider
}

// LLMProvider is the interface that any AI backend must implement.
type LLMProvider interface {
	// ReviewFile takes a file diff and enabled symbols, returns intents found.
	ReviewFile(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error)
}

// NewEngine creates a new review engine with the configured LLM provider.
func NewEngine(cfg *config.Config) (*Engine, error) {
	provider, err := NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Engine{cfg: cfg, provider: provider}, nil
}

// Review runs parallel reviews across all file diffs and returns a FocusedDiff.
func (e *Engine) Review(fileDiffs []diff.FileDiff) (*diff.FocusedDiff, error) {
	symbols := e.cfg.EnabledSymbols()
	parallel := e.cfg.Review.Parallel
	if parallel <= 0 {
		parallel = 1
	}

	// Build the raw diff from all file diffs
	var rawDiffBuilder strings.Builder
	for _, fd := range fileDiffs {
		rawDiffBuilder.WriteString(fd.String())
	}

	type result struct {
		intents []diff.Intent
		err     error
	}

	results := make([]result, len(fileDiffs))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup

	for i, fd := range fileDiffs {
		if fd.IsBinary || e.shouldIgnore(fd.NewName) {
			continue
		}

		wg.Add(1)
		go func(idx int, fileDiff diff.FileDiff) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("  Reviewing: %s\n", fileDiff.NewName)

			intents, err := e.provider.ReviewFile(&fileDiff, symbols)
			results[idx] = result{intents: intents, err: err}
		}(i, fd)
	}

	wg.Wait()

	// Group intents by file
	fileIntents := make(map[string]*diff.FocusedFile)
	var fileOrder []string
	var errCount int

	for i, r := range results {
		if r.err != nil {
			fmt.Printf("  Warning: %s\n", r.err)
			errCount++
			continue
		}
		for _, intent := range r.intents {
			path := intent.FilePath
			if path == "" {
				path = fileDiffs[i].NewName
			}
			if _, ok := fileIntents[path]; !ok {
				fileIntents[path] = &diff.FocusedFile{Path: path}
				fileOrder = append(fileOrder, path)
			}
			fileIntents[path].Intents = append(fileIntents[path].Intents, intent)
		}
	}

	if errCount > 0 {
		fmt.Printf("Warning: %d file(s) had review errors\n", errCount)
	}

	var files []diff.FocusedFile
	for _, path := range fileOrder {
		files = append(files, *fileIntents[path])
	}

	focused := &diff.FocusedDiff{
		RawDiff: rawDiffBuilder.String(),
		Files:   files,
	}

	return focused, nil
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
