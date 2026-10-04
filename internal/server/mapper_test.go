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
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/proto"
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

func TestMapperServiceMapConfigChangeReruns(t *testing.T) {
	ctx := context.Background()
	cfg := workspace.DefaultConfig()
	sqliteStore, routes := newTestServerWithOptions(t, uuid.New(), nil, Options{Config: cfg})
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/config"
	repoID := cgraph.RepositoryID(root)
	snap := &codeindexv1.Snapshot{Id: "snap-config", RepositoryId: repoID, CreatedUnix: 100}
	graph := cgraph.NewGraph(repoID, snap.Id)
	graph.Facts["f1"] = &codeindexv1.CodeFact{Id: "f1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Anchor: &codeindexv1.SourceAnchor{Path: "src/a.go"}}
	graph.Facts["f2"] = &codeindexv1.CodeFact{Id: "f2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Anchor: &codeindexv1.SourceAnchor{Path: "src/b.go"}}
	graph.Facts["s1"] = &codeindexv1.CodeFact{Id: "s1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Anchor: &codeindexv1.SourceAnchor{Path: "src/a.go"}}
	graph.Facts["s2"] = &codeindexv1.CodeFact{Id: "s2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: codeindexv1.FactKind_FACT_KIND_FUNCTION, Anchor: &codeindexv1.SourceAnchor{Path: "src/b.go"}}
	graph.AddEdgeFact(codeindexv1.EdgeKind_EDGE_KIND_CALLS, "s1", "s2", "", &codeindexv1.SourceAnchor{Path: "src/a.go"}, nil)
	if err := idx.Publish(ctx, root, snap, graph); err != nil {
		t.Fatalf("publish: %v", err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")

	mapOnce := func() (*codeindexv1.MapResult, int) {
		stream, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID}))
		if err != nil {
			t.Fatalf("map: %v", err)
		}
		var result *codeindexv1.MapResult
		progress := 0
		for stream.Receive() {
			if stream.Msg().GetProgress() != nil {
				progress++
			}
			if mapped := stream.Msg().GetResult(); mapped != nil {
				result = mapped
			}
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream: %v", err)
		}
		return result, progress
	}

	first, firstProgress := mapOnce()
	if first == nil || firstProgress == 0 {
		t.Fatalf("first map = %+v progress=%d", first, firstProgress)
	}

	cfg.Map.Grouping.Resolution = 2.5
	second, secondProgress := mapOnce()
	if second == nil || second.RunId == first.RunId {
		t.Fatalf("config change did not rerun: %+v vs %+v", first, second)
	}
	if secondProgress == 0 {
		t.Fatal("expected progress events on config change")
	}

	third, thirdProgress := mapOnce()
	if third == nil || third.RunId != second.RunId || thirdProgress != 0 {
		t.Fatalf("cached rerun: result=%+v progress=%d", third, thirdProgress)
	}
	repositoryClient := codeindexv1connect.NewRepositoryServiceClient(ts.Client(), ts.URL+"/api")
	if _, err := repositoryClient.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&codeindexv1.UpdateRepositoryMapConfigurationRequest{RepositoryId: repoID, Overrides: &codeindexv1.RepositoryMapConfiguration{Resolution: proto.Float64(3)}})); err != nil {
		t.Fatal(err)
	}
	fourth, fourthProgress := mapOnce()
	if fourth == nil || fourth.RunId == third.RunId || fourthProgress == 0 {
		t.Fatalf("repository overrides reused a map built with global defaults: result=%+v progress=%d", fourth, fourthProgress)
	}
	if _, err := repositoryClient.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&codeindexv1.UpdateRepositoryMapConfigurationRequest{RepositoryId: repoID})); err != nil {
		t.Fatal(err)
	}
	restored, restoredProgress := mapOnce()
	if restored == nil || restored.RunId != third.RunId || restoredProgress == 0 {
		t.Fatalf("reset did not restore the matching global map: result=%+v progress=%d", restored, restoredProgress)
	}
}

func TestMapperServiceMaterializesBoundedExternalImports(t *testing.T) {
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
	if err := idx.SaveRepositoryMapOverrides(ctx, repoID, &codeindexv1.RepositoryMapConfiguration{IncludeExternalImports: proto.Bool(true)}); err != nil {
		t.Fatalf("enable external imports: %v", err)
	}

	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.MapRepository(ctx, connect.NewRequest(&codeindexv1.MapRepositoryRequest{RepositoryId: repoID}))
	if err != nil {
		t.Fatalf("map repository: %v", err)
	}
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}

	// External imports materialize as a single External container plus one
	// element per distinct import. Importing components connect to the container,
	// so the connector count stays bounded by the per-view budget rather than
	// growing with files × imports.
	var connectors int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM connectors`).Scan(&connectors); err != nil {
		t.Fatal(err)
	}
	if connectors < 1 || connectors > materialize.DefaultMaxConnectorsPerView {
		t.Fatalf("import connectors = %d, want between 1 and %d", connectors, materialize.DefaultMaxConnectorsPerView)
	}
	var external int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'External'`).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if external != 1 {
		t.Fatalf("External elements = %d, want 1", external)
	}
	for _, name := range []string{"celery", "flask"} {
		var count int
		if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = ?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s elements = %d, want 1", name, count)
		}
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
