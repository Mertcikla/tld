package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/workspace"
)

func TestConfigCommandPathSetGetAndListRedactsSecrets(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	dir := t.TempDir()

	stdout, _, err := RunCmd(t, dir, "config", "path")
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if strings.TrimSpace(stdout) != filepath.Join(configDir, "tld.yaml") {
		t.Fatalf("config path = %q", stdout)
	}

	if stdout, _, err = RunCmd(t, dir, "config", "set", "api_key", "secret-value"); err != nil {
		t.Fatalf("config set api_key: %v\nstdout: %s", err, stdout)
	}
	stdout, _, err = RunCmd(t, dir, "config", "get", "api_key")
	if err != nil {
		t.Fatalf("config get api_key: %v", err)
	}
	if strings.TrimSpace(stdout) != "secret-value" {
		t.Fatalf("api_key get = %q, want secret-value", stdout)
	}

	stdout, _, err = RunCmd(t, dir, "config", "list")
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	if strings.Contains(stdout, "secret-value") || !strings.Contains(stdout, "********") {
		t.Fatalf("config list did not redact api_key:\n%s", stdout)
	}

	stdout, _, err = RunCmd(t, dir, "config", "list", "--show-secrets")
	if err != nil {
		t.Fatalf("config list --show-secrets: %v", err)
	}
	if !strings.Contains(stdout, "secret-value") {
		t.Fatalf("config list --show-secrets did not include api_key:\n%s", stdout)
	}
}

func TestConfigCommandJSONAndValidation(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	dir := t.TempDir()

	if _, _, err := RunCmd(t, dir, "config", "set", "watch.languages", "go,typescript"); err != nil {
		t.Fatalf("config set languages: %v", err)
	}

	stdout, _, err := RunCmd(t, dir, "--format", "json", "config", "get", "watch.languages")
	if err != nil {
		t.Fatalf("config get json: %v", err)
	}
	var value workspace.ConfigValue
	if err := json.Unmarshal([]byte(stdout), &value); err != nil {
		t.Fatalf("unmarshal config value: %v\n%s", err, stdout)
	}
	if value.Key != "watch.languages" || value.Value != "go,typescript" || value.Source != workspace.ConfigSourceFile {
		t.Fatalf("unexpected config value: %+v", value)
	}

	stdout, _, err = RunCmd(t, dir, "--format", "json", "config", "validate")
	if err != nil {
		t.Fatalf("config validate json: %v", err)
	}
	if !strings.Contains(stdout, `"ok": true`) {
		t.Fatalf("validate json did not report ok:\n%s", stdout)
	}

	if _, _, err := RunCmd(t, dir, "config", "set", "TLD_CONFIG_DIR", "/tmp/nope"); err == nil {
		t.Fatal("expected runtime locator key to be rejected")
	}
	if _, _, err := RunCmd(t, dir, "config", "set", "serve.port", "99999"); err == nil {
		t.Fatal("expected invalid port to be rejected")
	}
}

func TestConfigCommandEnvOverrideSource(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("PORT", "7777")
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(configDir, "tld.yaml"), []byte("serve:\n  port: \"8888\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, _, err := RunCmd(t, dir, "--format", "json", "config", "get", "serve.port")
	if err != nil {
		t.Fatalf("config get serve.port: %v", err)
	}
	var value workspace.ConfigValue
	if err := json.Unmarshal([]byte(stdout), &value); err != nil {
		t.Fatalf("unmarshal config value: %v\n%s", err, stdout)
	}
	if value.Value != "7777" || value.Source != workspace.ConfigSourceEnv || value.Env != "PORT" {
		t.Fatalf("serve.port = %+v, want PORT env override", value)
	}
}
