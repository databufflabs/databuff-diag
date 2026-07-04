package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "databuff-diag",
	Short: "DataBuff on-site environment diagnostic tool",
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
