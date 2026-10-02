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
)

func TestMapperServiceMapRepository(t *testing.T) {
	ctx := context.Background()
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/demo"
	repoID := cgraph.RepositoryID(root)
	snap := &codeindexv1.Snapshot{Id: "snap-1", RepositoryId: repoID, CreatedUnix: 100}
	graph := cgraph.NewGraph(repoID, snap.Id)
	paths := []string{"src/a.go", "src/b.go", "src/deep/c.go", "src/deep/d.go"}
	for i, path := range paths {
		id := "f" + string(rune('1'+i))
		graph.Facts[id] = &codeindexv1.CodeFact{
			Id: id, RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FILE,
			Name: "file " + path, Language: "go", Anchor: &codeindexv1.SourceAnchor{Path: path},
		}
	}
	if err := idx.Publish(ctx, root, snap, graph); err != nil {
		t.Fatalf("publish: %v", err)
	}
	for i := range paths {
		id := "f" + string(rune('1'+i))
		embedding := &codeindexv1.Embedding{Id: "e" + id, FactId: id, SnapshotId: snap.Id, Profile: "p1", Dimensions: 3, Vector: []float32{1, 0, 0}}
		if err := idx.SaveFactEmbedding(ctx, embedding); err != nil {
			t.Fatalf("save embedding %s: %v", id, err)
		}
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")

	stream, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID}))
	if err != nil {
		t.Fatalf("map repository: %v", err)
	}
	stages := map[string]bool{}
	var result *codeindexv1.MapResult
	for stream.Receive() {
		event := stream.Msg()
		if progress := event.GetProgress(); progress != nil {
			stages[progress.GetStage()] = true
		}
		if mapped := event.GetResult(); mapped != nil {
			result = mapped
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	for _, stage := range []string{"loading", "clustering", "binning", "materializing"} {
		if !stages[stage] {
			t.Fatalf("missing progress stage %q (got %v)", stage, stages)
		}
	}
	if result == nil {
		t.Fatal("map stream ended without a result")
	}
	if result.GetViewId() == 0 || result.GetFacts() != 4 || result.GetClusters() == 0 || result.GetBins() == 0 {
		t.Fatalf("result = %+v", result)
	}
	if result.GetRunId() == "" {
		t.Fatal("result has no run id")
	}

	var analysisRuns, groups, members int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_analysis_runs WHERE repository_id = ?`, repoID).Scan(&analysisRuns); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_groups WHERE run_id = ?`, result.GetRunId()).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_group_members WHERE group_id IN (SELECT id FROM codeindex_groups WHERE run_id = ?)`, result.GetRunId()).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if analysisRuns != 1 || groups == 0 || members != 4 {
		t.Fatalf("analysis runs=%d groups=%d members=%d", analysisRuns, groups, members)
	}
}

func TestMapperServiceRequiresEmbeddings(t *testing.T) {
	ctx := context.Background()
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/empty"
	repoID := cgraph.RepositoryID(root)
	snap := &codeindexv1.Snapshot{Id: "snap-empty", RepositoryId: repoID, CreatedUnix: 100}
	graph := cgraph.NewGraph(repoID, snap.Id)
	graph.Facts["f1"] = &codeindexv1.CodeFact{Id: "f1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Anchor: &codeindexv1.SourceAnchor{Path: "a.go"}}
	if err := idx.Publish(ctx, root, snap, graph); err != nil {
		t.Fatalf("publish: %v", err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")

	stream, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if err == nil {
		t.Fatal("expected an error when the snapshot has no embeddings")
	}
}
