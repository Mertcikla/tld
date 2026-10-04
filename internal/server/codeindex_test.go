package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestListSnapshotsExcludesWorkingTree(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	ctx := context.Background()

	root := "/repo-snapshots"
	repoID := cgraph.RepositoryID(root)
	saved := &codeindexv1.Snapshot{Id: "saved", RepositoryId: repoID, CreatedUnix: 100, Provenance: "commit", CommitMessage: "feat: add thing"}
	if err := idx.Publish(ctx, root, saved, cgraph.NewGraph(repoID, saved.Id)); err != nil {
		t.Fatalf("publish saved: %v", err)
	}
	live := &codeindexv1.Snapshot{Id: "live", RepositoryId: repoID, CreatedUnix: 200, Provenance: "working_tree", CommitMessage: "feat: add thing"}
	if err := idx.PublishHistorical(ctx, root, live, cgraph.NewGraph(repoID, live.Id)); err != nil {
		t.Fatalf("publish live: %v", err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeFactServiceClient(ts.Client(), ts.URL+"/api")

	snaps, err := client.ListSnapshots(ctx, connect.NewRequest(&codeindexv1.RepositoryID{Id: repoID}))
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	got := snaps.Msg.GetSnapshots()
	if len(got) != 1 || got[0].GetId() != saved.Id {
		t.Fatalf("snapshots = %+v, want only %s", got, saved.Id)
	}
	if got[0].GetCommitMessage() != saved.CommitMessage {
		t.Fatalf("commit message = %q, want %q", got[0].GetCommitMessage(), saved.CommitMessage)
	}
}

func TestRepositoryServiceAddRepository(t *testing.T) {
	workspaceID := uuid.New()
	sqliteStore, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	ctx := context.Background()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc A() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(ts.Client(), ts.URL+"/api")

	empty, err := client.AddRepository(ctx, connect.NewRequest(&codeindexv1.AddRepositoryRequest{}))
	if err == nil {
		for empty.Receive() {
		}
		err = empty.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty path error = %v, want invalid argument", err)
	}
	stream, err := client.AddRepository(ctx, connect.NewRequest(&codeindexv1.AddRepositoryRequest{Path: filepath.Join(dir, "missing")}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing directory error = %v, want invalid argument", err)
	}

	stream, err = client.AddRepository(ctx, connect.NewRequest(&codeindexv1.AddRepositoryRequest{Path: dir}))
	if err != nil {
		t.Fatalf("AddRepository: %v", err)
	}
	stages := map[string]bool{}
	var repository *codeindexv1.Repository
	for stream.Receive() {
		event := stream.Msg()
		if progress := event.GetProgress(); progress != nil {
			stages[progress.GetStage()] = true
		}
		if repo := event.GetRepository(); repo != nil {
			repository = repo
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if repository == nil {
		t.Fatal("stream ended without a repository")
	}
	repoID := cgraph.RepositoryID(resolved)
	if repository.GetId() != repoID || repository.GetRoot() != resolved {
		t.Fatalf("repository = %+v, want id %s root %s", repository, repoID, resolved)
	}
	if repository.GetLatestSnapshotId() == "" {
		t.Fatal("repository has no latest snapshot")
	}
	if len(stages) == 0 {
		t.Fatal("no indexing progress was streamed")
	}
	if _, err := idx.Repository(ctx, repoID); err != nil {
		t.Fatalf("repository not registered: %v", err)
	}
	snap, err := idx.Snapshot(ctx, repository.GetLatestSnapshotId())
	if err != nil {
		t.Fatalf("snapshot not published: %v", err)
	}
	if len(snap.GetSources()) != 1 || snap.GetSources()[0].GetPath() != "a.go" {
		t.Fatalf("snapshot sources = %+v", snap.GetSources())
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
