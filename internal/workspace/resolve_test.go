package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestResolveDir(t *testing.T) {
	t.Run("empty returns empty", func(t *testing.T) {
		if got := workspace.ResolveDir(""); got != "" {
			t.Fatalf("ResolveDir(%q) = %q, want empty", "", got)
		}
	})

	t.Run("root with workspace files is used as-is", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "elements.yaml"))
		if got := workspace.ResolveDir(dir); got != dir {
			t.Fatalf("ResolveDir(%q) = %q, want %q", dir, got, dir)
		}
	})

	t.Run("nested .tld is used when root has no files", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, ".tld")
		if err := os.MkdirAll(nested, 0o750); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(nested, "elements.yaml"))
		if got := workspace.ResolveDir(dir); got != nested {
			t.Fatalf("ResolveDir(%q) = %q, want %q", dir, got, nested)
		}
	})

	t.Run("uninitialized root is returned unchanged", func(t *testing.T) {
		dir := t.TempDir()
		if got := workspace.ResolveDir(dir); got != dir {
			t.Fatalf("ResolveDir(%q) = %q, want %q", dir, got, dir)
		}
	})
}

func TestIsWorkspaceDir(t *testing.T) {
	dir := t.TempDir()
	if workspace.IsWorkspaceDir(dir) {
		t.Fatalf("IsWorkspaceDir(%q) = true, want false", dir)
	}
	writeTestFile(t, filepath.Join(dir, "connectors.yaml"))
	if !workspace.IsWorkspaceDir(dir) {
		t.Fatalf("IsWorkspaceDir(%q) = false, want true", dir)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
