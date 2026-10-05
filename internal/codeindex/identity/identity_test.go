package identity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
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

func TestApplyIsolatesSameCheckoutAcrossOrganisations(t *testing.T) {
	st := openIdentityStore(t)
	ctxA := app.WithTenantOrgID(context.Background(), uuid.New())
	ctxB := app.WithTenantOrgID(context.Background(), uuid.New())
	const root = "/shared/checkout"
	const remote = "https://github.com/owner/repo"
	first, err := Apply(ctxA, st, root, "", remote, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Apply(ctxB, st, root, "", remote, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("organisations share a repository identity")
	}
	for _, tc := range []struct {
		ctx context.Context
		id  string
	}{{ctxA, first.ID}, {ctxB, second.ID}} {
		again, err := Apply(tc.ctx, st, root, "", remote, false)
		if err != nil || again.ID != tc.id {
			t.Fatalf("repeat identity = %q, err = %v, want %q", again.ID, err, tc.id)
		}
	}
}

func TestResolvePreservesExistingPathIdentity(t *testing.T) {
	st := openIdentityStore(t)
	ctx := app.WithTenantOrgID(context.Background(), uuid.New())
	const root = "/shared/legacy-checkout"
	id := graph.RepositoryID(root)
	if err := st.EnsureRepositoryIdentity(ctx, id, root, "", "", false); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve(ctx, st, root, "", "")
	if err != nil || resolved.ID != id {
		t.Fatalf("identity = %q, err = %v, want existing %q", resolved.ID, err, id)
	}
	local, err := Resolve(context.Background(), st, "/local/checkout", "", "")
	if err != nil || local.ID != graph.RepositoryID("/local/checkout") {
		t.Fatalf("local identity changed: %+v, err = %v", local, err)
	}
}
