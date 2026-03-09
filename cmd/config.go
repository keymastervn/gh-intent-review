package cmd

import (
	"fmt"
	"os"

	"github.com/keymastervn/gh-intent-review/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage gh-intent-review configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Init()
		if err != nil {
			return fmt.Errorf("initializing config: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Config written to %s\n", path)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := config.Load("")
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if loaded.ConfigPath != "" {
			fmt.Fprintf(os.Stderr, "# Config loaded from: %s\n", loaded.ConfigPath)
		} else {
			fmt.Fprintf(os.Stderr, "# No config file found — showing defaults\n")
		}
		fmt.Fprint(os.Stdout, loaded.Config.String())
		return nil
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
}
