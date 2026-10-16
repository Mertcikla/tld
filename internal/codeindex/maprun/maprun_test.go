package maprun

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/community"
	cgraph "github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
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

func TestRunRejectsIncompatibleSnapshotBeforeCachedMap(t *testing.T) {
	ctx := context.Background()
	ws, err := wsstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	snap := &codeindexv1.Snapshot{Id: "snapshot", RepositoryId: "repo", IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions()}
	g := cgraph.NewGraph("repo", snap.Id)
	g.AddFact(codeindexv1.FactKind_FACT_KIND_FILE, "a.go", "go", &codeindexv1.SourceAnchor{Path: "a.go"}, "", "", nil)
	if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
		t.Fatal(err)
	}
	deps := Deps{Workspace: ws, Codeindex: idx, Options: mapconfig.FromGlobal(nil)}
	req := Request{RepositoryID: "repo", SnapshotID: snap.Id}
	if _, _, err := Run(ctx, deps, req, nil); err != nil {
		t.Fatal(err)
	}
	// Simulate an upgrade making the producer stamps obsolete while the map
	// remains cached. Validation must precede the active-map cache lookup.
	if _, err := ws.DB().ExecContext(ctx, `UPDATE codeindex_snapshots SET tool_versions_json = '{}' WHERE id = ?`, snap.Id); err != nil {
		t.Fatal(err)
	}
	result, cached, err := Run(ctx, deps, req, nil)
	if !errors.Is(err, indexer.ErrIncompatibleSnapshot) || cached || result != nil {
		t.Fatalf("incompatible cached map accepted: cached=%v result=%v err=%v", cached, result, err)
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
		if err := idx.Publish(ctx, "/repo", &codeindexv1.Snapshot{IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions(), Id: name, RepositoryId: "repo", GitBranch: "branch-" + name}, g); err != nil {
			t.Fatal(err)
		}
	}
	const remote = "https://github.com/acme/repo.git"
	if err := idx.SetRepositoryOrigin(ctx, "repo", remote, false); err != nil {
		t.Fatal(err)
	}
	deps := Deps{Workspace: ws, Codeindex: idx, Options: mapconfig.FromGlobal(nil)}
	run := func(id string, wantCached bool) *codeindexv1.MapResult {
		t.Helper()
		result, cached, err := Run(ctx, deps, Request{RepositoryID: "repo", SnapshotID: id}, nil)
		if err != nil || cached != wantCached {
			t.Fatalf("map %s: cached=%v err=%v", id, cached, err)
		}
		var repo, repositoryID, branch, description string
		if err := ws.DB().QueryRowContext(ctx,
			`SELECT e.repo, e.repository_id, e.branch, e.description FROM elements e JOIN views v ON v.owner_element_id = e.id WHERE v.id = ?`,
			result.ViewId).Scan(&repo, &repositoryID, &branch, &description); err != nil {
			t.Fatal(err)
		}
		if repo != remote || repositoryID != "repo" || branch != "branch-"+id || description != "Generated map for code grounding" {
			t.Fatalf("map %s repository metadata: repo=%q repository_id=%q branch=%q description=%q", id, repo, repositoryID, branch, description)
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
		if count > 0 {
			var tags string
			if err := ws.DB().QueryRowContext(ctx, `SELECT tags FROM elements WHERE name = ?`, "only-"+other+".go").Scan(&tags); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(tags, materialize.StaleTag) {
				t.Fatalf("map %s did not mark %s stale", id, other)
			}
		}
		var tags string
		if err := ws.DB().QueryRowContext(ctx, `SELECT tags FROM elements WHERE name = ?`, "only-"+id+".go").Scan(&tags); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(tags, materialize.StaleTag) {
			t.Fatalf("active file still stale: %s", id)
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
		if mapping.Kind == cstore.MappingElement {
			item, err := ws.ElementByID(ctx, mapping.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			if mapping.SnapshotID != "a" && !slices.Contains(item.Tags, materialize.StaleTag) {
				t.Fatalf("reused mapping has stale provenance: %+v", mapping)
			}
		}
	}
	run("b", false)
	mappings, err = idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range mappings {
		if mapping.Kind == cstore.MappingElement {
			item, err := ws.ElementByID(ctx, mapping.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			if mapping.SnapshotID != "b" && !slices.Contains(item.Tags, materialize.StaleTag) {
				t.Fatalf("stale mapping provenance: %+v", mapping)
			}
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

// TestAnalysisGroupsNeverDuplicateIDs guards the persisted codeindex_groups
// primary key: even a hierarchy with repeated group keys must emit unique ids.
func TestAnalysisGroupsNeverDuplicateIDs(t *testing.T) {
	files := []community.File{{ID: "f0"}, {ID: "f1"}}
	groups := []*community.Group{
		{Key: "same", Name: "parent", Files: 2, Children: []*community.Group{
			{Key: "same", Name: "child", Files: 2, Members: []int{0, 1}},
		}},
	}
	out := analysisGroups("run", groups, files)
	if len(out) != 2 {
		t.Fatalf("groups = %d, want 2", len(out))
	}
	seen := map[string]bool{}
	for _, group := range out {
		if seen[group.ID] {
			t.Fatalf("duplicate group id %q", group.ID)
		}
		seen[group.ID] = true
	}
}

func TestColdSnapshotsKeepCommunityResources(t *testing.T) {
	ctx := context.Background()
	ws, err := wsstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	deps := Deps{Workspace: ws, Codeindex: idx, Options: mapconfig.FromGlobal(nil)}
	var first *codeindexv1.MapResult
	var mappings map[string]int64
	for _, snapshot := range []string{"cold-a", "cold-b", "cold-c"} {
		g := cgraph.NewGraph("repo", snapshot)
		var ids []string
		for i := range 40 {
			path := fmt.Sprintf("pkg%d/file%d.go", i/10, i)
			f := g.AddFact(codeindexv1.FactKind_FACT_KIND_FILE, path, "go", &codeindexv1.SourceAnchor{Path: path}, "", "", nil)
			ids = append(ids, f.Id)
		}
		for i := range 40 {
			for j := i + 1; j < 40; j++ {
				if i/10 == j/10 || j == i+1 {
					g.AddEdgeFact(codeindexv1.EdgeKind_EDGE_KIND_REFERENCES, ids[i], ids[j], "", &codeindexv1.SourceAnchor{Path: fmt.Sprint(i), StartByte: uint32(j)})
				}
			}
		}
		if err := idx.Publish(ctx, "/repo", &codeindexv1.Snapshot{IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions(), Id: snapshot, RepositoryId: "repo"}, g); err != nil {
			t.Fatal(err)
		}
		res, cached, err := Run(ctx, deps, Request{RepositoryID: "repo", SnapshotID: snapshot}, nil)
		if err != nil || cached {
			t.Fatalf("run: cached=%v err=%v", cached, err)
		}
		got, err := idx.MappingsByRepository(ctx, "repo")
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = res
			mappings = make(map[string]int64, len(got))
			for _, m := range got {
				mappings[m.LogicalKey] = m.ResourceID
			}
		} else {
			if res.Clusters != first.Clusters || res.Bins != first.Bins || res.WeightedTightness != first.WeightedTightness {
				t.Fatalf("cold map grouping changed: %v vs %v", res, first)
			}
			if len(got) != len(mappings) {
				t.Fatal("cold map changed resource count")
			}
			for _, m := range got {
				if mappings[m.LogicalKey] != m.ResourceID {
					t.Fatalf("cold map recreated resource %s", m.LogicalKey)
				}
			}
		}
	}
}
