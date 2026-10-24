package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestCacheTargetIdentity(t *testing.T) {
	cfg := *workspace.DefaultConfig()
	cfg.APIKey, cfg.ServerURL, cfg.WorkspaceID = "secret", "https://example.test", "org-a"
	key := func(cfg workspace.Config, target, dir string) string {
		t.Helper()
		value, err := CacheTarget(cfg, target, dir)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	remote := key(cfg, "cloud", "")
	cfg.APIKey, cfg.ServerURL = "rotated", "https://example.test/api/"
	if key(cfg, "remote", "") != remote || key(cfg, "auto", "") != remote {
		t.Fatal("credentials or URL alias changed destination identity")
	}
	cfg.WorkspaceID = "org-b"
	if key(cfg, "cloud", "") == remote {
		t.Fatal("different cloud organizations share an ID namespace")
	}
	cfg.WorkspaceID, cfg.ServerURL = "org-a", "https://other.test"
	if key(cfg, "cloud", "") == remote {
		t.Fatal("different cloud servers share an ID namespace")
	}
	dir := t.TempDir()
	local := key(cfg, "local", dir)
	if local == remote || local == key(cfg, "local", t.TempDir()) {
		t.Fatal("different databases share an ID namespace")
	}
	if local != key(cfg, "local", filepath.Join(dir, "nested", "..")) {
		t.Fatal("equivalent local paths use different namespaces")
	}
	cfg.Database.Driver, cfg.Database.DatabaseURL = "postgres", "postgres://user:secret@host/db?search_path=one"
	pg := key(cfg, "local", dir)
	cfg.Database.Driver, cfg.Database.DatabaseURL = "postgresql", "postgres://user:rotated@host/db?search_path=one"
	if pg != key(cfg, "local", t.TempDir()) {
		t.Fatal("Postgres identity depends on password, driver alias or data directory")
	}
	cfg.Database.DatabaseURL = "postgres://user:rotated@host/db?search_path=two"
	if pg == key(cfg, "local", dir) {
		t.Fatal("different Postgres schemas share an ID namespace")
	}
}

func TestCacheTargetBeforeDatabaseCreation(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cfg := *workspace.DefaultConfig()
	dir := filepath.Join(link, "new-data")
	before, err := CacheTarget(cfg, "local", dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tld.db"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	after, err := CacheTarget(cfg, "local", filepath.Join(root, "new-data"))
	if err != nil || before != after {
		t.Fatalf("database creation changed identity: %s != %s (%v)", before, after, err)
	}
}
