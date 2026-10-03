package server

import (
	"context"
	"net/http/httptest"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestCodeIndexFactServiceSnapshotsAndDiff(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	ctx := context.Background()

	root := "/repo"
	repoID := cgraph.RepositoryID(root)

	base := &codeindexv1.Snapshot{
		Id:           "snap-1",
		RepositoryId: repoID,
		CreatedUnix:  100,
		GitRevision:  "aaaa",
		GitBranch:    "main",
		Sources:      []*codeindexv1.SourceFile{{Path: "a.go", Hash: "h1", Size: 10}},
	}
	baseGraph := cgraph.NewGraph(repoID, base.Id)
	baseGraph.Facts["f1"] = &codeindexv1.CodeFact{
		Id: "f1", RepositoryId: repoID, SnapshotId: base.Id, LogicalKey: "l1",
		Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Name: "A", Code: "func A() {}",
		Anchor: &codeindexv1.SourceAnchor{Path: "a.go", SourceHash: "h1"},
	}
	if err := idx.Publish(ctx, root, base, baseGraph); err != nil {
		t.Fatalf("publish base: %v", err)
	}

	next := &codeindexv1.Snapshot{
		Id:           "snap-2",
		RepositoryId: repoID,
		CreatedUnix:  200,
		GitRevision:  "bbbb",
		GitBranch:    "main",
		Sources: []*codeindexv1.SourceFile{
			{Path: "a.go", Hash: "h2", Size: 14},
			{Path: "b.go", Hash: "h3", Size: 5},
		},
	}
	nextGraph := cgraph.NewGraph(repoID, next.Id)
	nextGraph.Facts["f1b"] = &codeindexv1.CodeFact{
		Id: "f1b", RepositoryId: repoID, SnapshotId: next.Id, LogicalKey: "l1",
		Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Name: "A", Code: "func A() int { return 1 }",
		Anchor: &codeindexv1.SourceAnchor{Path: "a.go", SourceHash: "h2"},
	}
	nextGraph.Facts["f2"] = &codeindexv1.CodeFact{
		Id: "f2", RepositoryId: repoID, SnapshotId: next.Id, LogicalKey: "l2",
		Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Name: "B", Code: "func B() {}",
		Anchor: &codeindexv1.SourceAnchor{Path: "b.go", SourceHash: "h3"},
	}
	if err := idx.Publish(ctx, root, next, nextGraph); err != nil {
		t.Fatalf("publish next: %v", err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeFactServiceClient(ts.Client(), ts.URL+"/api")

	snaps, err := client.ListSnapshots(ctx, connect.NewRequest(&codeindexv1.RepositoryID{Id: repoID}))
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if got := len(snaps.Msg.GetSnapshots()); got != 2 {
		t.Fatalf("snapshots = %d, want 2", got)
	}
	for _, snap := range snaps.Msg.GetSnapshots() {
		if len(snap.GetSources()) != 0 {
			t.Fatalf("snapshot %s sources should be omitted, got %d", snap.GetId(), len(snap.GetSources()))
		}
		expected := uint32(1)
		if snap.Id == next.Id {
			expected = 2
		}
		if snap.Statistics == nil || snap.Statistics.Facts != expected || snap.Statistics.Sources != expected || snap.Statistics.Edges != 0 || snap.Statistics.Chunks != 0 {
			t.Fatalf("snapshot %s statistics: %+v", snap.Id, snap.Statistics)
		}
	}

	repo, err := client.GetRepository(ctx, connect.NewRequest(&codeindexv1.RepositoryID{Id: repoID}))
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if repo.Msg.GetLatestSnapshotId() != "snap-2" {
		t.Fatalf("latest = %q, want snap-2", repo.Msg.GetLatestSnapshotId())
	}

	diff, err := client.DiffSnapshots(ctx, connect.NewRequest(&codeindexv1.SnapshotDiffRequest{
		FromSnapshotId: "snap-1",
		ToSnapshotId:   "snap-2",
	}))
	if err != nil {
		t.Fatalf("DiffSnapshots: %v", err)
	}
	if got := len(diff.Msg.GetSources()); got != 2 {
		t.Fatalf("source changes = %d, want 2", got)
	}
	if got := len(diff.Msg.GetFacts().GetAdded()); got != 1 {
		t.Fatalf("added facts = %d, want 1", got)
	}
	if got := len(diff.Msg.GetFacts().GetModified()); got != 1 {
		t.Fatalf("modified facts = %d, want 1", got)
	}
	if got := len(diff.Msg.GetFacts().GetRemoved()); got != 0 {
		t.Fatalf("removed facts = %d, want 0", got)
	}

	facts, err := client.ListFacts(ctx, connect.NewRequest(&codeindexv1.CodeFactFilter{SnapshotId: "snap-2"}))
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if got := len(facts.Msg.GetFacts()); got != 2 {
		t.Fatalf("facts = %d, want 2", got)
	}
}

func TestRepositoryServiceDeleteRepository(t *testing.T) {
	workspaceID := uuid.New()
	sqliteStore, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)

	root := "/repo-delete"
	repoID := cgraph.RepositoryID(root)
	snap := &codeindexv1.Snapshot{Id: "snap-del", RepositoryId: repoID, CreatedUnix: 100}
	if err := idx.Publish(ctx, root, snap, cgraph.NewGraph(repoID, snap.Id)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	view, err := sqliteStore.CreateView(ctx, "Repo Map", nil, nil)
	if err != nil {
		t.Fatalf("create view: %v", err)
	}
	element, err := sqliteStore.CreateElement(ctx, core.LibraryElement{Name: "Repo Root"})
	if err != nil {
		t.Fatalf("create element: %v", err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|view|" + repoID, Kind: cstore.MappingView, ResourceID: view.ID, RepositoryID: repoID, SnapshotID: snap.Id},
		{LogicalKey: "map|top|" + repoID, Kind: cstore.MappingElement, ResourceID: element.ID, RepositoryID: repoID, SnapshotID: snap.Id},
	}); err != nil {
		t.Fatalf("save mappings: %v", err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(ts.Client(), ts.URL+"/api")

	if _, err := client.DeleteRepository(ctx, connect.NewRequest(&codeindexv1.DeleteRepositoryRequest{})); err == nil {
		t.Fatal("expected an error for a missing repository id")
	}
	if _, err := client.DeleteRepository(ctx, connect.NewRequest(&codeindexv1.DeleteRepositoryRequest{Id: "missing"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing repository error = %v, want not found", err)
	}

	if _, err := client.DeleteRepository(ctx, connect.NewRequest(&codeindexv1.DeleteRepositoryRequest{Id: repoID, DeleteMaterialized: true})); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}

	if _, err := idx.Repository(ctx, repoID); err == nil {
		t.Fatal("repository still resolves after delete")
	}
	if _, err := sqliteStore.ViewByID(ctx, view.ID); err == nil {
		t.Fatal("materialized view still resolves after delete")
	}
	if _, err := sqliteStore.ElementByID(ctx, element.ID); err == nil {
		t.Fatal("materialized element still resolves after delete")
	}
	mappings, err := idx.MappingsByRepository(ctx, repoID)
	if err != nil {
		t.Fatalf("mappings: %v", err)
	}
	if len(mappings) != 0 {
		t.Fatalf("mappings remain: %d", len(mappings))
	}
}
