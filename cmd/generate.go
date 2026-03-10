package cmd

import (
	"fmt"
	"os"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/keymastervn/gh-intent-review/internal/diff"
	"github.com/keymastervn/gh-intent-review/internal/github"
	"github.com/keymastervn/gh-intent-review/internal/reviewer"
	"github.com/spf13/cobra"
)

var (
	generateModel  string
	generateOutput string
	generateConfig string
)

var generateCmd = &cobra.Command{
	Use:   "generate <pr-url>",
	Short: "Generate an intent-focused diff for a pull request",
	Long: `Fetches the PR diff, runs parallel agentic review on each file,
and produces an intentional diff using intent notation (¿!, ¿~, ¿&, ¿#, ¿=, ¿?).

The generated diff is stored at:
  ~/.gh-intent-review/<owner>/<repo>/<pr>-<short-sha>.intentional.diff

Or at a custom dir if output.dir is set in .gh-intent-review.yml.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prURL := args[0]

		loaded, err := config.Load(generateConfig)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg := loaded.Config

		if loaded.ConfigPath != "" {
			fmt.Fprintf(os.Stderr, "Using config: %s\n", loaded.ConfigPath)
		} else {
			fmt.Fprintf(os.Stderr, "No config file found — using defaults (provider: %s)\n", cfg.LLM.Provider)
			fmt.Fprintf(os.Stderr, "Tip: run from a directory containing .gh-intent-review.yml, or pass --config <path>\n")
		}

		// CLI flag overrides config
		if generateModel != "" {
			cfg.LLM.Model = generateModel
		}

		// Parse PR URL
		pr, err := github.ParsePRURL(prURL)
		if err != nil {
			return fmt.Errorf("parsing PR URL: %w", err)
		}

		// Fetch the diff from GitHub
		client, err := github.NewClient()
		if err != nil {
			return fmt.Errorf("creating GitHub client: %w", err)
		}

		// Fetch head SHA for versioned filename
		headSHA, err := client.GetPRHeadSHA(pr)
		if err != nil {
			return fmt.Errorf("fetching PR head SHA: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Fetching diff for %s/%s#%d (head: %s)...\n", pr.Owner, pr.Repo, pr.Number, diff.ShortSHA(headSHA))

		// Determine output path
		outputPath := generateOutput
		if outputPath == "" {
			if cfg.Output.Dir != "" {
				outputPath = diff.ProjectOutputPathWithSHA(cfg.Output.Dir, pr.Owner, pr.Repo, pr.Number, headSHA)
			} else {
				outputPath = diff.DefaultOutputPathWithSHA(pr.Owner, pr.Repo, pr.Number, headSHA)
			}
		}

		if err := generateIntentDiff(cfg, client, pr, outputPath, prURL); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Intent diff written to %s\n", outputPath)
		fmt.Fprintf(os.Stderr, "\nRun: gh intent-review review %s\n", prURL)

		return nil
	},
}

// generateIntentDiff fetches the PR diff, runs the agentic review, and writes the intentional diff.
// It is shared between the generate command and the review command's check_and_fetch auto-regeneration.
// prURL is the original GitHub PR URL passed to the agent for codebase context.
func generateIntentDiff(cfg *config.Config, client *github.Client, pr *github.PullRequest, outputPath, prURL string) error {
	rawDiff, err := client.GetPRDiff(pr)
	if err != nil {
		return fmt.Errorf("fetching PR diff: %w", err)
	}

	fileDiffs, err := diff.ParseUnifiedDiff(rawDiff)
	if err != nil {
		return fmt.Errorf("parsing diff: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Running agent review on %d file(s)...\n", len(fileDiffs))

	engine, err := reviewer.NewEngine(cfg)
	if err != nil {
		return fmt.Errorf("creating review engine: %w", err)
	}

	focusedDiff, err := engine.Review(fileDiffs, prURL)
	if err != nil {
		return fmt.Errorf("running review: %w", err)
	}

	if err := diff.WriteFocusedDiff(outputPath, focusedDiff); err != nil {
		return fmt.Errorf("writing focused diff: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Found %d intents across %d files\n", focusedDiff.TotalIntents(), len(focusedDiff.Files))
	return nil
}

func init() {
	generateCmd.Flags().StringVar(&generateModel, "model", "", "Agent model to use (overrides config, passed via --model to agent)")
	generateCmd.Flags().StringVarP(&generateOutput, "output", "o", "", "Output path (default: ~/.gh-intent-review/<owner>/<repo>/<pr>-<sha>.intentional.diff)")
	generateCmd.Flags().StringVarP(&generateConfig, "config", "c", "", "Path to config file (default: .gh-intent-review.yml in CWD or home dir)")
}
