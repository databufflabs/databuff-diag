package cmd

import (
	"fmt"

	"github.com/databufflabs/databuff-diag/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration utilities",
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the user config directory path",
	Run: func(cmd *cobra.Command, args []string) {
		dir, err := config.HomeDir()
		if err != nil {
			exitWithError(err)
		}
		fmt.Println(dir)
	},
}

func init() {
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}
