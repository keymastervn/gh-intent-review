package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/keymastervn/gh-intent-review/internal/diff"
	"github.com/keymastervn/gh-intent-review/internal/github"
)

// ReviewSession manages an interactive review of a focused diff.
type ReviewSession struct {
	diff *diff.FocusedDiff
	pr   *github.PullRequest
}

// ReviewResult holds the outcome of an interactive review session.
type ReviewResult struct {
	Approved    int
	Disapproved int
	Skipped     int
	Total       int
	Feedback    []IntentFeedback
}

// IntentFeedback captures reviewer feedback for a disapproved intent.
type IntentFeedback struct {
	File    string
	Intent  diff.Intent
	Comment string
}

// NewReviewSession creates a new interactive review session.
func NewReviewSession(fd *diff.FocusedDiff, pr *github.PullRequest) *ReviewSession {
	return &ReviewSession{diff: fd, pr: pr}
}

// Run starts the interactive review loop.
func (s *ReviewSession) Run() (*ReviewResult, error) {
	reader := bufio.NewReader(os.Stdin)
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

			// Render the intent
			renderIntentForReview(current, totalIntents, intent)

			// Prompt for action
			for {
				fmt.Print("  [a]pprove  [d]isapprove  [s]kip  [q]uit → ")
				input, err := reader.ReadString('\n')
				if err != nil {
					return result, err
				}

				input = strings.TrimSpace(strings.ToLower(input))

				switch input {
				case "a", "approve":
					s.diff.Files[fi].Intents[ii].Status = diff.IntentApproved
					result.Approved++
					fmt.Println("  ✓ Approved")
					goto next

				case "d", "disapprove":
					fmt.Print("  Comment (why disapprove): ")
					comment, err := reader.ReadString('\n')
					if err != nil {
						return result, err
					}
					comment = strings.TrimSpace(comment)

					s.diff.Files[fi].Intents[ii].Status = diff.IntentDisapproved
					s.diff.Files[fi].Intents[ii].ReviewComment = comment
					result.Disapproved++
					result.Feedback = append(result.Feedback, IntentFeedback{
						File:    file.Path,
						Intent:  intent,
						Comment: comment,
					})
					fmt.Println("  ✗ Disapproved")
					goto next

				case "s", "skip":
					s.diff.Files[fi].Intents[ii].Status = diff.IntentSkipped
					result.Skipped++
					fmt.Println("  → Skipped")
					goto next

				case "q", "quit":
					fmt.Println("\n  Review session ended early.")
					return result, nil

				default:
					fmt.Println("  Invalid input. Use: a, d, s, or q")
				}
			}
		next:
		}
	}

	return result, nil
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

	// Show affected lines
	if len(intent.AffectedLines) > 0 {
		for _, line := range intent.AffectedLines {
			fmt.Printf("  \033[32m+%s\033[0m\n", line)
		}
	}

	// Show the explanation
	fmt.Printf("  %s\n\n", intent.Explanation)
}

// PrintSummary outputs the review session summary.
func (r *ReviewResult) PrintSummary() {
	fmt.Println("\n═══ Review Summary ═══")
	fmt.Printf("  Total:       %d\n", r.Total)
	fmt.Printf("  Approved:    %d\n", r.Approved)
	fmt.Printf("  Disapproved: %d\n", r.Disapproved)
	fmt.Printf("  Skipped:     %d\n", r.Skipped)

	if len(r.Feedback) > 0 {
		fmt.Println("\n  Disapproved intents:")
		for _, fb := range r.Feedback {
			fmt.Printf("    ¿%s %s — %s\n", fb.Intent.Symbol, fb.Intent.FilePath, fb.Intent.Explanation)
			if fb.Comment != "" {
				fmt.Printf("      Comment: %s\n", fb.Comment)
			}
		}
	}
}
