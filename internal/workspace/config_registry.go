package workspace

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mertcikla/tld/internal/analyzer"
	"gopkg.in/yaml.v3"
)

type ConfigSource string

const (
	ConfigSourceDefault ConfigSource = "default"
	ConfigSourceFile    ConfigSource = "file"
	ConfigSourceEnv     ConfigSource = "env"
)

type ConfigDefinition struct {
	Key         string   `json:"key"`
	Env         []string `json:"env,omitempty"`
	Description string   `json:"description"`
	Secret      bool     `json:"secret,omitempty"`
}

type ConfigValue struct {
	Key         string       `json:"key"`
	Value       string       `json:"value"`
	Source      ConfigSource `json:"source"`
	Env         string       `json:"env,omitempty"`
	Description string       `json:"description"`
	Secret      bool         `json:"secret,omitempty"`
}

type ConfigValidationError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

func (e ConfigValidationError) Error() string {
	if e.Key == "" {
		return e.Message
	}
	return e.Key + ": " + e.Message
}

type ConfigValidationErrors []ConfigValidationError

func (e ConfigValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	return fmt.Sprintf("%s (+%d more)", e[0].Error(), len(e)-1)
}

type GlobalConfigState struct {
	Path     string
	Config   *Config
	File     *Config
	Values   []ConfigValue
	FileRoot *yaml.Node
}

func ConfigDefinitions() []ConfigDefinition {
	return append([]ConfigDefinition(nil), configDefinitions...)
}

func ConfigDefinitionForKey(key string) (ConfigDefinition, bool) {
	key = normalizeConfigKey(key)
	for _, def := range configDefinitions {
		if def.Key == key {
			return def, true
		}
	}
	return ConfigDefinition{}, false
}

func LoadGlobalConfigState() (*GlobalConfigState, error) {
	return loadGlobalConfigState(true)
}

func LoadGlobalConfigStateNoRepair() (*GlobalConfigState, error) {
	return loadGlobalConfigState(false)
}

func SetGlobalConfigValue(key, value string) error {
	key = normalizeConfigKey(key)
	if _, ok := ConfigDefinitionForKey(key); !ok {
		return fmt.Errorf("unknown global config key %q", key)
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	cfg := DefaultConfig()
	existingRoot, data, err := readConfigNode(path)
	if err != nil {
		if os.IsNotExist(err) {
			existingRoot = emptyConfigNode()
		} else {
			return err
		}
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return fmt.Errorf("parse global config: %w", err)
		}
	}
	if err := setConfigValue(cfg, key, value); err != nil {
		return err
	}
	if errs := ValidateGlobalConfig(cfg); len(errs) > 0 {
		return errs
	}
	return SaveGlobalConfigPreservingUnknown(cfg, existingRoot)
}

