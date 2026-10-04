package identity

import (
	"context"
	"path/filepath"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func openIdentityStore(t *testing.T) *cstore.Store {
	t.Helper()
	handle, err := dbrepo.OpenSQLite(context.Background(), dbrepo.DBOptions{
		SQLitePath: filepath.Join(t.TempDir(), "tld.db"),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return cstore.NewStoreFromHandle(handle)
}

func TestApplyReusesRemoteIdentityAcrossCheckouts(t *testing.T) {
	ctx := context.Background()
	st := openIdentityStore(t)

	first, err := Apply(ctx, st, "/Users/alice/proj", "", "https://github.com/Owner/Repo.git", false)
	if err != nil {
		t.Fatalf("apply first: %v", err)
	}
	if first.RemoteKey != "github.com/owner/repo" {
		t.Fatalf("remote key = %q", first.RemoteKey)
	}
	second, err := Apply(ctx, st, "/home/bob/proj", "", "git@github.com:Owner/Repo.git", false)
	if err != nil {
		t.Fatalf("apply second: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second id = %q, want %q", second.ID, first.ID)
	}
}

func TestResolveFallsBackToPathWithoutRemote(t *testing.T) {
	ctx := context.Background()
	st := openIdentityStore(t)

	resolved, err := Resolve(ctx, st, "/tmp/standalone", "", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.RemoteKey != "" {
		t.Fatalf("remote key = %q, want empty", resolved.RemoteKey)
	}
	if resolved.ID == "" {
		t.Fatal("expected a path-derived id")
	}
}

func TestResolveHonorsExplicitRegisteredID(t *testing.T) {
	ctx := context.Background()
	st := openIdentityStore(t)

	if err := st.EnsureRepositoryIdentity(ctx, "pinned", "/somewhere", "", "", false); err != nil {
		t.Fatalf("ensure pinned: %v", err)
	}
	resolved, err := Resolve(ctx, st, "/tmp/standalone", "pinned", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.ID != "pinned" {
		t.Fatalf("id = %q, want pinned", resolved.ID)
	}
}
