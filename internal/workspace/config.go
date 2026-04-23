package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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

// ServeConfig holds serve-specific settings from the global config file.
type ServeConfig struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

// GlobalConfig represents the global tld.yaml configuration file.
type GlobalConfig struct {
	Serve ServeConfig `yaml:"serve"`
}

// LoadGlobalConfig reads the global config file. Missing file is not an error.
func LoadGlobalConfig() (*GlobalConfig, error) {
	cfgPath, err := ConfigPath()
	if err != nil {
		return &GlobalConfig{}, nil
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return &GlobalConfig{}, nil
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &GlobalConfig{}, nil
	}
	return &cfg, nil
}
