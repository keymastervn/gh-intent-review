package diff

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FocusedDiff is the top-level container for an intentional diff.
// Format:
//
//	Section 1: The original git diff (unchanged)
//	Section 2: Intent blocks, each starting with ¿XX b/file.txt
//
// Example:
//
//	diff --git a/handler.js b/handler.js
//	--- a/handler.js
//	+++ b/handler.js
//	@@ -10,6 +10,8 @@
//	 const express = require('express');
//	+  const q = "SELECT *" + id;
//	+  const users = await db.execute(q);
//	 }
//
//	¿!! b/handler.js
//	@@ +10,1 @@
//	+  const q = "SELECT *" + id;
//	-{¿! SQL injection — use parameterized queries ¿!}
//
//	¿~~ b/handler.js
//	@@ +11,2 @@
//	+  for (let i = 0; i < users.length; i++) {
//	+    const profile = await fetchProfile(users[i].id);
//	-{¿~ N+1 query — use Promise.all() for parallel fetching ¿~}
type FocusedDiff struct {
	PROwner  string
	PRRepo   string
	PRNumber int
	RawDiff  string        // the original git diff, untouched
	Files    []FocusedFile // files that have intents
}

// FocusedFile groups intents by file path.
type FocusedFile struct {
	Path    string
	Intents []Intent
}

// Intent represents a single review intent block.
//
// Rendered as:
//
//	¿!! b/file.txt
//	@@ +10,2 @@
//	+  flagged line 1
//	+  flagged line 2
//	-{¿! explanation here ¿!}
//	 optional context line
type Intent struct {
	Symbol        string   // e.g. "!", "~", "&"
	Name          string   // e.g. "Security Risk"
	FilePath      string   // b/file.txt
	HunkHeader    string   // @@ +10,2 @@ (affected line range)
	AffectedLines []string // the + lines (code being flagged)
	ContextLines  []string // surrounding context (space-prefixed)
	Explanation   string   // the intent comment text
	Severity      string   // "critical", "warning", "info"

	// Review state (populated during interactive review)
	Status        IntentStatus
	ReviewComment string // reviewer's comment when disapproved
}

// IntentStatus tracks the review state of an intent.
type IntentStatus string

const (
	IntentPending     IntentStatus = ""
	IntentApproved    IntentStatus = "approved"
	IntentDisapproved IntentStatus = "disapproved"
	IntentSkipped     IntentStatus = "skipped"
	IntentCommented   IntentStatus = "commented"
)

// intentCommentRegex matches -{¿X ...¿X} on a comment line.
var intentCommentRegex = regexp.MustCompile(`^-\{¿([!~$&#=?])\s+(.*?)\s*¿[!~$&#=?]\}$`)

// intentBlockHeaderRegex matches ¿XX b/path lines (doubled symbol).
// Go's RE2 doesn't support backreferences, so we list each doubled pair.
var intentBlockHeaderRegex = regexp.MustCompile(`^¿(!!|~~|\$\$|&&|##|==|\?\?)\s+b/(.+)$`)

// TotalIntents returns the total number of intents across all files.
func (fd *FocusedDiff) TotalIntents() int {
	total := 0
	for _, f := range fd.Files {
		total += len(f.Intents)
	}
	return total
}

// ShortSHA returns the first 7 characters of a git SHA.
func ShortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// DefaultOutputPath returns the default storage path for an intentional diff.
// Without a config, diffs are stored at ~/.gh-intent-review/owner/repo/<pr>.intentional.diff
func DefaultOutputPath(owner, repo string, number int) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".gh-intent-review", owner, repo, fmt.Sprintf("%d.intentional.diff", number))
}

// ProjectOutputPath returns a project-local storage path (used when output.dir is configured).
func ProjectOutputPath(dir, owner, repo string, number int) string {
	return filepath.Join(dir, owner, repo, fmt.Sprintf("%d.intentional.diff", number))
}

// DefaultOutputPathWithSHA returns the default storage path including the head commit SHA.
// Diffs are stored at ~/.gh-intent-review/owner/repo/<pr>-<short-sha>.intentional.diff
func DefaultOutputPathWithSHA(owner, repo string, number int, sha string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".gh-intent-review", owner, repo, fmt.Sprintf("%d-%s.intentional.diff", number, ShortSHA(sha)))
}

// ProjectOutputPathWithSHA returns a project-local storage path including the head commit SHA.
func ProjectOutputPathWithSHA(dir, owner, repo string, number int, sha string) string {
	return filepath.Join(dir, owner, repo, fmt.Sprintf("%d-%s.intentional.diff", number, ShortSHA(sha)))
}

// FindDiffPath locates an existing intentional diff for the given PR.
// outputDir is the configured output.dir value (empty = use ~/.gh-intent-review).
// If sha is non-empty, looks for an exact SHA-matched file (e.g. 1-cb3513f.intentional.diff).
// If sha is empty, returns the most recently modified file matching <number>-*.intentional.diff.
// Returns ("", false) if no matching file is found.
func FindDiffPath(outputDir, owner, repo string, number int, sha string) (string, bool) {
	var dir string
	if outputDir != "" {
		dir = filepath.Join(outputDir, owner, repo)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		dir = filepath.Join(home, ".gh-intent-review", owner, repo)
	}

	if sha != "" {
		path := filepath.Join(dir, fmt.Sprintf("%d-%s.intentional.diff", number, ShortSHA(sha)))
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		return "", false
	}

	// No SHA provided: return the most recently modified matching file.
	pattern := filepath.Join(dir, fmt.Sprintf("%d-*.intentional.diff", number))
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", false
	}
	var best string
	var bestTime time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			best = m
		}
	}
	return best, best != ""
}

