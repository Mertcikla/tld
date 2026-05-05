package completion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteEnabledUsesGlobalConfigAndEnvOverride(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "tld.yaml"), []byte("completion:\n  remote: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if !remoteEnabled() {
		t.Fatal("expected completion.remote config to enable remote completion")
	}

	t.Setenv("TLD_COMPLETION_REMOTE", "0")
	if remoteEnabled() {
		t.Fatal("expected TLD_COMPLETION_REMOTE=0 to disable remote completion")
	}
}
