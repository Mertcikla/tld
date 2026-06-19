package workspace_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestLoadGlobalConfigStateReportsEnvSourcesAndDoesNotRewriteExistingConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_API_KEY", "env-secret")

	configPath := filepath.Join(configDir, "tld.yaml")
	original := "server_url: http://file.example\nunknown_root: keep-me\n"
	writeFile(t, configPath, original)

	state, err := workspace.LoadGlobalConfigState()
	if err != nil {
		t.Fatalf("LoadGlobalConfigState: %v", err)
	}
	if state.Config.APIKey != "env-secret" {
		t.Fatalf("APIKey = %q, want env-secret", state.Config.APIKey)
	}

	var apiKey workspace.ConfigValue
	for _, value := range state.Values {
		if value.Key == "api_key" {
			apiKey = value
			break
		}
	}
	if apiKey.Source != workspace.ConfigSourceEnv || apiKey.Env != "TLD_API_KEY" {
		t.Fatalf("api_key source = %q env = %q, want env/TLD_API_KEY", apiKey.Source, apiKey.Env)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "env-secret") {
		t.Fatalf("env override was persisted:\n%s", content)
	}
	if content != original {
		t.Fatalf("config was rewritten:\n%s", content)
	}
}

func TestSetGlobalConfigValuePreservesUnknownAndValidates(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "tld.yaml")
	writeFile(t, configPath, "server_url: https://tldiagram.com\nunknown_root: keep-me\nwatch:\n  unknown_watch: still-here\n")

	if err := workspace.SetGlobalConfigValue("serve.port", "9000"); err != nil {
		t.Fatalf("SetGlobalConfigValue: %v", err)
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Serve.Port != "9000" {
		t.Fatalf("Serve.Port = %q, want 9000", cfg.Serve.Port)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "unknown_root: keep-me") || !strings.Contains(content, "unknown_watch: still-here") {
		t.Fatalf("unknown keys were not preserved:\n%s", content)
	}

	if err := workspace.SetGlobalConfigValue("watch.watcher", "bogus"); err == nil {
		t.Fatal("expected invalid watcher to fail")
	}
}

func TestSetGlobalConfigValueSupportsEmbeddingMaxTokens(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)

	if err := workspace.SetGlobalConfigValue("watch.embedding.max_tokens", "8192"); err != nil {
		t.Fatalf("SetGlobalConfigValue: %v", err)
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Watch.Embedding.MaxTokens != 8192 {
		t.Fatalf("max_tokens = %v, want 8192", cfg.Watch.Embedding.MaxTokens)
	}
	if err := workspace.SetGlobalConfigValue("watch.embedding.max_tokens", "-1"); err == nil {
		t.Fatal("expected negative max_tokens to fail validation")
	}
}

func TestWatchDependenciesEnabledConfigDefaultsAndOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Watch.Dependencies.Enabled {
		t.Fatal("watch.dependencies.enabled should default to false")
	}
	if err := workspace.SetGlobalConfigValue("watch.dependencies.enabled", "true"); err != nil {
		t.Fatalf("SetGlobalConfigValue: %v", err)
	}
	cfg, err = workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if !cfg.Watch.Dependencies.Enabled {
		t.Fatal("watch.dependencies.enabled should be true after config set")
	}
	t.Setenv("TLD_WATCH_DEPENDENCIES_ENABLED", "false")
	cfg, err = workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig with env: %v", err)
	}
	if cfg.Watch.Dependencies.Enabled {
		t.Fatal("TLD_WATCH_DEPENDENCIES_ENABLED=false should override file config")
	}
}

func TestWatchEmbeddingEndpointSupportsMultipleValues(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "tld.yaml")
	writeFile(t, configPath, `watch:
  embedding:
    provider: openai
    endpoint:
      - http://127.0.0.1:8000/v1/embeddings
      - http://127.0.0.1:8001/v1/embeddings
    model: embeddinggemma-300m-4bit
    health_threshold: 0.7
`)

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	want := []string{"http://127.0.0.1:8000/v1/embeddings", "http://127.0.0.1:8001/v1/embeddings"}
	if got := cfg.Watch.Embedding.Endpoint.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("endpoints = %#v, want %#v", got, want)
	}

	if err := workspace.SetGlobalConfigValue("watch.embedding.endpoint", strings.Join(want, ",")); err != nil {
		t.Fatalf("SetGlobalConfigValue: %v", err)
	}
	cfg, err = workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig after set: %v", err)
	}
	if got := cfg.Watch.Embedding.Endpoint.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("endpoints after set = %#v, want %#v", got, want)
	}
}