func SaveGlobalConfigPreservingUnknown(cfg *Config, existingRoot *yaml.Node) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if existingRoot == nil {
		existingRoot, _, _ = readConfigNode(path)
	}
	root := configToYAMLNode(cfg, existingRoot)
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ValidateGlobalConfig(cfg *Config) ConfigValidationErrors {
	var errs ConfigValidationErrors
	add := func(key, msg string) {
		errs = append(errs, ConfigValidationError{Key: key, Message: msg})
	}

	if strings.TrimSpace(cfg.ServerURL) != "" && !validHTTPURL(cfg.ServerURL) {
		add("server_url", "must be a valid URL")
	}
	if cfg.Validation.Level < 1 || cfg.Validation.Level > 3 {
		add("validation.level", "must be 1, 2, or 3")
	}
	if strings.TrimSpace(cfg.Serve.Host) == "" {
		add("serve.host", "must be non-empty")
	}
	if !validPort(cfg.Serve.Port) {
		add("serve.port", "must be an integer between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Serve.DataDir) != "" {
		if _, err := expandConfigPath(cfg.Serve.DataDir); err != nil {
			add("serve.data_dir", err.Error())
		}
	}

	for _, item := range []struct {
		key   string
		value string
	}{
		{"watch.poll_interval", cfg.Watch.PollInterval},
		{"watch.debounce", cfg.Watch.Debounce},
	} {
		d, err := time.ParseDuration(strings.TrimSpace(item.value))
		if err != nil || d <= 0 {
			add(item.key, "must be a positive duration such as 500ms or 1s")
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Watch.Watcher)) {
	case "auto", "fsnotify", "poll":
	default:
		add("watch.watcher", "must be auto, fsnotify, or poll")
	}
	if len(normalizeConfigLanguages(cfg.Watch.Languages)) == 0 {
		add("watch.languages", "must include at least one supported language")
	}
	for _, item := range []struct {
		key   string
		value int
	}{
		{"watch.thresholds.max_elements_per_view", cfg.Watch.Thresholds.MaxElementsPerView},
		{"watch.thresholds.max_connectors_per_view", cfg.Watch.Thresholds.MaxConnectorsPerView},
		{"watch.thresholds.max_incoming_per_element", cfg.Watch.Thresholds.MaxIncomingPerElement},
		{"watch.thresholds.max_outgoing_per_element", cfg.Watch.Thresholds.MaxOutgoingPerElement},
		{"watch.thresholds.max_expanded_connectors_per_group", cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup},
	} {
		if item.value <= 0 {
			add(item.key, "must be positive")
		}
	}
	for _, item := range []struct {
		key   string
		value float64
	}{
		{"watch.layout.link_distance", cfg.Watch.Layout.LinkDistance},
		{"watch.layout.collide_radius", cfg.Watch.Layout.CollideRadius},
		{"watch.layout.gravity_strength", cfg.Watch.Layout.GravityStrength},
	} {
		if item.value <= 0 {
			add(item.key, "must be positive")
		}
	}
	if cfg.Watch.Layout.ChargeStrength == 0 {
		add("watch.layout.charge_strength", "must be non-zero")
	}

	provider := strings.TrimSpace(cfg.Watch.Embedding.Provider)
	switch provider {
	case "none", "openai", "ollama", "local-lexical", "local-deterministic-test":
	default:
		add("watch.embedding.provider", "must be none, openai, ollama, local-lexical, or local-deterministic-test")
	}
	if cfg.Watch.Embedding.Dimension < 0 {
		add("watch.embedding.dimension", "must be non-negative")
	}
	if provider == "openai" || provider == "ollama" {
		if strings.TrimSpace(cfg.Watch.Embedding.Endpoint) == "" || !validHTTPURL(cfg.Watch.Embedding.Endpoint) {
			add("watch.embedding.endpoint", "must be a valid URL for the selected provider")
		}
		if strings.TrimSpace(cfg.Watch.Embedding.Model) == "" {
			add("watch.embedding.model", "must be non-empty for the selected provider")
		}
		if cfg.Watch.Embedding.HealthThreshold <= 0 || cfg.Watch.Embedding.HealthThreshold > 1 {
			add("watch.embedding.health_threshold", "must be greater than 0 and at most 1")
		}
	}
	return errs
}

func ResolveServeOptions(cfg *Config, flagHost, flagPort string) ServeConfig {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	out := cfg.Serve
	if flagHost != "" {
		out.Host = flagHost
	}
	if flagPort != "" {
		out.Port = flagPort
	}
	return out
}

func ResolveCompletionRemote() bool {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return false
	}
	return cfg.Completion.Remote
}

func ResolveWatchLayoutConfig() WatchLayoutConfig {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return DefaultConfig().Watch.Layout
	}
	return cfg.Watch.Layout
}

