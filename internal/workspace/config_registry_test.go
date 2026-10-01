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

	configPath := filepath.Join(configDir, "tld.global.yaml")
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
	configPath := filepath.Join(configDir, "tld.global.yaml")
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

func TestEnsureGlobalConfigDoesNotRewriteExistingConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "tld.global.yaml")
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
	writeFile(t, filepath.Join(configDir, "tld.global.yaml"), `serve:
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
	configPath := filepath.Join(configDir, "tld.global.yaml")
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
			writeFile(t, filepath.Join(configDir, "tld.global.yaml"), tt.content)
			if _, err := workspace.LoadGlobalConfig(); err == nil {
				t.Fatal("expected invalid self-hosted config to fail")
			}
		})
	}
}

func TestLoadGlobalConfigStateReadsLegacyConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	writeFile(t, filepath.Join(configDir, "tld.yaml"), "server_url: https://legacy.example\napi_key: legacy-secret\nunknown_root: keep-me\n")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.ServerURL != "https://legacy.example" {
		t.Fatalf("ServerURL = %q, want legacy value", cfg.ServerURL)
	}
	if cfg.APIKey != "legacy-secret" {
		t.Fatalf("APIKey = %q, want legacy value", cfg.APIKey)
	}

	path, err := workspace.ResolveConfigPath()
	if err != nil {
		t.Fatalf("ResolveConfigPath: %v", err)
	}
	if path != filepath.Join(configDir, "tld.yaml") {
		t.Fatalf("ResolveConfigPath = %q, want legacy path", path)
	}
}

func TestSetGlobalConfigValueWritesNewNameAndPreservesLegacy(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	legacyPath := filepath.Join(configDir, "tld.yaml")
	original := "server_url: https://legacy.example\nunknown_root: keep-me\n"
	writeFile(t, legacyPath, original)

	if err := workspace.SetGlobalConfigValue("serve.port", "9000"); err != nil {
		t.Fatalf("SetGlobalConfigValue: %v", err)
	}

	newPath := filepath.Join(configDir, "tld.global.yaml")
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "port:") || !strings.Contains(content, "9000") {
		t.Fatalf("new config missing updated value:\n%s", content)
	}
	if !strings.Contains(content, "unknown_root: keep-me") {
		t.Fatalf("new config did not preserve legacy unknown key:\n%s", content)
	}

	legacyData, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy config: %v", err)
	}
	if string(legacyData) != original {
		t.Fatalf("legacy config was modified:\n%s", legacyData)
	}

	path, err := workspace.ResolveConfigPath()
	if err != nil {
		t.Fatalf("ResolveConfigPath: %v", err)
	}
	if path != newPath {
		t.Fatalf("ResolveConfigPath = %q, want canonical path", path)
	}
}

func TestLoadGlobalConfigPrefersCanonicalOverLegacy(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	writeFile(t, filepath.Join(configDir, "tld.yaml"), "server_url: https://legacy.example\n")
	writeFile(t, filepath.Join(configDir, "tld.global.yaml"), "server_url: https://canonical.example\n")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.ServerURL != "https://canonical.example" {
		t.Fatalf("ServerURL = %q, want canonical value", cfg.ServerURL)
	}
}

func TestEnsureGlobalConfigDoesNotCreateCanonicalWhenLegacyExists(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	writeFile(t, filepath.Join(configDir, "tld.yaml"), "server_url: https://legacy.example\n")

	if err := workspace.EnsureGlobalConfig(); err != nil {
		t.Fatalf("EnsureGlobalConfig: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tld.global.yaml")); !os.IsNotExist(err) {
		t.Fatalf("canonical config should not be created when legacy exists (err=%v)", err)
	}
}

func TestEnsureGlobalConfigCreatesCanonicalWhenMissing(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)

	if err := workspace.EnsureGlobalConfig(); err != nil {
		t.Fatalf("EnsureGlobalConfig: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tld.global.yaml")); err != nil {
		t.Fatalf("canonical config was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tld.yaml")); !os.IsNotExist(err) {
		t.Fatalf("legacy config should not be created (err=%v)", err)
	}
}

func TestExistingGlobalConfigPathReturnsLegacyWhenOnlyLegacy(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	legacyPath := filepath.Join(configDir, "tld.yaml")
	writeFile(t, legacyPath, "server_url: https://legacy.example\n")

	path, ok := workspace.ExistingGlobalConfigPath()
	if !ok || path != legacyPath {
		t.Fatalf("ExistingGlobalConfigPath = (%q, %v), want (%q, true)", path, ok, legacyPath)
	}
}