func TestEnsureGlobalConfigDoesNotRewriteExistingConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "tld.yaml")
	original := "server_url: https://example.invalid\napi_key: existing-secret\norg_id: existing-org\nunknown_root: keep-me\n"
	writeFile(t, configPath, original)

	if err := workspace.EnsureGlobalConfig(); err != nil {
		t.Fatalf("EnsureGlobalConfig: %v", err)
	}

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.ServerURL != "https://example.invalid" {
		t.Fatalf("ServerURL = %q, want existing value", cfg.ServerURL)
	}
	if cfg.APIKey != "existing-secret" {
		t.Fatalf("APIKey = %q, want existing-secret", cfg.APIKey)
	}
	if cfg.WorkspaceID != "existing-org" {
		t.Fatalf("WorkspaceID = %q, want existing-org", cfg.WorkspaceID)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "unknown_root: keep-me") {
		t.Fatalf("unknown key was not preserved:\n%s", content)
	}
	if content != original {
		t.Fatalf("existing config was rewritten:\n%s", content)
	}
}

func TestResolveWatchLayoutConfigUsesEnvOverride(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("LAYOUT_LINK_DISTANCE", "222")
	writeFile(t, filepath.Join(configDir, "tld.yaml"), "watch:\n  layout:\n    link_distance: 111\n")

	got := workspace.ResolveWatchLayoutConfig()
	if got.LinkDistance != 222 {
		t.Fatalf("LinkDistance = %v, want env override 222", got.LinkDistance)
	}
}

func TestGlobalConfigLSPDefaultsAndEnvOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_WATCH_LSP_ENABLED", "false")
	t.Setenv("TLD_WATCH_LSP_HEALTH_INTERVAL", "2m")
	t.Setenv("TLD_WATCH_LSP_MEMORY_LIMIT_BYTES", "2048")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Watch.LSP.Enabled {
		t.Fatal("Watch.LSP.Enabled = true, want env override false")
	}
	if cfg.Watch.LSP.HealthInterval != "2m" {
		t.Fatalf("Watch.LSP.HealthInterval = %q, want 2m", cfg.Watch.LSP.HealthInterval)
	}
	if cfg.Watch.LSP.MemoryLimitBytes != 2048 {
		t.Fatalf("Watch.LSP.MemoryLimitBytes = %d, want 2048", cfg.Watch.LSP.MemoryLimitBytes)
	}
	if cfg.Watch.LSP.Commands["go"] != "" {
		t.Fatalf("default Go LSP command = %q, want empty", cfg.Watch.LSP.Commands["go"])
	}
}

func TestGlobalConfigLSPCommandOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_WATCH_LSP_GO_COMMAND", "/opt/bin/gopls -remote=auto")
	writeFile(t, filepath.Join(configDir, "tld.yaml"), `watch:
  lsp:
    commands:
      python: pyright-langserver --stdio
`)

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if got := cfg.Watch.LSP.Commands["go"]; got != "/opt/bin/gopls -remote=auto" {
		t.Fatalf("Go command = %q", got)
	}
	if got := cfg.Watch.LSP.Commands["python"]; got != "pyright-langserver --stdio" {
		t.Fatalf("Python command = %q", got)
	}

	if err := workspace.SetGlobalConfigValue("watch.lsp.commands.typescript", "typescript-language-server --stdio"); err != nil {
		t.Fatalf("SetGlobalConfigValue command: %v", err)
	}
	cfg, err = workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig after set: %v", err)
	}
	if got := cfg.Watch.LSP.Commands["typescript"]; got != "typescript-language-server --stdio" {
		t.Fatalf("TypeScript command = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(configDir, "tld.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if content := string(data); !strings.Contains(content, "commands:") || !strings.Contains(content, "typescript: typescript-language-server --stdio") {
		t.Fatalf("command override was not written:\n%s", content)
	}
	if err := workspace.SetGlobalConfigValue("watch.lsp.commands.ruby", "ruby-lsp"); err == nil {
		t.Fatal("expected unsupported language key to fail")
	}
}

func TestGlobalConfigRejectsUnsupportedLSPCommandLanguage(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	writeFile(t, filepath.Join(configDir, "tld.yaml"), `watch:
  lsp:
    commands:
      ruby: ruby-lsp
`)

	if _, err := workspace.LoadGlobalConfig(); err == nil {
		t.Fatal("expected unsupported LSP command language to fail validation")
	}
}

func TestGlobalConfigScaleRecentAndCallerEnvOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_WATCH_SCALE_MAX_RECENT_FILES", "123")
	t.Setenv("TLD_WATCH_SCALE_MAX_CALLER_DEPTH", "7")
	t.Setenv("TLD_WATCH_SCALE_MAX_BLAST_RADIUS_HOPS", "0")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Watch.Scale.MaxRecentFiles != 123 {
		t.Fatalf("MaxRecentFiles = %d, want 123", cfg.Watch.Scale.MaxRecentFiles)
	}
	if cfg.Watch.Scale.MaxCallerDepth != 7 {
		t.Fatalf("MaxCallerDepth = %d, want 7", cfg.Watch.Scale.MaxCallerDepth)
	}
	if cfg.Watch.Scale.MaxBlastRadiusHops != 0 {
		t.Fatalf("MaxBlastRadiusHops = %d, want 0", cfg.Watch.Scale.MaxBlastRadiusHops)
	}
}

func TestGlobalConfigDatabaseEnvOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_DB_DRIVER", "postgres")
	t.Setenv("TLD_DATABASE_URL", "postgres://user:pass@example.test/tld")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Database.Driver != "postgres" {
		t.Fatalf("Database.Driver = %q, want postgres", cfg.Database.Driver)
	}
	if cfg.Database.DatabaseURL != "postgres://user:pass@example.test/tld" {
		t.Fatalf("Database.DatabaseURL = %q", cfg.Database.DatabaseURL)
	}
}

func TestGlobalConfigServeSelfHostedEnvOverrides(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_PUBLIC_URL", "https://app.example.com/")
	t.Setenv("TLD_ALLOWED_ORIGINS", "https://admin.example.com, https://preview.example.com:8443")
	writeFile(t, filepath.Join(configDir, "tld.yaml"), `serve:
  public_url: https://file.example.com
  allowed_origins:
    - https://file-admin.example.com
`)

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Serve.PublicURL != "https://app.example.com" {
		t.Fatalf("Serve.PublicURL = %q, want trimmed env value", cfg.Serve.PublicURL)
	}
	wantOrigins := []string{"https://admin.example.com", "https://preview.example.com:8443"}
	if !reflect.DeepEqual(cfg.Serve.AllowedOrigins, wantOrigins) {
		t.Fatalf("Serve.AllowedOrigins = %#v, want %#v", cfg.Serve.AllowedOrigins, wantOrigins)
	}
}

func TestSetGlobalConfigSelfHostedValuesPreservesUnknownAndNormalizesPublicURL(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "tld.yaml")
	writeFile(t, configPath, "serve:\n  host: 127.0.0.1\n  unknown_serve: keep-me\nunknown_root: keep-too\n")

	if err := workspace.SetGlobalConfigValue("serve.public_url", "https://app.example.com/"); err != nil {
		t.Fatalf("SetGlobalConfigValue public_url: %v", err)
	}
	if err := workspace.SetGlobalConfigValue("serve.allowed_origins", "https://admin.example.com,https://preview.example.com:8443"); err != nil {
		t.Fatalf("SetGlobalConfigValue allowed_origins: %v", err)
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Serve.PublicURL != "https://app.example.com" {
		t.Fatalf("Serve.PublicURL = %q, want trimmed URL", cfg.Serve.PublicURL)
	}
	wantOrigins := []string{"https://admin.example.com", "https://preview.example.com:8443"}
	if !reflect.DeepEqual(cfg.Serve.AllowedOrigins, wantOrigins) {
		t.Fatalf("Serve.AllowedOrigins = %#v, want %#v", cfg.Serve.AllowedOrigins, wantOrigins)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "unknown_serve: keep-me") || !strings.Contains(content, "unknown_root: keep-too") {
		t.Fatalf("unknown keys were not preserved:\n%s", content)
	}
	if strings.Contains(content, "https://app.example.com/") {
		t.Fatalf("public_url should be stored without trailing slash:\n%s", content)
	}
}

func TestGlobalConfigRejectsInvalidSelfHostedURLs(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "public url requires http scheme",
			content: "serve:\n  public_url: ftp://app.example.com\n",
		},
		{
			name:    "public url cannot use subpath",
			content: "serve:\n  public_url: https://app.example.com/tld\n",
		},
		{
			name: "allowed origin cannot include path",
			content: `serve:
  allowed_origins:
    - https://admin.example.com/app
`,
		},
		{
			name: "allowed origin requires http scheme",
			content: `serve:
  allowed_origins:
    - vscode-webview://abc123
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("TLD_CONFIG_DIR", configDir)
			writeFile(t, filepath.Join(configDir, "tld.yaml"), tt.content)
			if _, err := workspace.LoadGlobalConfig(); err == nil {
				t.Fatal("expected invalid self-hosted config to fail")
			}
		})
	}
}

func TestSetGlobalConfigLSPValue(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)

	if err := workspace.SetGlobalConfigValue("watch.lsp.memory_limit_bytes", "4096"); err != nil {
		t.Fatalf("SetGlobalConfigValue memory: %v", err)
	}
	if err := workspace.SetGlobalConfigValue("watch.lsp.health_interval", "30s"); err != nil {
		t.Fatalf("SetGlobalConfigValue interval: %v", err)
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Watch.LSP.MemoryLimitBytes != 4096 {
		t.Fatalf("Watch.LSP.MemoryLimitBytes = %d, want 4096", cfg.Watch.LSP.MemoryLimitBytes)
	}
	if cfg.Watch.LSP.HealthInterval != "30s" {
		t.Fatalf("Watch.LSP.HealthInterval = %q, want 30s", cfg.Watch.LSP.HealthInterval)
	}

	if err := workspace.SetGlobalConfigValue("watch.lsp.memory_limit_bytes", "0"); err == nil {
		t.Fatal("expected invalid memory limit to fail")
	}
}