func FormatConfigValue(value any) string {
	switch v := value.(type) {
	case []string:
		return strings.Join(v, ",")
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

var configDefinitions = []ConfigDefinition{
	{Key: "server_url", Env: []string{"TLD_SERVER_URL"}, Description: "tlDiagram cloud/server URL used by sync commands."},
	{Key: "api_key", Env: []string{"TLD_API_KEY"}, Description: "API key used to authenticate with tlDiagram.", Secret: true},
	{Key: "org_id", Env: []string{"TLD_ORG_ID"}, Description: "Default tlDiagram organization/workspace identifier."},
	{Key: "validation.level", Description: "Architectural warning strictness: 1 minimal, 2 standard, 3 strict."},
	{Key: "validation.allow_low_insight", Description: "Allow low-insight generated warning groups."},
	{Key: "validation.include_rules", Description: "Additional architectural warning rule codes to include."},
	{Key: "validation.exclude_rules", Description: "Architectural warning rule codes to suppress."},
	{Key: "serve.host", Env: []string{"TLD_HOST", "TLD_ADDR"}, Description: "Host address for the local web server."},
	{Key: "serve.port", Env: []string{"PORT", "TLD_ADDR"}, Description: "Port for the local web server."},
	{Key: "serve.data_dir", Env: []string{"TLD_DATA_DIR"}, Description: "Directory for local database, logs, and pid files."},
	{Key: "watch.languages", Env: []string{"TLD_WATCH_LANGUAGES"}, Description: "Comma-separated source languages watched by analyze/watch."},
	{Key: "watch.watcher", Env: []string{"TLD_WATCH_WATCHER"}, Description: "File watcher backend: auto, fsnotify, or poll."},
	{Key: "watch.poll_interval", Env: []string{"TLD_WATCH_POLL_INTERVAL"}, Description: "Polling interval used by the poll watcher."},
	{Key: "watch.debounce", Env: []string{"TLD_WATCH_DEBOUNCE"}, Description: "Delay used to batch file changes before rescanning."},
	{Key: "watch.thresholds.max_elements_per_view", Description: "Maximum generated elements in a watch-created view."},
	{Key: "watch.thresholds.max_connectors_per_view", Description: "Maximum generated connectors in a watch-created view."},
	{Key: "watch.thresholds.max_incoming_per_element", Description: "Incoming reference limit before collapsing context."},
	{Key: "watch.thresholds.max_outgoing_per_element", Description: "Outgoing reference limit before collapsing context."},
	{Key: "watch.thresholds.max_expanded_connectors_per_group", Description: "File-pair connector expansion limit before folder-level collapse."},
	{Key: "watch.embedding.provider", Env: []string{"TLD_EMBEDDING_PROVIDER"}, Description: "Embedding provider for watch identity and similarity."},
	{Key: "watch.embedding.endpoint", Env: []string{"TLD_EMBEDDING_ENDPOINT"}, Description: "Embedding provider endpoint when the provider uses HTTP."},
	{Key: "watch.embedding.model", Env: []string{"TLD_EMBEDDING_MODEL"}, Description: "Embedding model name."},
	{Key: "watch.embedding.dimension", Env: []string{"TLD_EMBEDDING_DIMENSION"}, Description: "Embedding vector dimension, or 0 to infer when supported."},
	{Key: "watch.embedding.health_threshold", Description: "Similarity threshold required by embedding health checks."},
	{Key: "watch.layout.link_distance", Env: []string{"LAYOUT_LINK_DISTANCE"}, Description: "Organic layout target link distance for generated watch views."},
	{Key: "watch.layout.charge_strength", Env: []string{"LAYOUT_CHARGE_STRENGTH"}, Description: "Organic layout node charge strength for generated watch views."},
	{Key: "watch.layout.collide_radius", Env: []string{"LAYOUT_COLLIDE_RADIUS"}, Description: "Organic layout collision radius for generated watch views."},
	{Key: "watch.layout.gravity_strength", Env: []string{"LAYOUT_GRAVITY_STRENGTH"}, Description: "Organic layout gravity strength for generated watch views."},
	{Key: "completion.remote", Env: []string{"TLD_COMPLETION_REMOTE"}, Description: "Allow shell completion to query remote resources."},
}

func loadGlobalConfigState(repair bool) (*GlobalConfigState, error) {
	path, err := ConfigPath()
	if err != nil {
		return &GlobalConfigState{Config: DefaultConfig(), File: DefaultConfig(), Values: buildConfigValues(DefaultConfig(), nil, nil)}, nil
	}
	cfg := DefaultConfig()
	fileCfg := DefaultConfig()
	root, data, err := readConfigNode(path)
	if err != nil {
		if os.IsNotExist(err) {
			if repair {
				_ = SaveGlobalConfig(cfg)
			}
			values, applyErr := applyEnvOverridesDetailed(cfg, root)
			return &GlobalConfigState{Path: path, Config: cfg, File: fileCfg, Values: values, FileRoot: root}, applyErr
		}
		return nil, err
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, fileCfg); err != nil {
			return nil, fmt.Errorf("parse global config: %w", err)
		}
		*cfg = *fileCfg
	}
	if repair && shouldSaveConfig(root) {
		_ = SaveGlobalConfigPreservingUnknown(fileCfg, root)
	}
	values, err := applyEnvOverridesDetailed(cfg, root)
	if err != nil {
		return nil, err
	}
	return &GlobalConfigState{Path: path, Config: cfg, File: fileCfg, Values: values, FileRoot: root}, nil
}

func readConfigNode(path string) (*yaml.Node, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, data, fmt.Errorf("parse global config: %w", err)
	}
	if root.Kind == 0 {
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, data, fmt.Errorf("parse global config: expected mapping document")
	}
	return &root, data, nil
}

func emptyConfigNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
}

func applyEnvOverridesDetailed(cfg *Config, root *yaml.Node) ([]ConfigValue, error) {
	sources := map[string]ConfigSource{}
	envSources := map[string]string{}
	for _, def := range configDefinitions {
		if hasYAMLPath(root, def.Key) {
			sources[def.Key] = ConfigSourceFile
		} else {
			sources[def.Key] = ConfigSourceDefault
		}
	}
	apply := func(key, env, value string) error {
		if value == "" {
			return nil
		}
		if err := setConfigValue(cfg, key, value); err != nil {
			return fmt.Errorf("%s from %s: %w", key, env, err)
		}
		sources[key] = ConfigSourceEnv
		envSources[key] = env
		return nil
	}
	for _, item := range []struct {
		key string
		env string
	}{
		{"server_url", "TLD_SERVER_URL"},
		{"api_key", "TLD_API_KEY"},
		{"org_id", "TLD_ORG_ID"},
		{"serve.host", "TLD_HOST"},
		{"serve.port", "PORT"},
		{"serve.data_dir", "TLD_DATA_DIR"},
		{"watch.languages", "TLD_WATCH_LANGUAGES"},
		{"watch.watcher", "TLD_WATCH_WATCHER"},
		{"watch.poll_interval", "TLD_WATCH_POLL_INTERVAL"},
		{"watch.debounce", "TLD_WATCH_DEBOUNCE"},
		{"watch.embedding.provider", "TLD_EMBEDDING_PROVIDER"},
		{"watch.embedding.endpoint", "TLD_EMBEDDING_ENDPOINT"},
		{"watch.embedding.model", "TLD_EMBEDDING_MODEL"},
		{"watch.embedding.dimension", "TLD_EMBEDDING_DIMENSION"},
		{"watch.layout.link_distance", "LAYOUT_LINK_DISTANCE"},
		{"watch.layout.charge_strength", "LAYOUT_CHARGE_STRENGTH"},
		{"watch.layout.collide_radius", "LAYOUT_COLLIDE_RADIUS"},
		{"watch.layout.gravity_strength", "LAYOUT_GRAVITY_STRENGTH"},
	} {
		if err := apply(item.key, item.env, os.Getenv(item.env)); err != nil {
			return nil, err
		}
	}
	if v := os.Getenv("TLD_COMPLETION_REMOTE"); v != "" {
		if err := apply("completion.remote", "TLD_COMPLETION_REMOTE", v); err != nil {
			return nil, err
		}
	}
	if addr := strings.TrimSpace(os.Getenv("TLD_ADDR")); addr != "" {
		host, port, err := splitAddrOverride(addr)
		if err != nil {
			return nil, fmt.Errorf("serve.host/serve.port from TLD_ADDR: %w", err)
		}
		if host != "" {
			if err := setConfigValue(cfg, "serve.host", host); err != nil {
				return nil, err
			}
			sources["serve.host"] = ConfigSourceEnv
			envSources["serve.host"] = "TLD_ADDR"
		}
		if port != "" {
			if err := setConfigValue(cfg, "serve.port", port); err != nil {
				return nil, err
			}
			sources["serve.port"] = ConfigSourceEnv
			envSources["serve.port"] = "TLD_ADDR"
		}
	}
	if errs := ValidateGlobalConfig(cfg); len(errs) > 0 {
		return nil, errs
	}
	return buildConfigValues(cfg, sources, envSources), nil
}

