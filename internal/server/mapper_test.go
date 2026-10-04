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
	graph.Facts["sym-a"] = &codeindexv1.CodeFact{Id: "sym-a", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Anchor: &codeindexv1.SourceAnchor{Path: "src/a.go"}}
	graph.Facts["sym-c"] = &codeindexv1.CodeFact{Id: "sym-c", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Anchor: &codeindexv1.SourceAnchor{Path: "src/deep/c.go"}}
	graph.AddEdgeFact(codeindexv1.EdgeKind_EDGE_KIND_CALLS, "sym-a", "sym-c", "", &codeindexv1.SourceAnchor{Path: "src/a.go"}, nil)
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
	for _, stage := range []string{"loading", "grouping", "materializing"} {
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

	maps, err := client.ListMaps(ctx, connect.NewRequest(&codeindexv1.ListMapsRequest{RepositoryId: repoID}))
	if err != nil || len(maps.Msg.Maps) != 1 || maps.Msg.Maps[0].Result.SnapshotId != snap.Id {
		t.Fatalf("completed maps: %+v: %v", maps, err)
	}
	reused, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID, SnapshotId: snap.Id}))
	if err != nil {
		t.Fatal(err)
	}
	for reused.Receive() {
		if reused.Msg().GetProgress() != nil {
			t.Fatal("cached map unexpectedly reran pipeline")
		}
		if reused.Msg().GetResult().GetRunId() != result.RunId {
			t.Fatal("cached map changed run")
		}
	}
	if err := reused.Err(); err != nil {
		t.Fatal(err)
	}
	invalid, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID, SnapshotId: snap.Id, WorkingTree: true}))
	if err == nil {
		for invalid.Receive() {
		}
		err = invalid.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("conflicting targets: %v", err)
	}

	var analysisRuns, groups, members int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_analysis_runs WHERE repository_id = ?`, repoID).Scan(&analysisRuns); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_groups WHERE run_id = ? AND kind = ?`, result.GetRunId(), codeindexv1.GroupKind_GROUP_KIND_COMMUNITY).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_group_members WHERE group_id IN (SELECT id FROM codeindex_groups WHERE run_id = ?)`, result.GetRunId()).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if analysisRuns != 1 || groups == 0 || members < 4 {
		t.Fatalf("analysis runs=%d groups=%d members=%d", analysisRuns, groups, members)
	}

	var connectors int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM connectors`).Scan(&connectors); err != nil {
		t.Fatal(err)
	}
	if connectors != 1 {
		t.Fatalf("connectors = %d, want 1 file-to-file edge", connectors)
	}
	var connectorEndpoints int
	if err := sqliteStore.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM connectors c
		JOIN elements s ON s.id = c.source_element_id
		JOIN elements t ON t.id = c.target_element_id
		WHERE s.name = 'a.go' AND t.name = 'c.go'`).Scan(&connectorEndpoints); err != nil {
		t.Fatal(err)
	}
	if connectorEndpoints != 1 {
		t.Fatalf("connector endpoints resolved = %d, want a.go -> deep/c.go", connectorEndpoints)
	}
}

func TestMapperServiceMaterializesImports(t *testing.T) {
	ctx := context.Background()
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/demo"
	repoID := cgraph.RepositoryID(root)
	snap := &codeindexv1.Snapshot{Id: "snap-imports", RepositoryId: repoID, CreatedUnix: 100}
	graph := cgraph.NewGraph(repoID, snap.Id)
	graph.Facts["f1"] = &codeindexv1.CodeFact{Id: "f1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Name: "a.go", Anchor: &codeindexv1.SourceAnchor{Path: "src/a.go"}}
	graph.Facts["f2"] = &codeindexv1.CodeFact{Id: "f2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Name: "b.go", Anchor: &codeindexv1.SourceAnchor{Path: "src/b.go"}}
	graph.Facts["s1"] = &codeindexv1.CodeFact{Id: "s1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Anchor: &codeindexv1.SourceAnchor{Path: "src/a.go"}, Imports: []string{"celery", "flask"}}
	if err := idx.Publish(ctx, root, snap, graph); err != nil {
		t.Fatalf("publish: %v", err)
	}
	for _, id := range []string{"f1", "f2"} {
		if err := idx.SaveFactEmbedding(ctx, &codeindexv1.Embedding{Id: "e" + id, FactId: id, SnapshotId: snap.Id, Profile: "p1", Dimensions: 3, Vector: []float32{1, 0, 0}}); err != nil {
			t.Fatalf("save embedding %s: %v", id, err)
		}
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID, IncludeImports: true}))
	if err != nil {
		t.Fatalf("map repository: %v", err)
	}
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}

	var connectors int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM connectors`).Scan(&connectors); err != nil {
		t.Fatal(err)
	}
	if connectors != 2 {
		t.Fatalf("import connectors = %d, want 2", connectors)
	}
	var external int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'External'`).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if external != 1 {
		t.Fatalf("External elements = %d, want 1", external)
	}
}

func TestMapperServiceGraphGroupingNeedsNoEmbeddings(t *testing.T) {
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
	if err != nil {
		t.Fatalf("map repository: %v", err)
	}
	var result *codeindexv1.MapResult
	for stream.Receive() {
		if mapped := stream.Msg().GetResult(); mapped != nil {
			result = mapped
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if result == nil || result.GetViewId() == 0 || result.GetFacts() != 1 {
		t.Fatalf("result = %+v, want a materialized map without embeddings", result)
	}
}

func TestMapperServiceEmbeddingModeRequiresEmbeddings(t *testing.T) {
	t.Setenv("TLD_MAP_GROUPING", "embedding")
	ctx := context.Background()
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/embedding"
	repoID := cgraph.RepositoryID(root)
	snap := &codeindexv1.Snapshot{Id: "snap-embedding", RepositoryId: repoID, CreatedUnix: 100}
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
		t.Fatal("expected an error when embedding mode runs without embeddings")
	}
}
