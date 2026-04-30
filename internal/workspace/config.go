package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// DataDir returns the default directory for server state, including the
// local SQLite database and logs.
func DataDir() (string, error) {
	if override := os.Getenv("TLD_DATA_DIR"); override != "" {
		return filepath.Abs(override)
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "tldiagram"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "tldiagram"), nil
}

// WorkspaceConfigPath returns the path to the workspace-local configuration file.
func WorkspaceConfigPath(dir string) string {
	return filepath.Join(dir, ".tld.yaml")
}

// ServeConfig holds serve-specific settings from the global config file.
type ServeConfig struct {
	Host    string `yaml:"host"`
	Port    string `yaml:"port"`
	DataDir string `yaml:"data_dir"`
}

type WatchEmbeddingConfig struct {
	Provider        string  `yaml:"provider"`
	Endpoint        string  `yaml:"endpoint"`
	Model           string  `yaml:"model"`
	Dimension       int     `yaml:"dimension"`
	HealthThreshold float64 `yaml:"health_threshold"`
}

type WatchThresholdConfig struct {
	MaxElementsPerView    int `yaml:"max_elements_per_view"`
	MaxConnectorsPerView  int `yaml:"max_connectors_per_view"`
	MaxIncomingPerElement int `yaml:"max_incoming_per_element"`
	MaxOutgoingPerElement int `yaml:"max_outgoing_per_element"`
}

type WatchConfig struct {
	Languages    []string             `yaml:"languages"`
	Watcher      string               `yaml:"watcher"`
	PollInterval string               `yaml:"poll_interval"`
	Debounce     string               `yaml:"debounce"`
	Thresholds   WatchThresholdConfig `yaml:"thresholds"`
	Embedding    WatchEmbeddingConfig `yaml:"embedding"`
}

// GlobalConfig represents the global tld.yaml configuration file.
type GlobalConfig struct {
	Serve ServeConfig `yaml:"serve"`
	Watch WatchConfig `yaml:"watch"`
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

// EnsureGlobalConfig ensures the global config file exists.
// If it doesn't, it writes a default one with commented instructions.
func EnsureGlobalConfig() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil // Already exists
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	defaultConfig := `# tlDiagram global configuration
serve:
  host: 127.0.0.1
  port: 8060
  # data_dir: ~/.local/share/tldiagram
watch:
  # languages: [go, python, typescript, javascript, java, c, cpp]
  # watcher: auto # auto, fsnotify, or poll
  # poll_interval: 1s
  # debounce: 500ms
  embedding:
    provider: openai
    endpoint: http://127.0.0.1:8000/v1/embeddings
    model: embeddinggemma-300m-4bit
`
	return os.WriteFile(path, []byte(defaultConfig), 0o644)
}

// ResolveDataDir returns the absolute path to the data directory, applying
// resolution priority: flag > env (TLD_DATA_DIR) > config > default.
func ResolveDataDir(cfg *GlobalConfig, flagDir string) (string, error) {
	// 1. Flag
	if flagDir != "" {
		return filepath.Abs(flagDir)
	}

	// 2. Env
	if env := os.Getenv("TLD_DATA_DIR"); env != "" {
		return filepath.Abs(env)
	}

	// 3. Config
	if cfg.Serve.DataDir != "" {
		dir := cfg.Serve.DataDir
		if strings.HasPrefix(dir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(home, dir[2:])
		}
		return filepath.Abs(dir)
	}

	// 4. Default
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	return base, nil
}
