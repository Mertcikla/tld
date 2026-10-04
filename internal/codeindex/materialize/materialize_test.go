package materialize

import (
	"context"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/visibility"
	"github.com/mertcikla/tld/v2/internal/core"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func openStores(t *testing.T) (*localstore.SQLiteStore, *cstore.Store) {
	t.Helper()
	sq, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	return sq, cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
}

func allVisible(proj project.Result) []visibility.Decision {
	var ds []visibility.Decision
	for _, e := range proj.Elements {
		ds = append(ds, visibility.Decision{Ref: e.Ref, Kind: "element", Visible: true})
	}
	for _, c := range proj.Connectors {
		ds = append(ds, visibility.Decision{Ref: c.Ref, Kind: "connector", Visible: true})
	}
	return ds
}

func fixture() project.Result {
	return project.Result{
		SnapshotID: "snap-1",
		Elements: []project.Element{
			{Ref: "fact|a.go|FACT_KIND_FUNCTION|A", Name: "A", Kind: pb.FactKind_FACT_KIND_FUNCTION, Repository: "repo", FilePath: "a.go", Language: "go"},
			{Ref: "fact|b.go|FACT_KIND_FUNCTION|B", Name: "B", Kind: pb.FactKind_FACT_KIND_FUNCTION, Repository: "repo", FilePath: "b.go", Language: "go"},
		},
		Connectors: []project.Connector{
			{Ref: "edge|CALLS|A|B", Kind: pb.EdgeKind_EDGE_KIND_CALLS, FromRef: "fact|a.go|FACT_KIND_FUNCTION|A", ToRef: "fact|b.go|FACT_KIND_FUNCTION|B", Weight: 1},
		},
	}
}

func TestElementInputRecordsLineAnchorAndRoot(t *testing.T) {
	el := project.Element{
		Ref:      "fact|a.go|FACT_KIND_FUNCTION|A",
		Name:     "A",
		Kind:     pb.FactKind_FACT_KIND_FUNCTION,
		FilePath: "a.go",
		Language: "go",
		Anchor:   &pb.SourceAnchor{Path: "a.go", StartLine: 4, EndLine: 5, SourceHash: "h"},
	}
	got := elementInput(el, Options{RepositoryID: "repo", RepositoryName: "demo", RepositoryRoot: "/repos/demo"}, true)
	if got.FilePath == nil || *got.FilePath != "a.go#L5" {
		t.Fatalf("file_path = %v, want a.go#L5 (1-based line)", got.FilePath)
	}
	if got.Repo == nil || *got.Repo != "/repos/demo" {
		t.Fatalf("repo = %v, want /repos/demo", got.Repo)
	}
	if got.RepositoryID == nil || *got.RepositoryID != "repo" {
		t.Fatalf("repository_id = %v, want repo", got.RepositoryID)
	}
}

func TestMapFileElementStampsRepositoryID(t *testing.T) {
	m := &mapMaterializer{input: MapInput{
		RepositoryID:   "repo-1",
		RepositoryRoot: "/work/repo",
		Files:          []community.File{{ID: "f1", Path: "src/main.go", DisplayName: "main.go"}},
	}}
	got := m.fileElement(0)
	if got.RepositoryID == nil || *got.RepositoryID != "repo-1" {
		t.Fatalf("repository_id = %v, want repo-1", got.RepositoryID)
	}
	if got.Repo == nil || *got.Repo != "/work/repo" {
		t.Fatalf("repo = %v, want /work/repo", got.Repo)
	}
}

func TestApplyIdempotent(t *testing.T) {
	ctx := context.Background()
	ws, idx := openStores(t)
	opts := Options{RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1"}
	proj := fixture()

	first, err := Apply(ctx, ws, idx, proj, allVisible(proj), opts)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if first.Elements != 2 || first.Connectors != 1 || first.ViewID == 0 {
		t.Fatalf("first result = %+v", first)
	}

	second, err := Apply(ctx, ws, idx, proj, allVisible(proj), opts)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if second.ViewID != first.ViewID {
		t.Fatalf("view changed across runs: %d -> %d", first.ViewID, second.ViewID)
	}
	if second.Elements != 2 || second.Connectors != 1 || second.Pruned != 0 {
		t.Fatalf("second result = %+v", second)
	}

	placements, err := ws.Placements(ctx, first.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 2 {
		t.Fatalf("placements = %d, want 2", len(placements))
	}
	connectors, err := ws.Connectors(ctx, first.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connectors) != 1 {
		t.Fatalf("connectors = %d, want 1", len(connectors))
	}
	mappings, err := idx.MappingsByRepository(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	// 2 elements + 1 connector + 1 view.
	if len(mappings) != 4 {
		t.Fatalf("mappings = %d, want 4 (%+v)", len(mappings), mappings)
	}
}

func TestApplyVisibilityControlsNoiseGate(t *testing.T) {
	ctx := context.Background()
	ws, idx := openStores(t)
	opts := Options{RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1"}
	proj := fixture()
	decisions := []visibility.Decision{
		{Ref: "fact|a.go|FACT_KIND_FUNCTION|A", Kind: "element", Visible: true},
		{Ref: "fact|b.go|FACT_KIND_FUNCTION|B", Kind: "element", Visible: false},
	}

	if _, err := Apply(ctx, ws, idx, proj, decisions, opts); err != nil {
		t.Fatalf("apply: %v", err)
	}
	aMapping, _, err := idx.MappingByLogicalKey(ctx, "fact|a.go|FACT_KIND_FUNCTION|A")
	if err != nil {
		t.Fatal(err)
	}
	bMapping, _, err := idx.MappingByLogicalKey(ctx, "fact|b.go|FACT_KIND_FUNCTION|B")
	if err != nil {
		t.Fatal(err)
	}
	a, err := ws.ElementByID(ctx, aMapping.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ws.ElementByID(ctx, bMapping.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if !a.BypassNoiseGate {
		t.Fatal("visible element should bypass the noise gate")
	}
	if b.BypassNoiseGate {
		t.Fatal("hidden element should not bypass the noise gate")
	}
}

func TestApplyPrunesStale(t *testing.T) {
	ctx := context.Background()
	ws, idx := openStores(t)
	opts := Options{RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1"}
	proj := fixture()

	if _, err := Apply(ctx, ws, idx, proj, allVisible(proj), opts); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	bMapping, ok, err := idx.MappingByLogicalKey(ctx, "fact|b.go|FACT_KIND_FUNCTION|B")
	if err != nil || !ok {
		t.Fatalf("b mapping: ok=%v err=%v", ok, err)
	}

	// Second snapshot drops B entirely.
	reduced := project.Result{SnapshotID: "snap-2", Elements: proj.Elements[:1]}
	res, err := Apply(ctx, ws, idx, reduced, allVisible(reduced), opts)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	// B and the A->B connector are both stale once B disappears.
	if res.Pruned != 2 {
		t.Fatalf("pruned = %d, want 2", res.Pruned)
	}
	if _, err := ws.ElementByID(ctx, bMapping.ResourceID); err == nil {
		t.Fatal("pruned element still exists")
	}
	if _, ok, _ := idx.MappingByLogicalKey(ctx, "fact|b.go|FACT_KIND_FUNCTION|B"); ok {
		t.Fatal("pruned mapping still present")
	}
	// The view mapping survives pruning.
	if _, ok, _ := idx.MappingByLogicalKey(ctx, "view|repo-1"); !ok {
		t.Fatal("view mapping was pruned")
	}
}

func TestApplyPreservesUserEdits(t *testing.T) {
	ctx := context.Background()
	ws, idx := openStores(t)
	opts := Options{RepositoryID: "repo-1", RepositoryName: "demo", RepositoryRoot: "/repo/demo", SnapshotID: "snap-1"}
	proj := fixture()

	first, err := Apply(ctx, ws, idx, proj, allVisible(proj), opts)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	mapping, _, err := idx.MappingByLogicalKey(ctx, "fact|a.go|FACT_KIND_FUNCTION|A")
	if err != nil {
		t.Fatal(err)
	}

	// The user renames the mapped element and places it in their own view.
	if _, err := ws.UpdateElement(ctx, mapping.ResourceID, core.LibraryElement{Name: "My Label", Tags: []string{"mine"}}); err != nil {
		t.Fatalf("user edit: %v", err)
	}
	userView, err := ws.CreateView(ctx, "My Diagram", nil, nil)
	if err != nil {
		t.Fatalf("create user view: %v", err)
	}
	if _, err := ws.AddPlacement(ctx, userView.ID, mapping.ResourceID, 12, 34); err != nil {
		t.Fatalf("user placement: %v", err)
	}
	edgeMapping, _, err := idx.MappingByLogicalKey(ctx, "edge|CALLS|A|B")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.UpdateConnector(ctx, edgeMapping.ResourceID, core.Connector{Style: "straight"}); err != nil {
		t.Fatalf("user connector style: %v", err)
	}

	// A new snapshot changes the source anchor and re-materializes the same ref.
	opts.SnapshotID = "snap-2"
	updated := fixture()
	for i := range updated.Elements {
		if updated.Elements[i].Ref == "fact|a.go|FACT_KIND_FUNCTION|A" {
			updated.Elements[i].Anchor = &pb.SourceAnchor{Path: "a.go", StartLine: 40, EndLine: 41, SourceHash: "h2"}
		}
	}
	if _, err := Apply(ctx, ws, idx, updated, allVisible(updated), opts); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	got, err := ws.ElementByID(ctx, mapping.ResourceID)
	if err != nil {
		t.Fatalf("element: %v", err)
	}
	if got.Name != "My Label" {
		t.Fatalf("name = %q, want preserved %q", got.Name, "My Label")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "mine" {
		t.Fatalf("tags = %v, want preserved [mine]", got.Tags)
	}
	if got.FilePath == nil || *got.FilePath != "a.go#L41" {
		t.Fatalf("file_path = %v, want updated a.go#L41", got.FilePath)
	}
	placements, err := ws.ListElementPlacements(ctx, mapping.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, placement := range placements {
		if placement.ViewID == userView.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("user placement was lost after re-materialization")
	}
	connectors, err := ws.Connectors(ctx, first.ViewID)
	if err != nil {
		t.Fatalf("connectors: %v", err)
	}
	style := ""
	for _, connector := range connectors {
		if connector.ID == edgeMapping.ResourceID {
			style = connector.Style
		}
	}
	if style != "straight" {
		t.Fatalf("connector style = %q, want preserved straight", style)
	}
}
