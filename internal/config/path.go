package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const DirName = ".databuff-diag"

// HomeDir returns the user config directory (~/.databuff-diag).
func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, DirName), nil
}
