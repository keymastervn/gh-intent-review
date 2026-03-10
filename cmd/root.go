package cmd

import (
	"fmt"
	"os"

	"github.com/keymastervn/gh-intent-review/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gh-intent-review",
	Short: "Agentic code review focused on intents, not diffs",
	Long: `gh-intent-review reinvents code review for the agentic era.

Instead of reviewing raw +/- diffs, it generates an intent-focused diff
using symbolic notation (¿!, ¿~, ¿&, ¿#, ¿=, ¿?) to highlight what
truly matters: security risks, performance drags, coupling violations,
cohesion issues, DRY violations, and complexity concerns.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of gh-intent-review",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(os.Stdout, "gh-intent-review %s\n", version.Current)
	},
}
