package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/proto"
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

// Identical content-derived IDs must coexist, and retries must update only the
// requesting organisation's row. Exercise actual writes rather than only filters.
func TestTenantScopeCollidingKeys(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testTenantScopeCollidingKeys(t, st)
}

func testTenantScopeCollidingKeys(t *testing.T, st *Store) {
	contexts := []context.Context{
		app.WithTenantOrgID(context.Background(), uuid.New()),
		app.WithTenantOrgID(context.Background(), uuid.New()),
	}
	write := func(ctx context.Context, label string, resourceID int64) {
		t.Helper()
		g := graph.NewGraph("repo", "snap")
		g.Facts["fact"] = &pb.CodeFact{Id: "fact", RepositoryId: "repo", SnapshotId: "snap", Name: label, LogicalKey: "file:main.go"}
		g.EdgeFacts["edge"] = &pb.EdgeFact{Id: "edge", RepositoryId: "repo", SnapshotId: "snap", LogicalKey: label}
		snap := &pb.Snapshot{Id: "snap", RepositoryId: "repo", GitBranch: label, Sources: []*pb.SourceFile{{Path: "main.go", Hash: label}}}
		g.Sources["main.go"] = &graph.Source{Text: []byte(label)}
		for _, err := range []error{
			st.EnsureRepositoryIdentity(ctx, "repo", "/"+label, "", "", false),
			st.Publish(ctx, "/"+label, snap, g),
			st.SaveMappings(ctx, []ResourceMapping{{LogicalKey: "file:main.go", ResourceID: resourceID, RepositoryID: "repo", SnapshotID: "snap"}}),
			st.SaveImpact(ctx, &pb.ImpactDiagram{RepositoryId: "repo", ComparisonKey: "live", ViewId: resourceID}),
			st.UpsertWatchState(ctx, WatchState{RepositoryID: "repo", State: label, OwnerID: label}),
			st.SaveCompletedMap(ctx, "repo", &pb.CompletedMap{Result: &pb.MapResult{RunId: "run", SnapshotId: "snap", ViewId: resourceID}, ConfigHash: label}),
			st.SaveRepositoryMapOverrides(ctx, "repo", &pb.RepositoryMapConfiguration{MaxLeafFiles: proto.Uint32(uint32(resourceID))}),
			st.SaveAnalysis(ctx, AnalysisRun{ID: "analysis", RepositoryID: "repo", SnapshotID: "snap", Groups: []AnalysisGroup{{ID: "group", Label: label, Members: []string{"fact"}}}}),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	verify := func(ctx context.Context, label string, resourceID int64) {
		t.Helper()
		m, ok, err := st.MappingByLogicalKey(ctx, "file:main.go")
		if err != nil || !ok || m.ResourceID != resourceID {
			t.Fatalf("%s mapping = %+v, ok=%v err=%v", label, m, ok, err)
		}
		impact, err := st.Impact(ctx, "repo", "live")
		if err != nil || impact.ViewId != resourceID {
			t.Fatalf("%s impact = %+v err=%v", label, impact, err)
		}
		watch, ok, err := st.WatchState(ctx, "repo")
		if err != nil || !ok || watch.State != label {
			t.Fatalf("%s watch = %+v err=%v", label, watch, err)
		}
		active, err := st.ActiveMap(ctx, "repo")
		if err != nil || active == nil || active.Result.ViewId != resourceID {
			t.Fatalf("%s active map = %+v err=%v", label, active, err)
		}
		overrides, err := st.RepositoryMapOverrides(ctx, "repo")
		if err != nil || overrides.GetMaxLeafFiles() != uint32(resourceID) {
			t.Fatalf("%s overrides = %+v err=%v", label, overrides, err)
		}
		repos, err := st.ListRepositories(ctx)
		if err != nil || len(repos) != 1 || repos[0].Root != "/"+label || repos[0].GitBranch != label || repos[0].Facts != 1 || repos[0].Edges != 1 || repos[0].Sources != 1 {
			t.Fatalf("%s repositories = %+v err=%v", label, repos, err)
		}
		snaps, err := st.Snapshots(ctx, "repo")
		if err != nil || len(snaps) != 1 || snaps[0].GitBranch != label || snaps[0].Statistics.Facts != 1 {
			t.Fatalf("%s snapshots = %+v err=%v", label, snaps, err)
		}
		facts, err := st.Facts(ctx, "snap", pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 10)
		if err != nil || len(facts) != 1 || facts[0].Name != label {
			t.Fatalf("%s facts = %+v err=%v", label, facts, err)
		}
		edges, err := st.EdgeFacts(ctx, "snap", pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", "", 10)
		if err != nil || len(edges) != 1 || edges[0].LogicalKey != label {
			t.Fatalf("%s edges = %+v err=%v", label, edges, err)
		}
		var groupLabel string
		if err := st.bun.NewRaw(`SELECT label FROM codeindex_groups WHERE id = ? AND org_id = ?`, "group", scope(ctx).value()).Scan(ctx, &groupLabel); err != nil || groupLabel != label {
			t.Fatalf("%s group = %s err=%v", label, groupLabel, err)
		}
	}
	write(contexts[0], "a", 111)
	write(contexts[1], "b", 999)
	verify(contexts[0], "a", 111)
	verify(contexts[1], "b", 999)
	// Same-tenant retries still update mutable rows without duplicating immutable ones.
	write(contexts[1], "b", 1000)
	verify(contexts[0], "a", 111)
	verify(contexts[1], "b", 1000)
	if err := st.DeleteSnapshot(contexts[1], "snap"); err != nil {
		t.Fatal(err)
	}
	verify(contexts[0], "a", 111)
	if _, err := st.Fact(contexts[1], "fact"); err == nil {
		t.Fatal("unreferenced B fact survived snapshot deletion")
	}
	write(contexts[1], "b", 1000)
	if err := st.DeleteRepository(contexts[1], "repo"); err != nil {
		t.Fatal(err)
	}
	verify(contexts[0], "a", 111)
	if _, err := st.Repository(contexts[1], "repo"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted repository err=%v", err)
	}
}

func TestTenantScopeLeaseAndWatchClaims(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testTenantScopeLeaseAndWatchClaims(t, st)
}

func testTenantScopeLeaseAndWatchClaims(t *testing.T, st *Store) {
	for _, ctx := range []context.Context{
		app.WithTenantOrgID(context.Background(), uuid.New()),
		app.WithTenantOrgID(context.Background(), uuid.New()),
	} {
		_, release, err := st.AcquireLease(ctx, "repo")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if _, _, err := st.AcquireLease(ctx, "repo"); !errors.Is(err, ErrBusy) {
			t.Fatalf("same-tenant lease err=%v", err)
		}
		claim := WatchState{RepositoryID: "repo", OwnerKind: "server", OwnerPID: os.Getpid(), OwnerID: "owner", State: "watching"}
		if err := st.ClaimWatch(ctx, claim); err != nil {
			t.Fatal(err)
		}
		if err := st.ClaimWatch(ctx, claim); err != nil {
			t.Fatal(err)
		}
		claim.OwnerID = "other"
		if err := st.ClaimWatch(ctx, claim); !errors.Is(err, ErrWatchActive) {
			t.Fatalf("same-tenant watch claim err=%v", err)
		}
	}
}

func TestRepositoryWritesPreserveForeignOwnership(t *testing.T) {
	for _, unscopedOwner := range []bool{false, true} {
		for _, operation := range []string{"identity", "publish", "historical"} {
			t.Run(fmt.Sprintf("unscoped=%t/%s", unscopedOwner, operation), func(t *testing.T) {
				st, handle := openTestStore(t)
				defer func() { _ = handle.Close() }()
				owner := context.Background()
				if !unscopedOwner {
					owner = app.WithTenantOrgID(owner, uuid.New())
				}
				other := app.WithTenantOrgID(context.Background(), uuid.New())
				original := &pb.Snapshot{Id: "original", RepositoryId: "repo"}
				if err := st.Publish(owner, "/original", original, graph.NewGraph("repo", original.Id)); err != nil {
					t.Fatal(err)
				}
				attempt := &pb.Snapshot{Id: "foreign", RepositoryId: "repo"}
				var err error
				switch operation {
				case "identity":
					err = st.EnsureRepositoryIdentity(other, "repo", "/foreign", "https://github.com/foreign/repo", "github.com/foreign/repo", true)
				case "publish":
					err = st.Publish(other, "/foreign", attempt, graph.NewGraph("repo", attempt.Id))
				case "historical":
					err = st.PublishHistorical(other, "/foreign", attempt, graph.NewGraph("repo", attempt.Id))
				}
				if err != nil {
					t.Fatalf("independent tenant write: %v", err)
				}
				// Unscoped reads intentionally include all tenants; inspect the owner row directly.
				var repo struct {
					Root             string
					LatestSnapshotId string `bun:"latest_snapshot_id"`
				}
				err = st.bun.NewRaw(`SELECT root, latest_snapshot_id FROM codeindex_repositories WHERE id = ? AND org_id = ?`, "repo", scope(owner).value()).Scan(owner, &repo)
				if err != nil || repo.Root != "/original" || repo.LatestSnapshotId != original.Id {
					t.Fatalf("owner repository changed: %+v, err = %v", repo, err)
				}
				var remote string
				var managed bool
				err = st.bun.NewRaw(`SELECT remote_url, managed FROM codeindex_repositories WHERE id = ? AND org_id = ?`, "repo", scope(owner).value()).Scan(owner, &remote, &managed)
				if err != nil || remote != "" || managed {
					t.Fatalf("owner origin changed: remote=%q managed=%v err=%v", remote, managed, err)
				}
				otherRepo, err := st.Repository(other, "repo")
				if err != nil || otherRepo.Root != "/foreign" {
					t.Fatalf("other repository = %+v, err = %v", otherRepo, err)
				}
				if operation != "identity" {
					if _, err := st.Snapshot(other, attempt.Id); err != nil {
						t.Fatalf("independent publication missing snapshot: %v", err)
					}
				}
			})
		}
	}
}

func TestSnapshotCollisionPreservesIndependentPublications(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctxA := app.WithTenantOrgID(context.Background(), uuid.New())
	ctxB := app.WithTenantOrgID(context.Background(), uuid.New())
	snap := &pb.Snapshot{Id: "shared-snapshot-id", RepositoryId: "repo-a", GitRevision: "original"}
	if err := st.Publish(ctxA, "/a", snap, graph.NewGraph(snap.RepositoryId, snap.Id)); err != nil {
		t.Fatal(err)
	}
	attempt := &pb.Snapshot{Id: snap.Id, RepositoryId: "repo-b", GitRevision: "foreign"}
	if err := st.Publish(ctxB, "/b", attempt, graph.NewGraph(attempt.RepositoryId, attempt.Id)); err != nil {
		t.Fatalf("independent publication: %v", err)
	}
	got, err := st.Snapshot(ctxA, snap.Id)
	if err != nil || got.RepositoryId != snap.RepositoryId || got.GitRevision != snap.GitRevision {
		t.Fatalf("owner snapshot changed: %+v, err = %v", got, err)
	}
	got, err = st.Snapshot(ctxB, attempt.Id)
	if err != nil || got.RepositoryId != attempt.RepositoryId || got.GitRevision != attempt.GitRevision {
		t.Fatalf("other snapshot = %+v, err = %v", got, err)
	}
	repo, err := st.Repository(ctxB, attempt.RepositoryId)
	if err != nil || repo.LatestSnapshotId != attempt.Id {
		t.Fatalf("other repository = %+v, err = %v", repo, err)
	}
}
