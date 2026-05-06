package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Config holds all global tld configuration, merging server settings,
// watch behaviors, and authentication.
type Config struct {
	ServerURL   string           `yaml:"server_url"`
	APIKey      string           `yaml:"api_key"`
	WorkspaceID string           `yaml:"org_id"`
	Validation  ValidationConfig `yaml:"validation"`
	Serve       ServeConfig      `yaml:"serve"`
	Watch       WatchConfig      `yaml:"watch"`
	Completion  CompletionConfig `yaml:"completion"`
}

// ValidationConfig represents workspace validation settings.
type ValidationConfig struct {
	Level           int      `yaml:"level"`
	AllowLowInsight bool     `yaml:"allow_low_insight"`
	IncludeRules    []string `yaml:"include_rules,omitempty"`
	ExcludeRules    []string `yaml:"exclude_rules,omitempty"`
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
	MaxElementsPerView            int `yaml:"max_elements_per_view"`
	MaxConnectorsPerView          int `yaml:"max_connectors_per_view"`
	MaxIncomingPerElement         int `yaml:"max_incoming_per_element"`
	MaxOutgoingPerElement         int `yaml:"max_outgoing_per_element"`
	MaxExpandedConnectorsPerGroup int `yaml:"max_expanded_connectors_per_group"`
}

type WatchLayoutConfig struct {
	LinkDistance    float64 `yaml:"link_distance"`
	ChargeStrength  float64 `yaml:"charge_strength"`
	CollideRadius   float64 `yaml:"collide_radius"`
	GravityStrength float64 `yaml:"gravity_strength"`
}

type WatchConfig struct {
	Languages    []string             `yaml:"languages"`
	Watcher      string               `yaml:"watcher"`
	PollInterval string               `yaml:"poll_interval"`
	Debounce     string               `yaml:"debounce"`
	Thresholds   WatchThresholdConfig `yaml:"thresholds"`
	Embedding    WatchEmbeddingConfig `yaml:"embedding"`
	Layout       WatchLayoutConfig    `yaml:"layout"`
}

type CompletionConfig struct {
	Remote bool `yaml:"remote"`
}

const DefaultValidationLevel = 2

// DefaultConfig returns a Config struct populated with system defaults.
func DefaultConfig() *Config {
	return &Config{
		ServerURL: "https://tldiagram.com",
		Validation: ValidationConfig{
			Level: DefaultValidationLevel,
		},
		Serve: ServeConfig{
			Host: "127.0.0.1",
			Port: "8060",
		},
		Watch: WatchConfig{
			Languages:    []string{"go", "python", "typescript", "javascript", "java", "c", "cpp", "rust"},
			Watcher:      "auto",
			PollInterval: "1s",
			Debounce:     "500ms",
			Thresholds: WatchThresholdConfig{
				MaxElementsPerView:            100,
				MaxConnectorsPerView:          200,
				MaxIncomingPerElement:         20,
				MaxOutgoingPerElement:         20,
				MaxExpandedConnectorsPerGroup: 24,
			},
			Embedding: WatchEmbeddingConfig{
				Provider:        "local-lexical",
				Endpoint:        "http://127.0.0.1:8000/v1/embeddings",
				Model:           "embeddinggemma-300m-4bit",
				HealthThreshold: 0.70,
			},
			Layout: WatchLayoutConfig{
				LinkDistance:    100,
				ChargeStrength:  -400,
				CollideRadius:   180,
				GravityStrength: 0.05,
			},
		},
	}
}

// LoadGlobalConfig reads the global config file, applies defaults to missing fields,
// handles environment variable overrides, and persists any added defaults back to YAML.
func LoadGlobalConfig() (*Config, error) {
	state, err := LoadGlobalConfigState()
	if err != nil {
		return nil, err
	}
	return state.Config, nil
}

// SaveGlobalConfig writes the config back to the global configuration file.
func SaveGlobalConfig(cfg *Config) error {
	return SaveGlobalConfigPreservingUnknown(cfg, nil)
}

// EnsureGlobalConfig ensures the global config file exists with full defaults.
func EnsureGlobalConfig() error {
	return SaveGlobalConfig(DefaultConfig())
}

// ResolveDataDir returns the absolute path to the data directory, applying
// resolution priority: flag > env (TLD_DATA_DIR) > config > default.
func ResolveDataDir(cfg *Config, flagDir string) (string, error) {
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