func buildConfigValues(cfg *Config, sources map[string]ConfigSource, envSources map[string]string) []ConfigValue {
	values := make([]ConfigValue, 0, len(configDefinitions))
	for _, def := range configDefinitions {
		source := sources[def.Key]
		if source == "" {
			source = ConfigSourceDefault
		}
		values = append(values, ConfigValue{
			Key:         def.Key,
			Value:       FormatConfigValue(getConfigValue(cfg, def.Key)),
			Source:      source,
			Env:         configValueEnv(def, envSources[def.Key]),
			Description: def.Description,
			Secret:      def.Secret,
		})
	}
	return values
}

func configValueEnv(def ConfigDefinition, active string) string {
	if active != "" {
		return active
	}
	return strings.Join(def.Env, ",")
}

func setConfigValue(cfg *Config, key, value string) error {
	key = normalizeConfigKey(key)
	switch key {
	case "server_url":
		cfg.ServerURL = strings.TrimSpace(value)
	case "api_key":
		cfg.APIKey = value
	case "org_id":
		cfg.WorkspaceID = strings.TrimSpace(value)
	case "validation.level":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Validation.Level = v
	case "validation.allow_low_insight":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Validation.AllowLowInsight = v
	case "validation.include_rules":
		cfg.Validation.IncludeRules = parseStringList(value)
	case "validation.exclude_rules":
		cfg.Validation.ExcludeRules = parseStringList(value)
	case "serve.host":
		cfg.Serve.Host = strings.TrimSpace(value)
	case "serve.port":
		cfg.Serve.Port = strings.TrimSpace(value)
	case "serve.data_dir":
		cfg.Serve.DataDir = strings.TrimSpace(value)
	case "watch.languages":
		cfg.Watch.Languages = parseStringList(value)
	case "watch.watcher":
		cfg.Watch.Watcher = strings.ToLower(strings.TrimSpace(value))
	case "watch.poll_interval":
		cfg.Watch.PollInterval = strings.TrimSpace(value)
	case "watch.debounce":
		cfg.Watch.Debounce = strings.TrimSpace(value)
	case "watch.thresholds.max_elements_per_view":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Watch.Thresholds.MaxElementsPerView = v
	case "watch.thresholds.max_connectors_per_view":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Watch.Thresholds.MaxConnectorsPerView = v
	case "watch.thresholds.max_incoming_per_element":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Watch.Thresholds.MaxIncomingPerElement = v
	case "watch.thresholds.max_outgoing_per_element":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Watch.Thresholds.MaxOutgoingPerElement = v
	case "watch.thresholds.max_expanded_connectors_per_group":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup = v
	case "watch.embedding.provider":
		cfg.Watch.Embedding.Provider = strings.TrimSpace(value)
	case "watch.embedding.endpoint":
		cfg.Watch.Embedding.Endpoint = strings.TrimSpace(value)
	case "watch.embedding.model":
		cfg.Watch.Embedding.Model = strings.TrimSpace(value)
	case "watch.embedding.dimension":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Watch.Embedding.Dimension = v
	case "watch.embedding.health_threshold":
		v, err := parseFloat(value)
		if err != nil {
			return err
		}
		cfg.Watch.Embedding.HealthThreshold = v
	case "watch.layout.link_distance":
		v, err := parseFloat(value)
		if err != nil {
			return err
		}
		cfg.Watch.Layout.LinkDistance = v
	case "watch.layout.charge_strength":
		v, err := parseFloat(value)
		if err != nil {
			return err
		}
		cfg.Watch.Layout.ChargeStrength = v
	case "watch.layout.collide_radius":
		v, err := parseFloat(value)
		if err != nil {
			return err
		}
		cfg.Watch.Layout.CollideRadius = v
	case "watch.layout.gravity_strength":
		v, err := parseFloat(value)
		if err != nil {
			return err
		}
		cfg.Watch.Layout.GravityStrength = v
	case "completion.remote":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Completion.Remote = v
	default:
		return fmt.Errorf("unknown global config key %q", key)
	}
	return nil
}

