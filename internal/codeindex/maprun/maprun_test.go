package maprun

import (
	"context"
	"path/filepath"
	"testing"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	wsstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestBuildFileInputsUsesStableKeys(t *testing.T) {
	const keyA = "fact|a.go|FACT_KIND_FILE|a.go"
	const keyB = "fact|b.go|FACT_KIND_FILE|b.go"
	facts := []*codeindexv1.CodeFact{
		{Id: "snap-1-file-a", LogicalKey: keyA, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Name: "a.go", Anchor: &codeindexv1.SourceAnchor{Path: "a.go"}},
		{Id: "snap-1-file-b", LogicalKey: keyB, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Name: "b.go", Anchor: &codeindexv1.SourceAnchor{Path: "b.go"}},
	}
	edges := []cstore.FileEdge{{FromFactID: "snap-1-file-a", ToFactID: "snap-1-file-b", Weight: 2}}
	imports := []cstore.FileImport{{FileFactID: "snap-1-file-a", Import: "fmt"}}

	files, mapEdges, mapImports := buildFileInputs(facts, edges, imports)

	if len(files) != 2 || files[0].ID != keyA || files[1].ID != keyB {
		t.Fatalf("files = %+v, want logical keys", files)
	}
	if len(mapEdges) != 1 || mapEdges[0].FromFactID != keyA || mapEdges[0].ToFactID != keyB {
		t.Fatalf("edges = %+v, want logical keys", mapEdges)
	}
	if len(mapImports) != 1 || mapImports[0].FileFactID != keyA {
		t.Fatalf("imports = %+v, want logical keys", mapImports)
	}
}

func TestStableFactKeyFallsBackToStructuralKey(t *testing.T) {
	fact := &codeindexv1.CodeFact{
		Id:     "snap-1-file-a",
		Kind:   codeindexv1.FactKind_FACT_KIND_FILE,
		Name:   "a.go",
		Anchor: &codeindexv1.SourceAnchor{Path: "a.go"},
	}
	want := cgraph.LogicalFactKey(fact.GetKind(), "a.go", "a.go")
	if got := stableFactKey(fact); got != want {
		t.Fatalf("stableFactKey = %q, want %q", got, want)
	}
}

func TestRunRestoresHistoricalMapAndRetainsOwnership(t *testing.T) {
	ctx := context.Background()
	ws, err := wsstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	for _, name := range []string{"a", "b"} {
		g := cgraph.NewGraph("repo", name)
		for _, path := range []string{"shared.go", "only-" + name + ".go"} {
			g.AddFact(codeindexv1.FactKind_FACT_KIND_FILE, path, "go", &codeindexv1.SourceAnchor{Path: path}, "", "", nil)
		}
		if err := idx.Publish(ctx, "/repo", &codeindexv1.Snapshot{Id: name, RepositoryId: "repo"}, g); err != nil {
			t.Fatal(err)
		}
	}
	deps := Deps{Workspace: ws, Codeindex: idx, Options: mapconfig.FromGlobal(nil)}
	run := func(id string, wantCached bool) *codeindexv1.MapResult {
		t.Helper()
		result, cached, err := Run(ctx, deps, Request{RepositoryID: "repo", SnapshotID: id}, nil)
		if err != nil || cached != wantCached {
			t.Fatalf("map %s: cached=%v err=%v", id, cached, err)
		}
		var count int
		if err := ws.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = ?`, "only-"+id+".go").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("map %s missing its file: count=%d", id, count)
		}
		other := "a"
		if id == "a" {
			other = "b"
		}
		if err := ws.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = ?`, "only-"+other+".go").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("map %s still displays %s", id, other)
		}
		return result
	}
	first := run("a", false)
	run("b", false)
	restored := run("a", false)
	if first.ViewId != restored.ViewId {
		t.Fatal("map root identity changed")
	}
	run("a", true)
	mappings, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range mappings {
		if mapping.SnapshotID != "a" {
			t.Fatalf("reused mapping has stale provenance: %+v", mapping)
		}
	}
	run("b", false)
	mappings, err = idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range mappings {
		if mapping.SnapshotID != "b" {
			t.Fatalf("stale mapping provenance: %+v", mapping)
		}
	}
	if err := idx.DeleteSnapshot(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := idx.InvalidateActiveMap(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	run("b", false)
	current, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != len(mappings) {
		t.Fatal("snapshot deletion changed map ownership")
	}
	before := map[string]int64{}
	for _, mapping := range mappings {
		before[mapping.LogicalKey] = mapping.ResourceID
	}
	for _, mapping := range current {
		if before[mapping.LogicalKey] != mapping.ResourceID {
			t.Fatalf("duplicated resource: %s", mapping.LogicalKey)
		}
	}
	// Deleting the currently mapped snapshot detaches provenance but retains ownership.
	if err := idx.DeleteSnapshot(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	current, err = idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != len(mappings) {
		t.Fatal("active snapshot deletion lost ownership")
	}
	for _, mapping := range current {
		if mapping.SnapshotID != "" {
			t.Fatal("deleted snapshot provenance remains")
		}
	}
	active, err := idx.ActiveMap(ctx, "repo")
	if err != nil || active != nil {
		t.Fatalf("deleted active map: %+v %v", active, err)
	}
}

func TestEdgeKindLabel(t *testing.T) {
	cases := map[codeindexv1.EdgeKind]string{
		codeindexv1.EdgeKind_EDGE_KIND_CALLS:           "calls",
		codeindexv1.EdgeKind_EDGE_KIND_IMPLEMENTS:      "implements",
		codeindexv1.EdgeKind_EDGE_KIND_TYPE_DEFINITION: "type definition",
		// References dominate dependency graphs and stay unlabeled.
		codeindexv1.EdgeKind_EDGE_KIND_REFERENCES:  "",
		codeindexv1.EdgeKind_EDGE_KIND_UNSPECIFIED: "",
	}
	for kind, want := range cases {
		if got := edgeKindLabel(kind); got != want {
			t.Fatalf("edgeKindLabel(%v) = %q, want %q", kind, got, want)
		}
	}
}
