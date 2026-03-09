package cmd

import (
	"fmt"

	"github.com/keymastervn/gh-intent-review/internal/diff"
	"github.com/keymastervn/gh-intent-review/internal/github"
	"github.com/keymastervn/gh-intent-review/internal/ui"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review <pr-url>",
	Short: "Interactively review intents for a pull request",
	Long: `Opens an interactive session to walk through each intent in the
focused diff. For each intent, you can:

  [a] Approve  - mark the intent as acceptable
  [d] Disapprove - flag the intent with a comment
  [s] Skip     - move to the next intent
  [q] Quit     - exit the review session

Disapproved intents are collected and can be posted as PR comments.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prURL := args[0]

		pr, err := github.ParsePRURL(prURL)
		if err != nil {
			return fmt.Errorf("parsing PR URL: %w", err)
		}

		// Load the intentional diff
		path := diff.DefaultOutputPath(pr.Owner, pr.Repo, pr.Number)
		focusedDiff, err := diff.ReadFocusedDiff(path)
		if err != nil {
			return fmt.Errorf("reading intentional diff at %s (did you run 'generate' first?): %w", path, err)
		}

		// Start interactive review session
		session := ui.NewReviewSession(focusedDiff, pr)
		result, err := session.Run()
		if err != nil {
			return fmt.Errorf("review session: %w", err)
		}

		// Print summary
		result.PrintSummary()

		return nil
	},
}
