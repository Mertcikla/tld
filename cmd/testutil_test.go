package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedElementWorkspaceIgnoresAmbientDataDir(t *testing.T) {
	dir := t.TempDir()
	ambientDataDir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", ambientDataDir)

	SeedElementWorkspace(t, dir)

	if _, err := os.Stat(filepath.Join(ambientDataDir, "tld.db")); !os.IsNotExist(err) {
		t.Fatalf("ambient data dir was used for test seed: %v", err)
	}
}
