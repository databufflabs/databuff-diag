package cmd

import (
	"github.com/databufflabs/databuff-diag/internal/daemon"
	"github.com/databufflabs/databuff-diag/internal/server"
	"github.com/spf13/cobra"
)

const defaultListen = ":8787"

var (
	listenAddr  string
	daemonMode  bool
	foreground  bool
	pidFile     string
	logFile     string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		runInBackground := daemonMode && !foreground
		if runInBackground {
			if !daemon.IsChild() {
				if err := daemon.Fork(daemon.Options{
					Listen:  listenAddr,
					PIDFile: pidFile,
					LogFile: logFile,
				}); err != nil {
					exitWithError(err)
				}
				return
			}
			if err := daemon.SetupChild(pidFile, logFile); err != nil {
				exitWithError(err)
			}
			defer daemon.Cleanup(pidFile)
		}

		if err := server.ListenAndServe(listenAddr); err != nil {
			exitWithError(err)
		}
	},
}

func init() {
	defaultPID, _ := daemon.DefaultPIDFile()
	defaultLog, _ := daemon.DefaultLogFile()

	serveCmd.Flags().StringVar(&listenAddr, "listen", defaultListen, "listen address")
	serveCmd.Flags().BoolVar(&daemonMode, "daemon", true, "run in background")
	serveCmd.Flags().BoolVar(&foreground, "foreground", false, "run in foreground (closing the terminal stops the service)")
	serveCmd.Flags().StringVar(&pidFile, "pid-file", defaultPID, "PID file path when running in background")
	serveCmd.Flags().StringVar(&logFile, "log-file", defaultLog, "log file path when running in background")
	rootCmd.AddCommand(serveCmd)
}
