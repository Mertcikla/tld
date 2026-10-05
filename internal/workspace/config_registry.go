package workspace

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	return loadGlobalConfigState(false)
}

func LoadGlobalConfigStateNoRepair() (*GlobalConfigState, error) {
	return loadGlobalConfigState(false)
}

func SetGlobalConfigValue(key, value string) error {
	key = normalizeConfigKey(key)
	if _, ok := ConfigDefinitionForKey(key); !ok {
		return fmt.Errorf("unknown global config key %q", key)
	}
	path, err := ResolveConfigPath()
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
		readPath, readErr := ResolveConfigPath()
		if readErr == nil {
			existingRoot, _, _ = readConfigNode(readPath)
		}
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
	switch strings.ToLower(strings.TrimSpace(cfg.Apply.Target)) {
	case "", "auto", "local", "remote":
	default:
		add("apply.target", "must be auto, local, or remote")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Database.Driver)) {
	case "", "sqlite", "postgres", "postgresql":
	default:
		add("database.driver", "must be sqlite or postgres")
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgres") || strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgresql") {
		if strings.TrimSpace(cfg.Database.DatabaseURL) == "" {
			add("database.url", "must be set when database.driver is postgres")
		}
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
	if strings.TrimSpace(cfg.Serve.PublicURL) != "" && !validRootHTTPURL(cfg.Serve.PublicURL) {
		add("serve.public_url", "must be an http or https root URL")
	}
	for _, origin := range cfg.Serve.AllowedOrigins {
		if !validHTTPOrigin(origin) {
			add("serve.allowed_origins", "entries must be http or https origins without a path")
			break
		}
	}
	if d, err := time.ParseDuration(strings.TrimSpace(cfg.Updates.CheckInterval)); err != nil || d <= 0 {
		add("updates.check_interval", "must be a positive duration such as 24h")
	}

	if cfg.Index.Tools.TimeoutSeconds < 0 {
		add("index.tools.timeout_seconds", "must be non-negative")
	}
	if cfg.Map.Grouping.Resolution <= 0 {
		add("map.grouping.resolution", "must be positive")
	}
	if cfg.Map.Grouping.MinGroupSize < 1 {
		add("map.grouping.min_group_size", "must be at least 1")
	}
	if cfg.Map.Grouping.MinRootGroups < 1 {
		add("map.grouping.min_root_groups", "must be at least 1")
	}
	if cfg.Map.Grouping.MaxRootGroups < cfg.Map.Grouping.MinRootGroups {
		add("map.grouping.max_root_groups", "must be at least min_root_groups")
	}
	if cfg.Map.Grouping.MaxChildren < 2 {
		add("map.grouping.max_children", "must be at least 2")
	}
	if cfg.Map.Grouping.MaxDepth < 0 {
		add("map.grouping.max_depth", "must be non-negative")
	}
	if cfg.Map.Grouping.MaxLeafFiles < 1 {
		add("map.grouping.max_leaf_files", "must be at least 1")
	}
	if cfg.Map.Budget.MaxConnectorsPerView < 1 {
		add("map.budget.max_connectors_per_view", "must be at least 1")
	}
	if cfg.Map.Budget.MaxLeafConnectorsPerView < 1 {
		add("map.budget.max_leaf_connectors_per_view", "must be at least 1")
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
	{Key: "apply.target", Env: []string{"TLD_APPLY_TARGET"}, Description: "Default apply target: auto, local, or remote."},
	{Key: "database.driver", Env: []string{"TLD_DB_DRIVER"}, Description: "Local database driver: sqlite or postgres."},
	{Key: "database.url", Env: []string{"TLD_DATABASE_URL"}, Description: "PostgreSQL database URL for local database mode.", Secret: true},
	{Key: "validation.level", Description: "Architectural warning strictness: 1 minimal, 2 standard, 3 strict."},
	{Key: "validation.allow_low_insight", Description: "Allow low-insight generated warning groups."},
	{Key: "validation.include_rules", Description: "Additional architectural warning rule codes to include."},
	{Key: "validation.exclude_rules", Description: "Architectural warning rule codes to suppress."},
	{Key: "serve.host", Env: []string{"TLD_HOST", "TLD_ADDR"}, Description: "Host address for the local web server."},
	{Key: "serve.port", Env: []string{"PORT", "TLD_ADDR"}, Description: "Port for the local web server."},
	{Key: "serve.data_dir", Env: []string{"TLD_DATA_DIR"}, Description: "Directory for local database and logs."},
	{Key: "serve.public_url", Env: []string{"TLD_PUBLIC_URL"}, Description: "Public root URL for reverse-proxied self-hosted deployments."},
	{Key: "serve.allowed_origins", Env: []string{"TLD_ALLOWED_ORIGINS"}, Description: "Additional comma-separated HTTP(S) origins allowed by local server CORS."},
	{Key: "index.tools.scip_go", Env: []string{"TLD_INDEX_SCIP_GO"}, Description: "Path or name of the scip-go indexer."},
	{Key: "index.tools.scip_typescript", Env: []string{"TLD_INDEX_SCIP_TYPESCRIPT"}, Description: "Path or name of the scip-typescript indexer."},
	{Key: "index.tools.scip_python", Env: []string{"TLD_INDEX_SCIP_PYTHON"}, Description: "Path or name of the scip-python indexer."},
	{Key: "index.tools.scip_dotnet", Env: []string{"TLD_INDEX_SCIP_DOTNET"}, Description: "Path or name of the scip-dotnet indexer."},
	{Key: "index.tools.scip_clang", Env: []string{"TLD_INDEX_SCIP_CLANG"}, Description: "Path or name of the scip-clang indexer."},
	{Key: "index.tools.scip_java", Env: []string{"TLD_INDEX_SCIP_JAVA"}, Description: "Path or name of the scip-java indexer."},
	{Key: "index.tools.scip_dart", Env: []string{"TLD_INDEX_SCIP_DART"}, Description: "Path or name of the scip-dart indexer."},
	{Key: "index.tools.scip_php", Env: []string{"TLD_INDEX_SCIP_PHP"}, Description: "Path or name of the scip-php indexer."},
	{Key: "index.tools.scip_ruby", Env: []string{"TLD_INDEX_SCIP_RUBY"}, Description: "Path or name of the scip-ruby indexer."},
	{Key: "index.tools.rust_analyzer", Env: []string{"TLD_INDEX_RUST_ANALYZER"}, Description: "Path or name of the rust-analyzer binary used for SCIP."},
	{Key: "index.tools.timeout_seconds", Env: []string{"TLD_INDEX_TOOL_TIMEOUT_SECONDS"}, Description: "Per-indexer invocation timeout in seconds."},
	{Key: "map.grouping.resolution", Env: []string{"TLD_MAP_GROUPING_RESOLUTION"}, Description: "Louvain resolution: higher values produce more, smaller communities."},
	{Key: "map.grouping.min_group_size", Env: []string{"TLD_MAP_GROUPING_MIN_GROUP_SIZE"}, Description: "Merge leaf groups smaller than this into their strongest sibling."},
	{Key: "map.grouping.min_root_groups", Env: []string{"TLD_MAP_GROUPING_MIN_ROOT_GROUPS"}, Description: "Minimum number of root components in a graph map."},
	{Key: "map.grouping.max_root_groups", Env: []string{"TLD_MAP_GROUPING_MAX_ROOT_GROUPS"}, Description: "Maximum number of root components in a graph map."},
	{Key: "map.grouping.max_children", Env: []string{"TLD_MAP_GROUPING_MAX_CHILDREN"}, Description: "Maximum fan-out of one map subdivision."},
	{Key: "map.grouping.max_depth", Env: []string{"TLD_MAP_GROUPING_MAX_DEPTH"}, Description: "Maximum depth of the map group hierarchy."},
	{Key: "map.grouping.max_leaf_files", Env: []string{"TLD_MAP_GROUPING_MAX_LEAF_FILES"}, Description: "Files per leaf view before the group is subdivided."},
	{Key: "map.budget.max_connectors_per_view", Env: []string{"TLD_MAP_BUDGET_MAX_CONNECTORS_PER_VIEW"}, Description: "Maximum rolled-up connectors drawn in one map view."},
	{Key: "map.budget.max_leaf_connectors_per_view", Env: []string{"TLD_MAP_BUDGET_MAX_LEAF_CONNECTORS_PER_VIEW"}, Description: "Maximum connectors drawn in a file-only map view."},
	{Key: "completion.remote", Env: []string{"TLD_COMPLETION_REMOTE"}, Description: "Allow shell completion to query remote resources."},
	{Key: "updates.auto", Env: []string{"TLD_UPDATES_AUTO"}, Description: "Automatically install available tld CLI updates during startup checks."},
	{Key: "updates.check_interval", Env: []string{"TLD_UPDATES_CHECK_INTERVAL"}, Description: "Minimum time between GitHub update checks."},
}

func loadGlobalConfigState(repair bool) (*GlobalConfigState, error) {
	path, err := ResolveConfigPath()
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
		normalizeConfig(fileCfg)
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
		{"apply.target", "TLD_APPLY_TARGET"},
		{"database.driver", "TLD_DB_DRIVER"},
		{"database.url", "TLD_DATABASE_URL"},
		{"serve.host", "TLD_HOST"},
		{"serve.port", "PORT"},
		{"serve.data_dir", "TLD_DATA_DIR"},
		{"serve.public_url", "TLD_PUBLIC_URL"},
		{"serve.allowed_origins", "TLD_ALLOWED_ORIGINS"},
		{"index.tools.scip_go", "TLD_INDEX_SCIP_GO"},
		{"index.tools.scip_typescript", "TLD_INDEX_SCIP_TYPESCRIPT"},
		{"index.tools.scip_python", "TLD_INDEX_SCIP_PYTHON"},
		{"index.tools.scip_dotnet", "TLD_INDEX_SCIP_DOTNET"},
		{"index.tools.scip_clang", "TLD_INDEX_SCIP_CLANG"},
		{"index.tools.scip_java", "TLD_INDEX_SCIP_JAVA"},
		{"index.tools.scip_dart", "TLD_INDEX_SCIP_DART"},
		{"index.tools.scip_php", "TLD_INDEX_SCIP_PHP"},
		{"index.tools.scip_ruby", "TLD_INDEX_SCIP_RUBY"},
		{"index.tools.rust_analyzer", "TLD_INDEX_RUST_ANALYZER"},
		{"index.tools.timeout_seconds", "TLD_INDEX_TOOL_TIMEOUT_SECONDS"},
		{"map.grouping.resolution", "TLD_MAP_GROUPING_RESOLUTION"},
		{"map.grouping.min_group_size", "TLD_MAP_GROUPING_MIN_GROUP_SIZE"},
		{"map.grouping.min_root_groups", "TLD_MAP_GROUPING_MIN_ROOT_GROUPS"},
		{"map.grouping.max_root_groups", "TLD_MAP_GROUPING_MAX_ROOT_GROUPS"},
		{"map.grouping.max_children", "TLD_MAP_GROUPING_MAX_CHILDREN"},
		{"map.grouping.max_depth", "TLD_MAP_GROUPING_MAX_DEPTH"},
		{"map.grouping.max_leaf_files", "TLD_MAP_GROUPING_MAX_LEAF_FILES"},
		{"map.budget.max_connectors_per_view", "TLD_MAP_BUDGET_MAX_CONNECTORS_PER_VIEW"},
		{"map.budget.max_leaf_connectors_per_view", "TLD_MAP_BUDGET_MAX_LEAF_CONNECTORS_PER_VIEW"},
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
	if v := os.Getenv("TLD_UPDATES_AUTO"); v != "" {
		if err := apply("updates.auto", "TLD_UPDATES_AUTO", v); err != nil {
			return nil, err
		}
	}
	if v := os.Getenv("TLD_UPDATES_CHECK_INTERVAL"); v != "" {
		if err := apply("updates.check_interval", "TLD_UPDATES_CHECK_INTERVAL", v); err != nil {
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
	case "apply.target":
		cfg.Apply.Target = strings.ToLower(strings.TrimSpace(value))
	case "database.driver":
		cfg.Database.Driver = strings.ToLower(strings.TrimSpace(value))
	case "database.url":
		cfg.Database.DatabaseURL = strings.TrimSpace(value)
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
	case "serve.public_url":
		cfg.Serve.PublicURL = normalizePublicURLValue(value)
	case "serve.allowed_origins":
		cfg.Serve.AllowedOrigins = parseStringList(value)
	case "index.tools.scip_go":
		cfg.Index.Tools.SCIPGo = strings.TrimSpace(value)
	case "index.tools.scip_typescript":
		cfg.Index.Tools.SCIPTypeScript = strings.TrimSpace(value)
	case "index.tools.scip_python":
		cfg.Index.Tools.SCIPPython = strings.TrimSpace(value)
	case "index.tools.scip_dotnet":
		cfg.Index.Tools.SCIPDotnet = strings.TrimSpace(value)
	case "index.tools.scip_clang":
		cfg.Index.Tools.SCIPClang = strings.TrimSpace(value)
	case "index.tools.scip_java":
		cfg.Index.Tools.SCIPJava = strings.TrimSpace(value)
	case "index.tools.scip_dart":
		cfg.Index.Tools.SCIPDart = strings.TrimSpace(value)
	case "index.tools.scip_php":
		cfg.Index.Tools.SCIPPhp = strings.TrimSpace(value)
	case "index.tools.scip_ruby":
		cfg.Index.Tools.SCIPRuby = strings.TrimSpace(value)
	case "index.tools.rust_analyzer":
		cfg.Index.Tools.RustAnalyzer = strings.TrimSpace(value)
	case "index.tools.timeout_seconds":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Index.Tools.TimeoutSeconds = v
	case "map.grouping.resolution":
		v, err := parseFloat(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.Resolution = v
	case "map.grouping.min_group_size":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.MinGroupSize = v
	case "map.grouping.min_root_groups":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.MinRootGroups = v
	case "map.grouping.max_root_groups":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.MaxRootGroups = v
	case "map.grouping.max_children":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.MaxChildren = v
	case "map.grouping.max_depth":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.MaxDepth = v
	case "map.grouping.max_leaf_files":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Grouping.MaxLeafFiles = v
	case "map.budget.max_connectors_per_view":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Budget.MaxConnectorsPerView = v
	case "map.budget.max_leaf_connectors_per_view":
		v, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.Map.Budget.MaxLeafConnectorsPerView = v
	case "completion.remote":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Completion.Remote = v
	case "updates.auto":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Updates.Auto = v
	case "updates.check_interval":
		cfg.Updates.CheckInterval = strings.TrimSpace(value)
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
	case "apply.target":
		return cfg.Apply.Target
	case "database.driver":
		return cfg.Database.Driver
	case "database.url":
		return cfg.Database.DatabaseURL
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
	case "serve.public_url":
		return cfg.Serve.PublicURL
	case "serve.allowed_origins":
		return cfg.Serve.AllowedOrigins
	case "index.tools.scip_go":
		return cfg.Index.Tools.SCIPGo
	case "index.tools.scip_typescript":
		return cfg.Index.Tools.SCIPTypeScript
	case "index.tools.scip_python":
		return cfg.Index.Tools.SCIPPython
	case "index.tools.scip_dotnet":
		return cfg.Index.Tools.SCIPDotnet
	case "index.tools.scip_clang":
		return cfg.Index.Tools.SCIPClang
	case "index.tools.scip_java":
		return cfg.Index.Tools.SCIPJava
	case "index.tools.scip_dart":
		return cfg.Index.Tools.SCIPDart
	case "index.tools.scip_php":
		return cfg.Index.Tools.SCIPPhp
	case "index.tools.scip_ruby":
		return cfg.Index.Tools.SCIPRuby
	case "index.tools.rust_analyzer":
		return cfg.Index.Tools.RustAnalyzer
	case "index.tools.timeout_seconds":
		return cfg.Index.Tools.TimeoutSeconds
	case "map.grouping.resolution":
		return cfg.Map.Grouping.Resolution
	case "map.grouping.min_group_size":
		return cfg.Map.Grouping.MinGroupSize
	case "map.grouping.min_root_groups":
		return cfg.Map.Grouping.MinRootGroups
	case "map.grouping.max_root_groups":
		return cfg.Map.Grouping.MaxRootGroups
	case "map.grouping.max_children":
		return cfg.Map.Grouping.MaxChildren
	case "map.grouping.max_depth":
		return cfg.Map.Grouping.MaxDepth
	case "map.grouping.max_leaf_files":
		return cfg.Map.Grouping.MaxLeafFiles
	case "map.budget.max_connectors_per_view":
		return cfg.Map.Budget.MaxConnectorsPerView
	case "map.budget.max_leaf_connectors_per_view":
		return cfg.Map.Budget.MaxLeafConnectorsPerView
	case "completion.remote":
		return cfg.Completion.Remote
	case "updates.auto":
		return cfg.Updates.Auto
	case "updates.check_interval":
		return cfg.Updates.CheckInterval
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

	apply := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(apply, "target", cfg.Apply.Target, desc("apply.target"))
	appendUnknownEntries(apply, mappingValueNode(existing, "apply"), setOf("target"))
	addMap(mapping, "apply", apply, "CLI apply target settings.")

	database := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(database, "driver", cfg.Database.Driver, desc("database.driver"))
	addScalar(database, "url", cfg.Database.DatabaseURL, desc("database.url"))
	appendUnknownEntries(database, mappingValueNode(existing, "database"), setOf("driver", "url"))
	addMap(mapping, "database", database, "Local database settings.")

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
	addScalar(serve, "public_url", cfg.Serve.PublicURL, desc("serve.public_url"))
	addStringSeq(serve, "allowed_origins", cfg.Serve.AllowedOrigins, desc("serve.allowed_origins"))
	appendUnknownEntries(serve, mappingValueNode(existing, "serve"), setOf("host", "port", "data_dir", "public_url", "allowed_origins"))
	addMap(mapping, "serve", serve, "Local web server settings.")

	indexNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	indexTools := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(indexTools, "scip_go", cfg.Index.Tools.SCIPGo, desc("index.tools.scip_go"))
	addScalar(indexTools, "scip_typescript", cfg.Index.Tools.SCIPTypeScript, desc("index.tools.scip_typescript"))
	addScalar(indexTools, "scip_python", cfg.Index.Tools.SCIPPython, desc("index.tools.scip_python"))
	addScalar(indexTools, "scip_dotnet", cfg.Index.Tools.SCIPDotnet, desc("index.tools.scip_dotnet"))
	addScalar(indexTools, "scip_clang", cfg.Index.Tools.SCIPClang, desc("index.tools.scip_clang"))
	addScalar(indexTools, "scip_java", cfg.Index.Tools.SCIPJava, desc("index.tools.scip_java"))
	addScalar(indexTools, "scip_dart", cfg.Index.Tools.SCIPDart, desc("index.tools.scip_dart"))
	addScalar(indexTools, "scip_php", cfg.Index.Tools.SCIPPhp, desc("index.tools.scip_php"))
	addScalar(indexTools, "scip_ruby", cfg.Index.Tools.SCIPRuby, desc("index.tools.scip_ruby"))
	addScalar(indexTools, "rust_analyzer", cfg.Index.Tools.RustAnalyzer, desc("index.tools.rust_analyzer"))
	addScalar(indexTools, "timeout_seconds", cfg.Index.Tools.TimeoutSeconds, desc("index.tools.timeout_seconds"))
	appendUnknownEntries(indexTools, mappingValueNode(mappingValueNode(existing, "index"), "tools"), setOf("scip_go", "scip_typescript", "scip_python", "scip_dotnet", "scip_clang", "scip_java", "scip_dart", "scip_php", "scip_ruby", "rust_analyzer", "timeout_seconds"))
	addMap(indexNode, "tools", indexTools, "External SCIP indexer binaries resolved on PATH.")

	appendUnknownEntries(indexNode, mappingValueNode(existing, "index"), setOf("tools"))
	addMap(mapping, "index", indexNode, "In-tree codeindex engine settings.")

	mapNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapGrouping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(mapGrouping, "resolution", cfg.Map.Grouping.Resolution, desc("map.grouping.resolution"))
	addScalar(mapGrouping, "min_group_size", cfg.Map.Grouping.MinGroupSize, desc("map.grouping.min_group_size"))
	addScalar(mapGrouping, "min_root_groups", cfg.Map.Grouping.MinRootGroups, desc("map.grouping.min_root_groups"))
	addScalar(mapGrouping, "max_root_groups", cfg.Map.Grouping.MaxRootGroups, desc("map.grouping.max_root_groups"))
	addScalar(mapGrouping, "max_children", cfg.Map.Grouping.MaxChildren, desc("map.grouping.max_children"))
	addScalar(mapGrouping, "max_depth", cfg.Map.Grouping.MaxDepth, desc("map.grouping.max_depth"))
	addScalar(mapGrouping, "max_leaf_files", cfg.Map.Grouping.MaxLeafFiles, desc("map.grouping.max_leaf_files"))
	appendUnknownEntries(mapGrouping, mappingValueNode(mappingValueNode(existing, "map"), "grouping"), setOf("resolution", "min_group_size", "min_root_groups", "max_root_groups", "max_children", "max_depth", "max_leaf_files"))
	addMap(mapNode, "grouping", mapGrouping, "Louvain grouping parameters for graph maps.")

	mapBudget := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(mapBudget, "max_connectors_per_view", cfg.Map.Budget.MaxConnectorsPerView, desc("map.budget.max_connectors_per_view"))
	addScalar(mapBudget, "max_leaf_connectors_per_view", cfg.Map.Budget.MaxLeafConnectorsPerView, desc("map.budget.max_leaf_connectors_per_view"))
	appendUnknownEntries(mapBudget, mappingValueNode(mappingValueNode(existing, "map"), "budget"), setOf("max_connectors_per_view", "max_leaf_connectors_per_view"))
	addMap(mapNode, "budget", mapBudget, "Connector drawing budgets for graph map views.")

	appendUnknownEntries(mapNode, mappingValueNode(existing, "map"), setOf("grouping", "budget"))
	addMap(mapping, "map", mapNode, "Graph map pipeline settings.")

	completion := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(completion, "remote", cfg.Completion.Remote, desc("completion.remote"))
	appendUnknownEntries(completion, mappingValueNode(existing, "completion"), setOf("remote"))
	addMap(mapping, "completion", completion, "Shell completion settings.")

	updates := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(updates, "auto", cfg.Updates.Auto, desc("updates.auto"))
	addScalar(updates, "check_interval", cfg.Updates.CheckInterval, desc("updates.check_interval"))
	appendUnknownEntries(updates, mappingValueNode(existing, "updates"), setOf("auto", "check_interval"))
	addMap(mapping, "updates", updates, "CLI update check settings.")

	appendUnknownEntries(mapping, existing, setOf("server_url", "api_key", "org_id", "apply", "database", "validation", "serve", "index", "map", "completion", "updates"))
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
	case int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v, 10)}
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
	for part := range strings.SplitSeq(dotted, ".") {
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

func normalizeConfig(cfg *Config) {
	cfg.Serve.PublicURL = normalizePublicURLValue(cfg.Serve.PublicURL)
	for i, origin := range cfg.Serve.AllowedOrigins {
		cfg.Serve.AllowedOrigins[i] = strings.TrimSpace(origin)
	}
}

func normalizePublicURLValue(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func validRootHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return false
	}
	return parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Hostname() != ""
}

func validHTTPOrigin(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Hostname() != ""
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

func setOf(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
