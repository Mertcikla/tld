package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/pkg/app"
)

// TestTenantScopeIsolatesCodeindex verifies that repositories and everything
// derived from them are invisible across organisations, while an unscoped
// context (self-hosted single-tenant) still sees every row.
func TestTenantScopeIsolatesCodeindex(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	orgA, orgB := uuid.New(), uuid.New()
	ctxA := app.WithTenantOrgID(ctx, orgA)
	ctxB := app.WithTenantOrgID(ctx, orgB)

	graphA := graph.NewGraph("repo-a", "snap-a")
	graphA.Facts["fact-a"] = &pb.CodeFact{Id: "fact-a", RepositoryId: "repo-a", SnapshotId: "snap-a"}
	if err := st.Publish(ctxA, "/repo-a", &pb.Snapshot{Id: "snap-a", RepositoryId: "repo-a"}, graphA); err != nil {
		t.Fatalf("publish A: %v", err)
	}
	graphB := graph.NewGraph("repo-b", "snap-b")
	graphB.Facts["fact-b"] = &pb.CodeFact{Id: "fact-b", RepositoryId: "repo-b", SnapshotId: "snap-b"}
	if err := st.Publish(ctxB, "/repo-b", &pb.Snapshot{Id: "snap-b", RepositoryId: "repo-b"}, graphB); err != nil {
		t.Fatalf("publish B: %v", err)
	}

	// Each organisation only lists its own repository.
	reposA, err := st.ListRepositories(ctxA)
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(reposA) != 1 || reposA[0].Id != "repo-a" {
		t.Fatalf("org A repositories = %+v, want only repo-a", reposA)
	}
	reposB, err := st.ListRepositories(ctxB)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(reposB) != 1 || reposB[0].Id != "repo-b" {
		t.Fatalf("org B repositories = %+v, want only repo-b", reposB)
	}

	// Cross-organisation point lookups miss rather than leak.
	if _, err := st.Repository(ctxA, "repo-b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("org A Repository(repo-b) err = %v, want sql.ErrNoRows", err)
	}
	if _, err := st.Snapshot(ctxA, "snap-b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("org A Snapshot(snap-b) err = %v, want sql.ErrNoRows", err)
	}
	if _, err := st.Fact(ctxA, "fact-b"); err == nil {
		t.Fatal("org A can read org B's fact")
	}
	if snaps, err := st.Snapshots(ctxA, "repo-b"); err != nil || len(snaps) != 0 {
		t.Fatalf("org A Snapshots(repo-b) = %+v (err %v), want none", snaps, err)
	}

	// Watch state is stamped and isolated too.
	if err := st.UpsertWatchState(ctxA, WatchState{RepositoryID: "repo-a", State: "running"}); err != nil {
		t.Fatalf("upsert watch A: %v", err)
	}
	if _, ok, err := st.WatchState(ctxA, "repo-a"); err != nil || !ok {
		t.Fatalf("org A watch state missing: ok=%v err=%v", ok, err)
	}
	if _, ok, err := st.WatchState(ctxB, "repo-a"); err != nil || ok {
		t.Fatalf("org B can read org A's watch state: ok=%v err=%v", ok, err)
	}

	// A nil organisation keeps the pre-multi-tenant behaviour and sees all rows.
	all, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("unordered list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unscoped repositories = %d, want 2", len(all))
	}

	// The organisation is persisted on the repository row.
	var gotOrg string
	if err := st.bun.NewRaw(`SELECT org_id FROM codeindex_repositories WHERE id = ?`, "repo-a").Scan(ctx, &gotOrg); err != nil {
		t.Fatal(err)
	}
	if gotOrg != orgA.String() {
		t.Fatalf("repo-a org_id = %q, want %q", gotOrg, orgA.String())
	}
}

// TestRepositoryRemoteKeyUniquePerOrg verifies remote_key uniqueness is scoped
// to the organisation: two organisations may index the same remote, but a
// duplicate within one organisation is still rejected.
func TestRepositoryRemoteKeyUniquePerOrg(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	orgA, orgB := uuid.New(), uuid.New()
	ctxA := app.WithTenantOrgID(ctx, orgA)
	ctxB := app.WithTenantOrgID(ctx, orgB)
	const remoteKey = "github.com/owner/repo"

	if err := st.EnsureRepositoryIdentity(ctxA, "repo-a", "/a", "https://github.com/owner/repo", remoteKey, false); err != nil {
		t.Fatalf("register A: %v", err)
	}
	if err := st.EnsureRepositoryIdentity(ctxA, "repo-a-2", "/a2", "https://github.com/owner/repo", remoteKey, false); err == nil {
		t.Fatal("duplicate remote_key accepted within one organisation")
	}
	if err := st.EnsureRepositoryIdentity(ctxB, "repo-b", "/b", "https://github.com/owner/repo", remoteKey, false); err != nil {
		t.Fatalf("register B: %v", err)
	}

	gotA, ok, err := st.RepositoryByRemoteKey(ctxA, remoteKey)
	if err != nil || !ok || gotA != "repo-a" {
		t.Fatalf("org A remote key lookup = %q ok=%v err=%v, want repo-a", gotA, ok, err)
	}
	gotB, ok, err := st.RepositoryByRemoteKey(ctxB, remoteKey)
	if err != nil || !ok || gotB != "repo-b" {
		t.Fatalf("org B remote key lookup = %q ok=%v err=%v, want repo-b", gotB, ok, err)
	}
}
