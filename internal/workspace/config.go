//go:build !tldlocal

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
	Watch       WatchConfig      `yaml:"watch"`
	Completion  CompletionConfig `yaml:"completion"`
	Updates     UpdatesConfig    `yaml:"updates"`
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

type WatchEmbeddingConfig struct {
	Provider        string       `yaml:"provider"`
	Endpoint        EndpointList `yaml:"endpoint"`
	Model           string       `yaml:"model"`
	Dimension       int          `yaml:"dimension"`
	RuntimePath     string       `yaml:"runtime_path"`
	HealthThreshold float64      `yaml:"health_threshold"`
	MaxTokens       int          `yaml:"max_tokens"`
}

type EndpointList []string

func (e EndpointList) String() string {
	return strings.Join(e.Values(), ",")
}

func (e EndpointList) Values() []string {
	out := make([]string, 0, len(e))
	for _, value := range e {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, strings.TrimRight(part, "/"))
			}
		}
	}
	return out
}

func (e *EndpointList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	var values []string
	switch value.Kind {
	case yaml.SequenceNode:
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("endpoint entries must be strings")
			}
			values = append(values, item.Value)
		}
	case yaml.ScalarNode:
		values = append(values, value.Value)
	default:
		return fmt.Errorf("endpoint must be a string or list of strings")
	}
	*e = EndpointList(values).Values()
	return nil
}

type WatchThresholdConfig struct {
	MaxElementsPerView            int `yaml:"max_elements_per_view"`
	MaxConnectorsPerView          int `yaml:"max_connectors_per_view"`
	MaxIncomingPerElement         int `yaml:"max_incoming_per_element"`
	MaxOutgoingPerElement         int `yaml:"max_outgoing_per_element"`
	MaxExpandedConnectorsPerGroup int `yaml:"max_expanded_connectors_per_group"`
}

type WatchVisibilityWeightsConfig struct {
	Changed               float64 `yaml:"changed"`
	Selected              float64 `yaml:"selected"`
	UserShow              float64 `yaml:"user_show"`
	UserHide              float64 `yaml:"user_hide"`
	HighSignalFact        float64 `yaml:"high_signal_fact"`
	RelationshipProximity float64 `yaml:"relationship_proximity"`
	DependencyFact        float64 `yaml:"dependency_fact"`
	UtilityNoise          float64 `yaml:"utility_noise"`
	HighDegreeNoise       float64 `yaml:"high_degree_noise"`
}

type WatchVisibilityConfig struct {
	CoreThresholdEnabled   bool                         `yaml:"core_threshold_enabled"`
	CoreThreshold          float64                      `yaml:"core_threshold"`
	TierMultiplier         float64                      `yaml:"tier_multiplier"`
	MaxExpansionMultiplier float64                      `yaml:"max_expansion_multiplier"`
	Weights                WatchVisibilityWeightsConfig `yaml:"weights"`
}

type WatchLayoutConfig struct {
	LinkDistance    float64 `yaml:"link_distance"`
	ChargeStrength  float64 `yaml:"charge_strength"`
	CollideRadius   float64 `yaml:"collide_radius"`
	GravityStrength float64 `yaml:"gravity_strength"`
}

type WatchScaleConfig struct {
	Strategy           string `yaml:"strategy"`
	MaxTrackedFiles    int    `yaml:"max_tracked_files"`
	MaxLimitedFiles    int    `yaml:"max_limited_files"`
	MaxRecentFiles     int    `yaml:"max_recent_files"`
	MaxCallerDepth     int    `yaml:"max_caller_depth"`
	MaxBlastRadiusHops int    `yaml:"max_blast_radius_hops"`
}

type WatchDependencyConfig struct {
	Enabled bool `yaml:"enabled"`
}

type WatchLSPConfig struct {
	Enabled          bool              `yaml:"enabled"`
	HealthInterval   string            `yaml:"health_interval"`
	MemoryLimitBytes int64             `yaml:"memory_limit_bytes"`
	Commands         map[string]string `yaml:"commands"`
}

type WatchConfig struct {
	Languages    []string              `yaml:"languages"`
	Watcher      string                `yaml:"watcher"`
	PollInterval string                `yaml:"poll_interval"`
	Debounce     string                `yaml:"debounce"`
	Thresholds   WatchThresholdConfig  `yaml:"thresholds"`
	Visibility   WatchVisibilityConfig `yaml:"visibility"`
	Embedding    WatchEmbeddingConfig  `yaml:"embedding"`
	Layout       WatchLayoutConfig     `yaml:"layout"`
	Scale        WatchScaleConfig      `yaml:"scale"`
	Dependencies WatchDependencyConfig `yaml:"dependencies"`
	LSP          WatchLSPConfig        `yaml:"lsp"`
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
		Watch: WatchConfig{
			Languages:    []string{"go", "python", "typescript", "javascript", "java", "c", "cpp", "rust"},
			Watcher:      "auto",
			PollInterval: "10s",
			Debounce:     "500ms",
			Thresholds: WatchThresholdConfig{
				MaxElementsPerView:            100,
				MaxConnectorsPerView:          200,
				MaxIncomingPerElement:         20,
				MaxOutgoingPerElement:         20,
				MaxExpandedConnectorsPerGroup: 24,
			},
			Visibility: WatchVisibilityConfig{
				CoreThresholdEnabled:   true,
				CoreThreshold:          1,
				TierMultiplier:         0.5,
				MaxExpansionMultiplier: 2,
				Weights: WatchVisibilityWeightsConfig{
					Changed:               100,
					Selected:              100,
					UserShow:              100,
					UserHide:              -100,
					HighSignalFact:        1.5,
					RelationshipProximity: 1,
					DependencyFact:        0.2,
					UtilityNoise:          -0.8,
					HighDegreeNoise:       -1.5,
				},
			},
			Embedding: WatchEmbeddingConfig{
				Provider:        "local-lexical",
				Endpoint:        EndpointList{"http://127.0.0.1:8000/v1/embeddings"},
				Model:           "embeddinggemma-300m-4bit",
				HealthThreshold: 0.70,
			},
			Layout: WatchLayoutConfig{
				LinkDistance:    100,
				ChargeStrength:  -400,
				CollideRadius:   180,
				GravityStrength: 0.05,
			},
			Scale: WatchScaleConfig{
				Strategy:           "auto",
				MaxTrackedFiles:    15000,
				MaxLimitedFiles:    2000,
				MaxRecentFiles:     1000,
				MaxCallerDepth:     10,
				MaxBlastRadiusHops: 1,
			},
			Dependencies: WatchDependencyConfig{
				Enabled: false,
			},
			LSP: WatchLSPConfig{
				Enabled:          true,
				HealthInterval:   "1m",
				MemoryLimitBytes: 4294967296,
				Commands: map[string]string{
					"c":          "",
					"cpp":        "",
					"go":         "",
					"java":       "",
					"javascript": "",
					"python":     "",
					"rust":       "",
					"typescript": "",
				},
			},
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
func EnsureGlobalConfig() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
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
