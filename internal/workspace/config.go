//go:build !tldlocal

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

const (
	// GlobalConfigName is the canonical filename for the global tld config.
	GlobalConfigName = "tld.global.yaml"
	// LegacyGlobalConfigName is the previous global config filename. It is
	// still read for backwards compatibility but never written.
	LegacyGlobalConfigName = "tld.yaml"
)

// ConfigPath returns the canonical path to the global configuration file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, GlobalConfigName), nil
}

// LegacyConfigPath returns the legacy global configuration file path.
func LegacyConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, LegacyGlobalConfigName), nil
}

// ExistingGlobalConfigPath returns the path to an existing global config file,
// preferring the canonical tld.global.yaml and falling back to the legacy
// tld.yaml. The boolean is false when neither file exists.
func ExistingGlobalConfigPath() (string, bool) {
	if path, err := ConfigPath(); err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return path, true
		}
	}
	if legacy, err := LegacyConfigPath(); err == nil {
		if _, statErr := os.Stat(legacy); statErr == nil {
			return legacy, true
		}
	}
	path, _ := ConfigPath()
	return path, false
}

// ResolveConfigPath returns the global configuration file to read from. The
// canonical tld.global.yaml is preferred when present, falling back to the
// legacy tld.yaml. When neither exists the canonical path is returned so new
// files are created with the new name.
func ResolveConfigPath() (string, error) {
	if path, ok := ExistingGlobalConfigPath(); ok {
		return path, nil
	}
	return ConfigPath()
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
	Apply       ApplyConfig      `yaml:"apply"`
	Database    DatabaseConfig   `yaml:"database"`
	Validation  ValidationConfig `yaml:"validation"`
	Serve       ServeConfig      `yaml:"serve"`
	Index       IndexConfig      `yaml:"index"`
	Completion  CompletionConfig `yaml:"completion"`
	Updates     UpdatesConfig    `yaml:"updates"`
}

// IndexConfig configures the in-tree codeindex engine: embedding endpoint,
// external SCIP indexers, and the vector backend.
type IndexConfig struct {
	Embedding IndexEmbeddingConfig `yaml:"embedding"`
	Tools     IndexToolsConfig     `yaml:"tools"`
	Vector    IndexVectorConfig    `yaml:"vector"`
}

// IndexEmbeddingConfig is the OpenAI-compatible chunk-embedding pipeline config.
type IndexEmbeddingConfig struct {
	Endpoint       string `yaml:"endpoint"`
	Model          string `yaml:"model"`
	APIKey         string `yaml:"api_key"`
	Task           string `yaml:"task"`
	DocumentPrefix string `yaml:"document_prefix"`
	QueryPrefix    string `yaml:"query_prefix"`
	Dimensions     int    `yaml:"dimensions"`
	BatchSize      int    `yaml:"batch_size"`
	MaxInputChars  int    `yaml:"max_input_chars"`
}

// IndexToolsConfig overrides the external SCIP indexer binaries resolved on PATH.
type IndexToolsConfig struct {
	SCIPGo         string `yaml:"scip_go"`
	SCIPTypeScript string `yaml:"scip_typescript"`
	SCIPPython     string `yaml:"scip_python"`
	SCIPDotnet     string `yaml:"scip_dotnet"`
	SCIPClang      string `yaml:"scip_clang"`
	SCIPJava       string `yaml:"scip_java"`
	SCIPDart       string `yaml:"scip_dart"`
	SCIPPhp        string `yaml:"scip_php"`
	SCIPRuby       string `yaml:"scip_ruby"`
	RustAnalyzer   string `yaml:"rust_analyzer"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// IndexVectorConfig selects the vector index backend.
type IndexVectorConfig struct {
	// Backend is "sqlite-vec" (embedded) or "pgvector" (Postgres). Empty follows
	// the database dialect.
	Backend string `yaml:"backend"`
}

// ApplyConfig controls where CLI workspace plans are materialized.
type ApplyConfig struct {
	Target string `yaml:"target"`
}

type DatabaseConfig struct {
	Driver      string `yaml:"driver"`
	DatabaseURL string `yaml:"url"`
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
	Host           string   `yaml:"host"`
	Port           string   `yaml:"port"`
	DataDir        string   `yaml:"data_dir"`
	PublicURL      string   `yaml:"public_url"`
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type CompletionConfig struct {
	Remote bool `yaml:"remote"`
}

type UpdatesConfig struct {
	Auto          bool   `yaml:"auto"`
	CheckInterval string `yaml:"check_interval"`
}

const DefaultValidationLevel = 2

// DefaultConfig returns a Config struct populated with system defaults.
func DefaultConfig() *Config {
	return &Config{
		ServerURL: "https://tldiagram.com",
		Apply: ApplyConfig{
			Target: "auto",
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
		},
		Validation: ValidationConfig{
			Level: DefaultValidationLevel,
		},
		Serve: ServeConfig{
			Host: "127.0.0.1",
			Port: "8060",
		},
		Index: IndexConfig{
			Embedding: IndexEmbeddingConfig{
				Task:      "nl2code",
				BatchSize: 32,
			},
			Tools: IndexToolsConfig{
				SCIPGo:         "scip-go",
				SCIPTypeScript: "scip-typescript",
				SCIPPython:     "scip-python",
				SCIPDotnet:     "scip-dotnet",
				SCIPClang:      "scip-clang",
				SCIPJava:       "scip-java",
				SCIPDart:       "scip-dart",
				SCIPPhp:        "scip-php",
				SCIPRuby:       "scip-ruby",
				RustAnalyzer:   "rust-analyzer",
				TimeoutSeconds: 300,
			},
			Vector: IndexVectorConfig{Backend: "sqlite-vec"},
		},
		Updates: UpdatesConfig{
			Auto:          false,
			CheckInterval: "24h",
		},
	}
}

// LoadGlobalConfig reads the global config file, applies defaults to missing fields,
// and handles environment variable overrides.
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
// An existing legacy tld.yaml satisfies the requirement and is left in place.
func EnsureGlobalConfig() error {
	if _, ok := ExistingGlobalConfigPath(); ok {
		return nil
	}
	return SaveGlobalConfig(DefaultConfig())
}

// ResetGlobalConfig rewrites the global config file with full defaults.
func ResetGlobalConfig() error {
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