func getConfigValue(cfg *Config, key string) any {
	switch normalizeConfigKey(key) {
	case "server_url":
		return cfg.ServerURL
	case "api_key":
		return cfg.APIKey
	case "org_id":
		return cfg.WorkspaceID
	case "validation.level":
		return cfg.Validation.Level
	case "validation.allow_low_insight":
		return cfg.Validation.AllowLowInsight
	case "validation.include_rules":
		return cfg.Validation.IncludeRules
	case "validation.exclude_rules":
		return cfg.Validation.ExcludeRules
	case "serve.host":
		return cfg.Serve.Host
	case "serve.port":
		return cfg.Serve.Port
	case "serve.data_dir":
		return cfg.Serve.DataDir
	case "watch.languages":
		return cfg.Watch.Languages
	case "watch.watcher":
		return cfg.Watch.Watcher
	case "watch.poll_interval":
		return cfg.Watch.PollInterval
	case "watch.debounce":
		return cfg.Watch.Debounce
	case "watch.thresholds.max_elements_per_view":
		return cfg.Watch.Thresholds.MaxElementsPerView
	case "watch.thresholds.max_connectors_per_view":
		return cfg.Watch.Thresholds.MaxConnectorsPerView
	case "watch.thresholds.max_incoming_per_element":
		return cfg.Watch.Thresholds.MaxIncomingPerElement
	case "watch.thresholds.max_outgoing_per_element":
		return cfg.Watch.Thresholds.MaxOutgoingPerElement
	case "watch.thresholds.max_expanded_connectors_per_group":
		return cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup
	case "watch.embedding.provider":
		return cfg.Watch.Embedding.Provider
	case "watch.embedding.endpoint":
		return cfg.Watch.Embedding.Endpoint
	case "watch.embedding.model":
		return cfg.Watch.Embedding.Model
	case "watch.embedding.dimension":
		return cfg.Watch.Embedding.Dimension
	case "watch.embedding.health_threshold":
		return cfg.Watch.Embedding.HealthThreshold
	case "watch.layout.link_distance":
		return cfg.Watch.Layout.LinkDistance
	case "watch.layout.charge_strength":
		return cfg.Watch.Layout.ChargeStrength
	case "watch.layout.collide_radius":
		return cfg.Watch.Layout.CollideRadius
	case "watch.layout.gravity_strength":
		return cfg.Watch.Layout.GravityStrength
	case "completion.remote":
		return cfg.Completion.Remote
	default:
		return ""
	}
}

