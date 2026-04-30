package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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

// Config holds all global tld configuration, merging server settings,
// watch behaviors, and authentication.
type Config struct {
	ServerURL   string           `yaml:"server_url"`
	APIKey      string           `yaml:"api_key"`
	WorkspaceID string           `yaml:"org_id"`
	Validation  ValidationConfig `yaml:"validation"`
	Serve       ServeConfig      `yaml:"serve"`
	Watch       WatchConfig      `yaml:"watch"`
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
			Languages:    []string{"go", "python", "typescript", "javascript", "java", "c", "cpp"},
			Watcher:      "auto",
			PollInterval: "1s",
			Debounce:     "500ms",
			Thresholds: WatchThresholdConfig{
				MaxElementsPerView:    100,
				MaxConnectorsPerView:  200,
				MaxIncomingPerElement: 20,
				MaxOutgoingPerElement: 20,
			},
			Embedding: WatchEmbeddingConfig{
				Provider:        "local-lexical",
				Endpoint:        "http://127.0.0.1:8000/v1/embeddings",
				Model:           "embeddinggemma-300m-4bit",
				HealthThreshold: 0.70,
			},
		},
	}
}

// LoadGlobalConfig reads the global config file, applies defaults to missing fields,
// handles environment variable overrides, and persists any added defaults back to YAML.
func LoadGlobalConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Save default config if missing entirely
			_ = SaveGlobalConfig(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read global config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse global config: %w", err)
	}

	// baseCfg holds File + Defaults (no overrides) for comparison and saving.
	baseCfg := *cfg

	// Apply Env Overrides to cfg (but not baseCfg)
	applyEnvOverrides(cfg)

	// If the file was missing fields that we've now filled with defaults,
	// save the baseCfg (the one without env overrides) back to disk.
	if shouldSave(data, &baseCfg) {
		_ = SaveGlobalConfig(&baseCfg)
	}

	return cfg, nil
}

// shouldSave returns true if the current base config has fields that weren't in the original data.
func shouldSave(originalData []byte, cfg *Config) bool {
	if len(originalData) == 0 {
		return true
	}
	// Simple approach: marshal and compare. If we added fields, it will be different.
	// We use a normalized marshaling to avoid noise from formatting.
	var current map[string]any
	_ = yaml.Unmarshal(originalData, &current)

	newData, _ := yaml.Marshal(cfg)
	var updated map[string]any
	_ = yaml.Unmarshal(newData, &updated)

	return !reflect.DeepEqual(current, updated)
}

func applyEnvOverrides(cfg *Config) {
	envs := []struct {
		key    string
		target *string
		label  string
	}{
		{"TLD_SERVER_URL", &cfg.ServerURL, "server_url"},
		{"TLD_API_KEY", &cfg.APIKey, "api_key"},
		{"TLD_ORG_ID", &cfg.WorkspaceID, "org_id"},
		{"TLD_HOST", &cfg.Serve.Host, "serve.host"},
		{"PORT", &cfg.Serve.Port, "serve.port"},
		{"TLD_WATCH_WATCHER", &cfg.Watch.Watcher, "watch.watcher"},
		{"TLD_WATCH_POLL_INTERVAL", &cfg.Watch.PollInterval, "watch.poll_interval"},
		{"TLD_WATCH_DEBOUNCE", &cfg.Watch.Debounce, "watch.debounce"},
		{"TLD_EMBEDDING_PROVIDER", &cfg.Watch.Embedding.Provider, "watch.embedding.provider"},
		{"TLD_EMBEDDING_ENDPOINT", &cfg.Watch.Embedding.Endpoint, "watch.embedding.endpoint"},
		{"TLD_EMBEDDING_MODEL", &cfg.Watch.Embedding.Model, "watch.embedding.model"},
	}

	for _, env := range envs {
		if v := os.Getenv(env.key); v != "" {
			if *env.target != v {
				fmt.Fprintf(os.Stderr, "Notice: %s overridden by %s\n", env.label, env.key)
				*env.target = v
			}
		}
	}

	// Numeric overrides
	if v := os.Getenv("TLD_EMBEDDING_DIMENSION"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			if cfg.Watch.Embedding.Dimension != d {
				fmt.Fprintf(os.Stderr, "Notice: watch.embedding.dimension overridden by TLD_EMBEDDING_DIMENSION\n")
				cfg.Watch.Embedding.Dimension = d
			}
		}
	}

	// Slice overrides
	if v := os.Getenv("TLD_WATCH_LANGUAGES"); v != "" {
		langs := strings.Split(v, ",")
		for i := range langs {
			langs[i] = strings.TrimSpace(langs[i])
		}
		if !reflect.DeepEqual(cfg.Watch.Languages, langs) {
			fmt.Fprintf(os.Stderr, "Notice: watch.languages overridden by TLD_WATCH_LANGUAGES\n")
			cfg.Watch.Languages = langs
		}
	}

	// TLD_ADDR overrides both host and port if it contains a colon
	if addr := os.Getenv("TLD_ADDR"); addr != "" {
		parts := strings.Split(addr, ":")
		if len(parts) == 2 {
			fmt.Fprintf(os.Stderr, "Notice: serve.host and serve.port overridden by TLD_ADDR\n")
			cfg.Serve.Host = parts[0]
			cfg.Serve.Port = parts[1]
		}
	}
}

// SaveGlobalConfig writes the config back to the global configuration file.
func SaveGlobalConfig(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# tlDiagram global configuration\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
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