// WriteFocusedDiff writes an intentional diff to disk.
func WriteFocusedDiff(path string, fd *FocusedDiff) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(RenderFocusedDiff(fd)), 0644)
}

// ReadFocusedDiff reads and parses an intentional diff from disk.
func ReadFocusedDiff(path string) (*FocusedDiff, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseIntentionalDiff(string(data))
}

// RenderFocusedDiff renders the full intentional diff: raw diff + intent blocks.
func RenderFocusedDiff(fd *FocusedDiff) string {
	var b strings.Builder

	// Section 1: original diff
	if fd.RawDiff != "" {
		b.WriteString(fd.RawDiff)
		if !strings.HasSuffix(fd.RawDiff, "\n") {
			b.WriteString("\n")
		}
	}

	// Section 2: intent blocks
	for _, file := range fd.Files {
		for _, intent := range file.Intents {
			b.WriteString("\n")
			b.WriteString(RenderIntentBlock(intent))
		}
	}

	return b.String()
}

// RenderIntentBlock renders a single intent block.
func RenderIntentBlock(intent Intent) string {
	var b strings.Builder

	// Header: ¿!! b/file.txt
	b.WriteString(fmt.Sprintf("¿%s%s b/%s\n", intent.Symbol, intent.Symbol, intent.FilePath))

	// Hunk header
	if intent.HunkHeader != "" {
		b.WriteString(intent.HunkHeader + "\n")
	}

	// Affected lines (+ prefix)
	for _, line := range intent.AffectedLines {
		b.WriteString("+" + line + "\n")
	}

	// Comment line (- prefix with intent marker)
	b.WriteString(fmt.Sprintf("-{¿%s %s ¿%s}\n", intent.Symbol, intent.Explanation, intent.Symbol))

	// Context lines (space prefix)
	for _, line := range intent.ContextLines {
		b.WriteString(" " + line + "\n")
	}

	return b.String()
}

// ParseIntentionalDiff parses an intentional diff text into structured form.
// It handles both the raw diff section and the intent block section.
func ParseIntentionalDiff(text string) (*FocusedDiff, error) {
	fd := &FocusedDiff{}

	// Split into lines
	scanner := bufio.NewScanner(strings.NewReader(text))
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Find where intent blocks start (first ¿XX line)
	intentStart := -1
	for i, line := range allLines {
		if intentBlockHeaderRegex.MatchString(line) {
			intentStart = i
			break
		}
	}

	// Section 1: raw diff (everything before first intent block)
	if intentStart > 0 {
		fd.RawDiff = strings.Join(allLines[:intentStart], "\n") + "\n"
	} else if intentStart == -1 {
		// No intent blocks at all — whole thing is raw diff
		fd.RawDiff = text
		return fd, nil
	}

	// Section 2: parse intent blocks
	fileIntents := make(map[string]*FocusedFile) // keyed by file path
	var fileOrder []string                       // preserve order

	i := intentStart
	for i < len(allLines) {
		line := allLines[i]

		// Match intent block header: ¿!! b/file.txt
		headerMatch := intentBlockHeaderRegex.FindStringSubmatch(line)
		if headerMatch == nil {
			i++
			continue
		}

		symbol := string(headerMatch[1][0]) // "!!" -> "!", "~~" -> "~"
		filePath := headerMatch[2]
		i++

		intent := Intent{
			Symbol:   symbol,
			Name:     SymbolToName(symbol),
			FilePath: filePath,
			Severity: SymbolToSeverity(symbol),
		}

		// Parse the block body
		for i < len(allLines) {
			bodyLine := allLines[i]

			// Stop at next intent block header or empty-then-header
			if intentBlockHeaderRegex.MatchString(bodyLine) {
				break
			}

			// Hunk header
			if strings.HasPrefix(bodyLine, "@@") {
				intent.HunkHeader = bodyLine
				i++
				continue
			}

			// Comment line: -{¿X ... ¿X}
			if commentMatch := intentCommentRegex.FindStringSubmatch(bodyLine); commentMatch != nil {
				intent.Explanation = strings.TrimSpace(commentMatch[2])
				i++
				continue
			}

			// Affected line: +...
			if strings.HasPrefix(bodyLine, "+") {
				intent.AffectedLines = append(intent.AffectedLines, bodyLine[1:])
				i++
				continue
			}

			// Context line: space-prefixed
			if strings.HasPrefix(bodyLine, " ") {
				intent.ContextLines = append(intent.ContextLines, bodyLine[1:])
				i++
				continue
			}

			// Empty line — could be separator between blocks
			if bodyLine == "" {
				i++
				continue
			}

			// Unknown line — skip
			i++
		}

		// Add intent to file group
		if _, ok := fileIntents[filePath]; !ok {
			fileIntents[filePath] = &FocusedFile{Path: filePath}
			fileOrder = append(fileOrder, filePath)
		}
		fileIntents[filePath].Intents = append(fileIntents[filePath].Intents, intent)
	}

	// Build files in order
	for _, path := range fileOrder {
		fd.Files = append(fd.Files, *fileIntents[path])
	}

	return fd, nil
}

// SymbolToName maps intent symbols to their default names.
func SymbolToName(symbol string) string {
	switch symbol {
	case "!":
		return "Security Risk"
	case "~":
		return "Performance Drag"
	case "$":
		return "Resource Cost"
	case "&":
		return "Coupling Violation"
	case "#":
		return "Cohesion / SOLID Issue"
	case "=":
		return "DRY Violation"
	case "?":
		return "KISS Violation"
	default:
		return "Unknown"
	}
}

// SymbolToSeverity maps intent symbols to default severity.
func SymbolToSeverity(symbol string) string {
	switch symbol {
	case "!":
		return "critical"
	case "~", "$":
		return "warning"
	default:
		return "info"
	}
}
