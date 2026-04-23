package workspace_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	configDir, err := os.MkdirTemp("", "tld-workspace-config-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(configDir) }()

	if err := os.MkdirAll(configDir, 0700); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tld.yaml"), []byte("server_url: https://tldiagram.com\napi_key: \"\"\norg_id: \"\"\n"), 0600); err != nil {
		panic(err)
	}
	if err := os.Setenv("TLD_CONFIG_DIR", configDir); err != nil {
		panic(err)
	}

	code := m.Run()
	if err := os.RemoveAll(configDir); err != nil {
		panic(err)
	}
	os.Exit(code)
}
