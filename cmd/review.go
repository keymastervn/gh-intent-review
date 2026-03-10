package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
	"github.com/keymastervn/gh-intent-review/internal/github"
	"github.com/keymastervn/gh-intent-review/internal/ui"
	"github.com/keymastervn/gh-intent-review/internal/version"
	"github.com/spf13/cobra"
)

var reviewConfig string

var reviewCmd = &cobra.Command{
	Use:   "review <pr-url>",
	Short: "Interactively review intents for a pull request",
	Long: `Opens an interactive session to walk through each intent in the
focused diff. For each intent, you can:

  [e] Elaborate - ask the LLM/agent to explain the issue in more detail
  [c] Comment   - post an AI-suggested comment on the PR via gh CLI
  [s] Skip      - move to the next intent
  [q] Quit      - exit the review session`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prURL := args[0]

		pr, err := github.ParsePRURL(prURL)
		if err != nil {
			return fmt.Errorf("parsing PR URL: %w", err)
		}

		loaded, err := config.Load(reviewConfig)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Resolve which diff file to use, checking against the current PR head SHA.
		diffPath := resolveDiffPath(loaded.Config, pr, prURL)

		focusedDiff, err := diff.ReadFocusedDiff(diffPath)
		if err != nil {
			return fmt.Errorf("reading intentional diff at %s (did you run 'generate' first?): %w", diffPath, err)
		}

		// Start interactive review session
		session := ui.NewReviewSession(focusedDiff, pr, loaded.Config)
		result, err := session.Run()
		if err != nil {
			return fmt.Errorf("review session: %w", err)
		}

		// Print summary
		result.PrintSummary()

		// Auto-approve if enabled and no intents were found, or every intent was skipped (none commented).
		if loaded.Config.Review.AutoApprove && (result.Total == 0 || result.Skipped == result.Total) {
			fmt.Fprintf(os.Stderr, "\nAuto-approving PR (no unresolved intents)...\n")
			llm := loaded.Config.LLM
			body := fmt.Sprintf(
				"Intent-based agentic review accepted, auto-approved\n\n_v%s model:%s provider:%s_",
				version.Current, llm.Model, llm.Provider,
			)
			if err := approvePR(prURL, body); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: auto-approve failed: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "✓ PR approved\n")
			}
		}

		return nil
	},
}

func init() {
	reviewCmd.Flags().StringVarP(&reviewConfig, "config", "c", "", "Path to config file (default: .gh-intent-review.yml in CWD or home dir)")
}

// approvePR runs `gh pr review --approve` with the given review body.
func approvePR(prURL, body string) error {
	cmd := exec.Command("gh", "pr", "review", prURL, "--approve", "--body", body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// resolveDiffPath determines which intentional diff file to use for the review session.
//
// Resolution order:
//  1. Fetch the current PR head SHA and look for an exact-match file (<pr>-<sha>.intentional.diff).
//  2. If not found and check_and_fetch is true: auto-generate a new intentional diff.
//  3. If not found and check_and_fetch is false: fall back to any existing <pr>-*.intentional.diff
//     (most recently modified) with a staleness warning.
//  4. Final fallback to the legacy path <pr>.intentional.diff for backward compatibility.
func resolveDiffPath(cfg *config.Config, pr *github.PullRequest, prURL string) string {
	outputDir := cfg.Output.Dir

	client, clientErr := github.NewClient()
	if clientErr == nil {
		headSHA, shaErr := client.GetPRHeadSHA(pr)
		if shaErr == nil {
			// 1. Exact SHA match
			if path, found := diff.FindDiffPath(outputDir, pr.Owner, pr.Repo, pr.Number, headSHA); found {
				return path
			}

			// 2. check_and_fetch: auto-generate
			if cfg.Review.CheckAndFetch {
				var genPath string
				if outputDir != "" {
					genPath = diff.ProjectOutputPathWithSHA(outputDir, pr.Owner, pr.Repo, pr.Number, headSHA)
				} else {
					genPath = diff.DefaultOutputPathWithSHA(pr.Owner, pr.Repo, pr.Number, headSHA)
				}
				fmt.Fprintf(os.Stderr, "No diff found for current head %s — generating...\n", diff.ShortSHA(headSHA))
				if err := generateIntentDiff(cfg, client, pr, genPath, prURL); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: auto-generate failed: %v\n", err)
				} else {
					return genPath
				}
			} else {
				// 3. Fall back to any existing SHA-versioned file with a staleness warning
				if path, found := diff.FindDiffPath(outputDir, pr.Owner, pr.Repo, pr.Number, ""); found {
					fmt.Fprintf(os.Stderr, "Warning: no diff for current head %s — using existing file (may be outdated).\n         Set check_and_fetch: true in .gh-intent-review.yml to auto-update.\n", diff.ShortSHA(headSHA))
					return path
				}
			}
		}
	}

	// 4. Legacy fallback (<pr>.intentional.diff) for backward compatibility
	if outputDir != "" {
		return diff.ProjectOutputPath(outputDir, pr.Owner, pr.Repo, pr.Number)
	}
	return diff.DefaultOutputPath(pr.Owner, pr.Repo, pr.Number)
}
