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
	generateModel    string
	generateProvider string
	generateOutput   string
	generateParallel int
)

var generateCmd = &cobra.Command{
	Use:   "generate <pr-url>",
	Short: "Generate an intent-focused diff for a pull request",
	Long: `Fetches the PR diff, runs parallel agentic review on each file,
and produces an intentional diff using intent notation (¿!, ¿~, ¿&, ¿#, ¿=, ¿?).

The generated diff is stored at:
  ~/.gh-intent-review/<owner>/<repo>/<pr>.intentional.diff

Or at a custom dir if output.dir is set in .gh-intent-review.yml.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prURL := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// CLI flags override config
		if generateModel != "" {
			cfg.LLM.Model = generateModel
		}
		if generateProvider != "" {
			cfg.LLM.Provider = generateProvider
		}
		if generateParallel > 0 {
			cfg.Review.Parallel = generateParallel
		}

		// Parse PR URL
		pr, err := github.ParsePRURL(prURL)
		if err != nil {
			return fmt.Errorf("parsing PR URL: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Fetching diff for %s/%s#%d...\n", pr.Owner, pr.Repo, pr.Number)

		// Fetch the diff from GitHub
		client, err := github.NewClient()
		if err != nil {
			return fmt.Errorf("creating GitHub client: %w", err)
		}

		rawDiff, err := client.GetPRDiff(pr)
		if err != nil {
			return fmt.Errorf("fetching PR diff: %w", err)
		}

		// Parse the raw diff into file diffs
		fileDiffs, err := diff.ParseUnifiedDiff(rawDiff)
		if err != nil {
			return fmt.Errorf("parsing diff: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Reviewing %d files with %d parallel workers...\n", len(fileDiffs), cfg.Review.Parallel)

		// Run the agentic review
		engine, err := reviewer.NewEngine(cfg)
		if err != nil {
			return fmt.Errorf("creating review engine: %w", err)
		}

		focusedDiff, err := engine.Review(fileDiffs)
		if err != nil {
			return fmt.Errorf("running review: %w", err)
		}

		// Write the intentional diff
		outputPath := generateOutput
		if outputPath == "" {
			if cfg.Output.Dir != "" {
				outputPath = diff.ProjectOutputPath(cfg.Output.Dir, pr.Owner, pr.Repo, pr.Number)
			} else {
				outputPath = diff.DefaultOutputPath(pr.Owner, pr.Repo, pr.Number)
			}
		}

		if err := diff.WriteFocusedDiff(outputPath, focusedDiff); err != nil {
			return fmt.Errorf("writing focused diff: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Intent diff written to %s\n", outputPath)
		fmt.Fprintf(os.Stderr, "Found %d intents across %d files\n", focusedDiff.TotalIntents(), len(focusedDiff.Files))
		fmt.Fprintf(os.Stderr, "\nRun: gh intent-review review %s\n", prURL)

		return nil
	},
}

func init() {
	generateCmd.Flags().StringVar(&generateModel, "model", "", "LLM model to use (overrides config)")
	generateCmd.Flags().StringVar(&generateProvider, "provider", "", "LLM provider (overrides config)")
	generateCmd.Flags().StringVarP(&generateOutput, "output", "o", "", "Output path (default: ~/.gh-intent-review/<owner>/<repo>/<pr>.intentional.diff)")
	generateCmd.Flags().IntVarP(&generateParallel, "parallel", "p", 0, "Number of parallel review workers (overrides config)")
}
