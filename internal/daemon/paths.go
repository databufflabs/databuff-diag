package daemon

import (
	"path/filepath"

	"github.com/databufflabs/databuff-diag/internal/config"
)

// DefaultPIDFile returns the default PID file path (~/.databuff-diag/databuff-diag.pid).
func DefaultPIDFile() (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "databuff-diag.pid"), nil
}

// DefaultLogFile returns the default log file path (~/.databuff-diag/databuff-diag.log).
func DefaultLogFile() (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "databuff-diag.log"), nil
}
