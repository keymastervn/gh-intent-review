package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
	"github.com/keymastervn/gh-intent-review/internal/github"
	"golang.org/x/term"
)

// ReviewSession manages an interactive review of a focused diff.
type ReviewSession struct {
	diff   *diff.FocusedDiff
	pr     *github.PullRequest
	cfg    *config.Config
	reader *bufio.Reader
}

// ReviewResult holds the outcome of an interactive review session.
type ReviewResult struct {
	Commented int
	Skipped   int
	Total     int
	Comments  []IntentComment
}

// IntentComment records a comment posted on the pull request for an intent.
type IntentComment struct {
	File   string
	Intent diff.Intent
	Body   string
}

// NewReviewSession creates a new interactive review session.
func NewReviewSession(fd *diff.FocusedDiff, pr *github.PullRequest, cfg *config.Config) *ReviewSession {
	return &ReviewSession{
		diff:   fd,
		pr:     pr,
		cfg:    cfg,
		reader: bufio.NewReader(os.Stdin),
	}
}

// Run starts the interactive review loop.
func (s *ReviewSession) Run() (*ReviewResult, error) {
	result := &ReviewResult{}

	totalIntents := s.diff.TotalIntents()
	if totalIntents == 0 {
		fmt.Println("No intents found in the focused diff. The code looks clean!")
		return result, nil
	}

	fmt.Printf("\n  Intent Review: %s/%s#%d\n", s.pr.Owner, s.pr.Repo, s.pr.Number)
	fmt.Printf("  %d intents to review\n\n", totalIntents)

	current := 0
	for fi, file := range s.diff.Files {
		if len(file.Intents) == 0 {
			continue
		}

		fmt.Printf("─── %s ───\n\n", file.Path)

		for ii, intent := range file.Intents {
			current++
			result.Total++

			renderIntentForReview(current, totalIntents, intent)

			done := false
			for !done {
				fmt.Print("  [e]laborate  [c]omment  [s]kip  [q]uit → ")

				key, err := s.readKey()
				if err != nil {
					return result, err
				}
				fmt.Println()

				switch key {
				case 'e':
					fmt.Print("  Prompt [elaborate this issue]: ")
					prompt, err := s.readLine()
					if err != nil {
						return result, err
					}
					if prompt == "" {
						prompt = "elaborate this issue"
					}

					// Show command being invoked as dim/darkened text
					fmt.Printf("  \033[2m$ %s\033[0m\n", ElaborateVerboseHint(s.cfg, prompt))

					stop := Spinner("Thinking...")
					explanation, elaborateErr := ElaborateIntent(s.cfg, intent, prompt)
					stop()

					if elaborateErr != nil {
						fmt.Printf("  Error: %v\n\n", elaborateErr)
					} else {
						fmt.Printf("\n  ─── Elaboration ───\n%s\n  ───────────────────\n\n", indentText(explanation, "  "))
					}
					// Stay in loop — user can elaborate again or choose another action

				case 'c':
					suggestion := buildDefaultComment(file.Path, intent)
					fmt.Printf("\n  \033[2mWrite a comment (↑ to load AI suggestion):\033[0m\n")
					editor := NewCommentEditor(suggestion)
					userBody, ok := editor.Run()
					if !ok || strings.TrimSpace(userBody) == "" {
						fmt.Println("  Comment cancelled.")
						break
					}
					if err := postPRComment(s.pr, userBody); err != nil {
						fmt.Printf("  Error posting comment: %v\n\n", err)
					} else {
						fmt.Println("  ✓ Comment posted")
						s.diff.Files[fi].Intents[ii].Status = diff.IntentCommented
						s.diff.Files[fi].Intents[ii].ReviewComment = userBody
						result.Commented++
						result.Comments = append(result.Comments, IntentComment{
							File:   file.Path,
							Intent: intent,
							Body:   userBody,
						})
						done = true
					}

				case 's':
					s.diff.Files[fi].Intents[ii].Status = diff.IntentSkipped
					result.Skipped++
					fmt.Println("  → Skipped")
					done = true

				case 'q', 3: // 'q' or Ctrl+C
					fmt.Println("\n  Review session ended early.")
					return result, nil
				}
			}
		}
	}

	return result, nil
}

// readKey reads a single keypress without requiring Enter.
// Uses raw terminal mode when stdin is a TTY; falls back to line-buffered input otherwise.
func (s *ReviewSession) readKey() (byte, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err == nil {
			var b [1]byte
			_, readErr := os.Stdin.Read(b[:])
			term.Restore(fd, oldState)
			return b[0], readErr
		}
	}
	// Fallback for non-TTY (pipes, tests)
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if len(line) > 0 {
		return line[0], nil
	}
	return 0, nil
}

// readLine reads a full line of cooked-mode text input.
func (s *ReviewSession) readLine() (string, error) {
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// buildDefaultComment formats the AI-generated intent as a GitHub PR comment body.
func buildDefaultComment(filePath string, intent diff.Intent) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**¿%s %s** in `%s`\n\n", intent.Symbol, intent.Name, filePath))
	b.WriteString(intent.Explanation)
	if len(intent.AffectedLines) > 0 {
		b.WriteString("\n\n```\n")
		for _, line := range intent.AffectedLines {
			b.WriteString(line + "\n")
		}
		b.WriteString("```")
	}
	return b.String()
}

// postPRComment posts a general comment on the pull request via the gh CLI.
func postPRComment(pr *github.PullRequest, body string) error {
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", pr.Owner, pr.Repo, pr.Number)
	cmd := exec.Command("gh", "pr", "comment", prURL, "--body", body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// indentText prefixes every line of text with the given indent string.
func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func renderIntentForReview(current, total int, intent diff.Intent) {
	severityColor := ""
	switch intent.Severity {
	case "critical":
		severityColor = "\033[31m" // red
	case "warning":
		severityColor = "\033[33m" // yellow
	default:
		severityColor = "\033[36m" // cyan
	}
	reset := "\033[0m"

	fmt.Printf("  [%d/%d] %s¿%s %s%s\n", current, total, severityColor, intent.Symbol, intent.Name, reset)

	if len(intent.AffectedLines) > 0 {
		for _, line := range intent.AffectedLines {
			fmt.Printf("  \033[32m+%s\033[0m\n", line)
		}
	}

	fmt.Printf("  %s\n\n", intent.Explanation)
}

// PrintSummary outputs the review session summary.
func (r *ReviewResult) PrintSummary() {
	fmt.Println("\n═══ Review Summary ═══")
	fmt.Printf("  Total:     %d\n", r.Total)
	fmt.Printf("  Commented: %d\n", r.Commented)
	fmt.Printf("  Skipped:   %d\n", r.Skipped)

	if len(r.Comments) > 0 {
		fmt.Println("\n  Posted comments:")
		for _, c := range r.Comments {
			fmt.Printf("    ¿%s %s — %s\n", c.Intent.Symbol, c.File, c.Intent.Explanation)
		}
	}
}
