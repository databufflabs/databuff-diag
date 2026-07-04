package cmd

import (
	"fmt"

	"github.com/databufflabs/databuff-diag/internal/server"
	"github.com/spf13/cobra"
)

const defaultListen = ":8787"

var listenAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("databuff-diag listening on %s\n", listenAddr)
		if err := server.ListenAndServe(listenAddr); err != nil {
			exitWithError(err)
		}
	},
}

func init() {
	serveCmd.Flags().StringVar(&listenAddr, "listen", defaultListen, "listen address")
	rootCmd.AddCommand(serveCmd)
}