func configToYAMLNode(cfg *Config, existingRoot *yaml.Node) *yaml.Node {
	var existing *yaml.Node
	if existingRoot != nil && len(existingRoot.Content) > 0 {
		existing = existingRoot.Content[0]
	}
	root := &yaml.Node{Kind: yaml.DocumentNode}
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = []*yaml.Node{mapping}

	addScalar(mapping, "server_url", cfg.ServerURL, desc("server_url"))
	addScalar(mapping, "api_key", cfg.APIKey, desc("api_key"))
	addScalar(mapping, "org_id", cfg.WorkspaceID, desc("org_id"))

	validation := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(validation, "level", cfg.Validation.Level, desc("validation.level"))
	addScalar(validation, "allow_low_insight", cfg.Validation.AllowLowInsight, desc("validation.allow_low_insight"))
	addStringSeq(validation, "include_rules", cfg.Validation.IncludeRules, desc("validation.include_rules"))
	addStringSeq(validation, "exclude_rules", cfg.Validation.ExcludeRules, desc("validation.exclude_rules"))
	appendUnknownEntries(validation, mappingValueNode(existing, "validation"), setOf("level", "allow_low_insight", "include_rules", "exclude_rules"))
	addMap(mapping, "validation", validation, "Workspace validation and architectural warning settings.")

	serve := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(serve, "host", cfg.Serve.Host, desc("serve.host"))
	addScalar(serve, "port", cfg.Serve.Port, desc("serve.port"))
	addScalar(serve, "data_dir", cfg.Serve.DataDir, desc("serve.data_dir"))
	appendUnknownEntries(serve, mappingValueNode(existing, "serve"), setOf("host", "port", "data_dir"))
	addMap(mapping, "serve", serve, "Local web server settings.")

	watchNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addStringSeq(watchNode, "languages", cfg.Watch.Languages, desc("watch.languages"))
	addScalar(watchNode, "watcher", cfg.Watch.Watcher, desc("watch.watcher"))
	addScalar(watchNode, "poll_interval", cfg.Watch.PollInterval, desc("watch.poll_interval"))
	addScalar(watchNode, "debounce", cfg.Watch.Debounce, desc("watch.debounce"))

	thresholds := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(thresholds, "max_elements_per_view", cfg.Watch.Thresholds.MaxElementsPerView, desc("watch.thresholds.max_elements_per_view"))
	addScalar(thresholds, "max_connectors_per_view", cfg.Watch.Thresholds.MaxConnectorsPerView, desc("watch.thresholds.max_connectors_per_view"))
	addScalar(thresholds, "max_incoming_per_element", cfg.Watch.Thresholds.MaxIncomingPerElement, desc("watch.thresholds.max_incoming_per_element"))
	addScalar(thresholds, "max_outgoing_per_element", cfg.Watch.Thresholds.MaxOutgoingPerElement, desc("watch.thresholds.max_outgoing_per_element"))
	addScalar(thresholds, "max_expanded_connectors_per_group", cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup, desc("watch.thresholds.max_expanded_connectors_per_group"))
	appendUnknownEntries(thresholds, mappingValueNode(mappingValueNode(existing, "watch"), "thresholds"), setOf("max_elements_per_view", "max_connectors_per_view", "max_incoming_per_element", "max_outgoing_per_element", "max_expanded_connectors_per_group"))
	addMap(watchNode, "thresholds", thresholds, "Limits used while materializing generated watch views.")

	embedding := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(embedding, "provider", cfg.Watch.Embedding.Provider, desc("watch.embedding.provider"))
	addScalar(embedding, "endpoint", cfg.Watch.Embedding.Endpoint, desc("watch.embedding.endpoint"))
	addScalar(embedding, "model", cfg.Watch.Embedding.Model, desc("watch.embedding.model"))
	addScalar(embedding, "dimension", cfg.Watch.Embedding.Dimension, desc("watch.embedding.dimension"))
	addScalar(embedding, "health_threshold", cfg.Watch.Embedding.HealthThreshold, desc("watch.embedding.health_threshold"))
	appendUnknownEntries(embedding, mappingValueNode(mappingValueNode(existing, "watch"), "embedding"), setOf("provider", "endpoint", "model", "dimension", "health_threshold"))
	addMap(watchNode, "embedding", embedding, "Embedding settings used by watch/analyze identity matching.")

	layout := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(layout, "link_distance", cfg.Watch.Layout.LinkDistance, desc("watch.layout.link_distance"))
	addScalar(layout, "charge_strength", cfg.Watch.Layout.ChargeStrength, desc("watch.layout.charge_strength"))
	addScalar(layout, "collide_radius", cfg.Watch.Layout.CollideRadius, desc("watch.layout.collide_radius"))
	addScalar(layout, "gravity_strength", cfg.Watch.Layout.GravityStrength, desc("watch.layout.gravity_strength"))
	appendUnknownEntries(layout, mappingValueNode(mappingValueNode(existing, "watch"), "layout"), setOf("link_distance", "charge_strength", "collide_radius", "gravity_strength"))
	addMap(watchNode, "layout", layout, "Organic layout tuning for generated watch views.")

	appendUnknownEntries(watchNode, mappingValueNode(existing, "watch"), setOf("languages", "watcher", "poll_interval", "debounce", "thresholds", "embedding", "layout"))
	addMap(mapping, "watch", watchNode, "Source watch/analyze pipeline settings.")

	completion := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(completion, "remote", cfg.Completion.Remote, desc("completion.remote"))
	appendUnknownEntries(completion, mappingValueNode(existing, "completion"), setOf("remote"))
	addMap(mapping, "completion", completion, "Shell completion settings.")

	appendUnknownEntries(mapping, existing, setOf("server_url", "api_key", "org_id", "validation", "serve", "watch", "completion"))
	return root
}

