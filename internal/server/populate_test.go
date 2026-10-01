package server

import (
	"context"
	"errors"
	"testing"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestCanonicalizeDedupesSameLogicalKeyOnly(t *testing.T) {
	projection := project.Result{
		SnapshotID: "snap",
		Elements: []project.Element{
			{Ref: "fact|a.go|FUNCTION|Hello", Name: "Hello", Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, FilePath: "a.go"},
			{Ref: "fact|a.go|FUNCTION|Hello", Name: "Hello", Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, FilePath: "a.go"},
			{Ref: "fact|b.go|FUNCTION|Hello", Name: "Hello", Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, FilePath: "b.go"},
		},
		Connectors: []project.Connector{
			{Ref: "e1", Kind: codeindexv1.EdgeKind_EDGE_KIND_CALLS, FromRef: "fact|b.go|FUNCTION|Hello", ToRef: "fact|a.go|FUNCTION|Hello"},
			{Ref: "e2", Kind: codeindexv1.EdgeKind_EDGE_KIND_CALLS, FromRef: "fact|b.go|FUNCTION|Hello", ToRef: "fact|a.go|FUNCTION|Hello"},
		},
	}

	got := canonicalize(projection)
	if len(got.Elements) != 2 {
		t.Fatalf("elements = %d, want 2 (same logical key merged, same name kept): %+v", len(got.Elements), got.Elements)
	}
	if len(got.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1 (parallel edges merged): %+v", len(got.Connectors), got.Connectors)
	}
}

func TestDedupeResultsPrefersDirectMatch(t *testing.T) {
	results := dedupeResults([]populateElementResult{
		{ID: 1, Name: "Hello", MatchReason: "neighbor", ViaKind: "calls"},
		{ID: 1, Name: "Hello", MatchReason: "vector", SimilarityScore: 0.9},
		{ID: 2, Name: "main", MatchReason: "neighbor"},
	})
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2: %+v", len(results), results)
	}
	if results[0].ID != 1 || results[0].MatchReason != "vector" {
		t.Fatalf("dedupe should keep the direct match: %+v", results[0])
	}
}

func newPopulateDeps(t *testing.T) (*populateDeps, *localstore.SQLiteStore) {
	t.Helper()
	sqliteStore, _ := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	return &populateDeps{db: sqliteStore.DB(), ws: sqliteStore, idx: idx}, sqliteStore
}

func publishRepo(t *testing.T, idx *cstore.Store, repoID, snapID string) {
	t.Helper()
	ctx := context.Background()
	g := cgraph.NewGraph(repoID, snapID)
	if err := idx.Publish(ctx, "/"+repoID, &codeindexv1.Snapshot{Id: snapID, RepositoryId: repoID}, g); err != nil {
		t.Fatalf("publish %s: %v", repoID, err)
	}
}

func TestResolveRepositoryWithoutIndexedRepositories(t *testing.T) {
	deps, _ := newPopulateDeps(t)
	if _, err := deps.resolveRepository(context.Background(), 1); !errors.Is(err, errPopulateNoRepository) {
		t.Fatalf("err = %v, want errPopulateNoRepository", err)
	}
}

func TestResolveRepositoryFallsBackToSingleIndexedRepository(t *testing.T) {
	deps, _ := newPopulateDeps(t)
	publishRepo(t, deps.idx, "repo-a", "snap-a")

	got, err := deps.resolveRepository(context.Background(), 1)
	if err != nil {
		t.Fatalf("resolveRepository: %v", err)
	}
	if got != "repo-a" {
		t.Fatalf("repository = %q, want repo-a", got)
	}
}

func TestResolveRepositoryAmbiguousWithoutViewLink(t *testing.T) {
	deps, _ := newPopulateDeps(t)
	publishRepo(t, deps.idx, "repo-a", "snap-a")
	publishRepo(t, deps.idx, "repo-b", "snap-b")

	if _, err := deps.resolveRepository(context.Background(), 1); !errors.Is(err, errPopulateAmbiguousRepository) {
		t.Fatalf("err = %v, want errPopulateAmbiguousRepository", err)
	}
}

func TestResolveRepositoryPrefersViewMapping(t *testing.T) {
	deps, _ := newPopulateDeps(t)
	publishRepo(t, deps.idx, "repo-a", "snap-a")
	publishRepo(t, deps.idx, "repo-b", "snap-b")
	if err := deps.idx.SaveMappings(context.Background(), []cstore.ResourceMapping{
		{LogicalKey: "view|repo-b", Kind: cstore.MappingView, ResourceID: 7, RepositoryID: "repo-b", SnapshotID: "snap-b"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := deps.resolveRepository(context.Background(), 7)
	if err != nil {
		t.Fatalf("resolveRepository: %v", err)
	}
	if got != "repo-b" {
		t.Fatalf("repository = %q, want repo-b", got)
	}
}

func TestResolveRepositoryUsesPlacedElementMappings(t *testing.T) {
	deps, sqliteStore := newPopulateDeps(t)
	publishRepo(t, deps.idx, "repo-a", "snap-a")
	publishRepo(t, deps.idx, "repo-b", "snap-b")
	ctx := context.Background()

	view, err := sqliteStore.CreateView(ctx, "My View", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	element, err := sqliteStore.CreateElement(ctx, core.LibraryElement{Name: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqliteStore.AddPlacement(ctx, view.ID, element.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := deps.idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "fact|hello", Kind: cstore.MappingElement, ResourceID: element.ID, RepositoryID: "repo-b", SnapshotID: "snap-b"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := deps.resolveRepository(ctx, view.ID)
	if err != nil {
		t.Fatalf("resolveRepository: %v", err)
	}
	if got != "repo-b" {
		t.Fatalf("repository = %q, want repo-b", got)
	}
}
