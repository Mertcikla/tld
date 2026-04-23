package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir returns the path to the global configuration directory.
func ConfigDir() (string, error) {
	if override := os.Getenv("TLD_CONFIG_DIR"); override != "" {
		return override, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tldiagram"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "tldiagram"), nil
}

// ConfigPath returns the path to the global configuration file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tld.yaml"), nil
}

// WorkspaceConfigPath returns the path to the workspace-local configuration file.
func WorkspaceConfigPath(dir string) string {
	return filepath.Join(dir, ".tld.yaml")
}