func addMap(mapping *yaml.Node, key string, value *yaml.Node, comment string) {
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, HeadComment: comment}, value)
}

func addStringSeq(mapping *yaml.Node, key string, values []string, comment string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, HeadComment: comment}, seq)
}

func addScalar(mapping *yaml.Node, key string, value any, comment string) {
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, HeadComment: comment}, scalarNode(value))
}

func scalarNode(value any) *yaml.Node {
	switch v := value.(type) {
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
	case float64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(v, 'f', -1, 64)}
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprint(v)}
	}
}

func appendUnknownEntries(dst, src *yaml.Node, known map[string]struct{}) {
	if src == nil || src.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		key := src.Content[i].Value
		if _, ok := known[key]; ok {
			continue
		}
		dst.Content = append(dst.Content, cloneYAMLNode(src.Content[i]), cloneYAMLNode(src.Content[i+1]))
	}
}

func shouldSaveConfig(root *yaml.Node) bool {
	if root == nil {
		return true
	}
	for _, def := range configDefinitions {
		if !hasYAMLPath(root, def.Key) {
			return true
		}
	}
	return false
}

func hasYAMLPath(root *yaml.Node, dotted string) bool {
	if root == nil || len(root.Content) == 0 {
		return false
	}
	node := root.Content[0]
	for _, part := range strings.Split(dotted, ".") {
		if node == nil || node.Kind != yaml.MappingNode {
			return false
		}
		node = mappingValueNode(node, part)
		if node == nil {
			return false
		}
	}
	return true
}

func desc(key string) string {
	if def, ok := ConfigDefinitionForKey(key); ok {
		return def.Description
	}
	return ""
}

func normalizeConfigKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func parseStringList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("must be a boolean")
	}
}

func parseInt(value string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	return v, nil
}

func parseFloat(value string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("must be a number")
	}
	return v, nil
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func validPort(value string) bool {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	return err == nil && port >= 1 && port <= 65535
}

func expandConfigPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("user home dir: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	return filepath.Abs(path)
}

func splitAddrOverride(addr string) (host string, port string, err error) {
	if strings.Count(addr, ":") == 1 {
		parts := strings.Split(addr, ":")
		return parts[0], parts[1], nil
	}
	if strings.Contains(addr, ":") {
		h, p, err := net.SplitHostPort(addr)
		if err != nil {
			return "", "", err
		}
		return h, p, nil
	}
	return addr, "", nil
}

func normalizeConfigLanguages(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		lang := strings.ToLower(strings.TrimSpace(value))
		if lang == "" {
			continue
		}
		if _, ok := analyzer.LanguageSpecFor(analyzer.Language(lang)); ok {
			seen[lang] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for lang := range seen {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

func setOf(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
