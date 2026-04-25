package workspace_test

import (
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/internal/workspace"
)

func TestResolveDataDirDefaultsToAppDataDir(t *testing.T) {
	t.Setenv("TLD_DATA_DIR", "")

	xdgData := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdgData)

	got, err := workspace.ResolveDataDir(&workspace.GlobalConfig{}, "")
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}

	want := filepath.Join(xdgData, "tldiagram")
	if got != want {
		t.Fatalf("ResolveDataDir default = %q, want %q", got, want)
	}
}
