package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// PrepareCommand fingerprints optional commit setup without changing the
	// compatibility hash of existing configurations when it is unset.
	PrepareCommand string `yaml:"-" json:"PrepareCommand,omitempty"`
	GRPC           struct {
		Address string `yaml:"address"`
	} `yaml:"grpc"`
	Surreal struct {
		URL       string `yaml:"url"`
		Namespace string `yaml:"namespace"`
		Database  string `yaml:"database"`
		Username  string `yaml:"username"`
		Password  string `yaml:"password"`
	} `yaml:"surreal"`
	Web struct {
		Address string `yaml:"address"`
		Dir     string `yaml:"dir"`
	} `yaml:"web"`
	Tools struct {
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
	} `yaml:"tools"`
}

func Default() Config {
	var c Config
	c.GRPC.Address = "127.0.0.1:50051"
	c.Web.Address = "127.0.0.1:8090"
	c.Web.Dir = "web/dist"
	c.Surreal.URL = "ws://127.0.0.1:8001/rpc"
	c.Surreal.Namespace = "codeindex"
	c.Surreal.Database = "main"
	c.Surreal.Username = "root"
	c.Surreal.Password = "root"
	c.Tools.SCIPGo = "scip-go"
	c.Tools.SCIPTypeScript = "scip-typescript"
	c.Tools.SCIPPython = "scip-python"
	c.Tools.SCIPDotnet = "scip-dotnet"
	c.Tools.SCIPClang = "scip-clang"
	c.Tools.SCIPJava = "scip-java"
	c.Tools.SCIPDart = "scip-dart"
	c.Tools.SCIPPhp = "scip-php"
	c.Tools.SCIPRuby = "scip-ruby"
	c.Tools.RustAnalyzer = "rust-analyzer"
	c.Tools.TimeoutSeconds = 300
	return c
}
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return c, err
		}
		if err := yaml.Unmarshal(b, &c); err != nil {
			return c, err
		}
	}
	c.ApplyEnv()
	if c.Tools.TimeoutSeconds <= 0 {
		c.Tools.TimeoutSeconds = 300
	}
	return c, nil
}

// ApplyEnv overlays the CODEINDEX_* environment variables onto the config so
// they take precedence over file and global settings. It is used by the CLI's
// config bridge as well as Load.
func (c *Config) ApplyEnv() {
	env := map[string]*string{"CODEINDEX_GRPC_ADDRESS": &c.GRPC.Address, "CODEINDEX_SURREAL_URL": &c.Surreal.URL, "CODEINDEX_SURREAL_NAMESPACE": &c.Surreal.Namespace, "CODEINDEX_SURREAL_DATABASE": &c.Surreal.Database, "CODEINDEX_SURREAL_USERNAME": &c.Surreal.Username, "CODEINDEX_SURREAL_PASSWORD": &c.Surreal.Password, "CODEINDEX_SCIP_GO": &c.Tools.SCIPGo, "CODEINDEX_SCIP_TYPESCRIPT": &c.Tools.SCIPTypeScript, "CODEINDEX_SCIP_PYTHON": &c.Tools.SCIPPython, "CODEINDEX_SCIP_DOTNET": &c.Tools.SCIPDotnet, "CODEINDEX_SCIP_CLANG": &c.Tools.SCIPClang, "CODEINDEX_SCIP_JAVA": &c.Tools.SCIPJava, "CODEINDEX_SCIP_DART": &c.Tools.SCIPDart, "CODEINDEX_SCIP_PHP": &c.Tools.SCIPPhp, "CODEINDEX_SCIP_RUBY": &c.Tools.SCIPRuby, "CODEINDEX_RUST_ANALYZER": &c.Tools.RustAnalyzer}
	for name, p := range env {
		if v, ok := os.LookupEnv(name); ok {
			*p = v
		}
	}
}
func (c Config) ToolTimeout() time.Duration {
	return time.Duration(c.Tools.TimeoutSeconds) * time.Second
}
