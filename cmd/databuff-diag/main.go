package main

import (
	"os"

	"github.com/databufflabs/databuff-diag/cmd/databuff-diag/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
