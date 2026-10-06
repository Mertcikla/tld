package impact

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestSceneAssemblesRepositoryMembersAndTransientChanges(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	view, err := ws.CreateView(ctx, "repo map", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := ws.CreateView(ctx, "other repo map", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ws.CreateElement(ctx, core.LibraryElement{Name: "a.go", FilePath: stringPtr("a.go"), RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	contextElement, err := ws.CreateElement(ctx, core.LibraryElement{Name: "b.go", FilePath: stringPtr("b.go"), RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := ws.CreateElement(ctx, core.LibraryElement{Name: "x.go", FilePath: stringPtr("x.go"), RepositoryID: stringPtr("other")})
	if err != nil {
		t.Fatal(err)
	}
	for _, placement := range []struct {
		viewID, elementID int64
		x, y              float64
	}{{view.ID, changed.ID, 10, 20}, {view.ID, contextElement.ID, 100, 20}, {unrelated.ID, foreign.ID, 0, 0}} {
		if _, err := ws.AddPlacement(ctx, placement.viewID, placement.elementID, placement.x, placement.y); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|view|repo", Kind: cstore.MappingView, ResourceID: view.ID, RepositoryID: "repo", SnapshotID: "head"},
		{LogicalKey: "map|view|other", Kind: cstore.MappingView, ResourceID: unrelated.ID, RepositoryID: "other", SnapshotID: "head"},
		{LogicalKey: "map|file|a", Kind: cstore.MappingElement, ResourceID: changed.ID, RepositoryID: "repo", SnapshotID: "head"},
		{LogicalKey: "map|file|b", Kind: cstore.MappingElement, ResourceID: contextElement.ID, RepositoryID: "repo", SnapshotID: "head"},
	}); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, files map[string]string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		if len(facts) > 2 {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts["a.go"].Id, facts["b.go"].Id, "", facts["a.go"].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", map[string]string{"a.go": "func A() { return 1 }", "b.go": "func B() {}"})
	publish("head", map[string]string{"a.go": "func A() { return 2 }", "b.go": "func B() {}", "new.go": "func C() {}"})
	diagram, err := Save(ctx, ws, idx, "repo", "key", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := (Service{Workspace: ws, Index: idx}).Scene(ctx, diagram)
	if err != nil {
		t.Fatal(err)
	}
	if len(scene.GetTree()) != 1 || int64(scene.GetTree()[0].GetId()) != view.ID {
		t.Fatalf("scene tree = %+v", scene.GetTree())
	}
	content := scene.GetViews()[strconv.FormatInt(view.ID, 10)]
	if content == nil {
		t.Fatalf("scene missing view content: %+v", scene.GetViews())
	}
	byPath := map[string]*pb.ScenePlacement{}
	for _, placement := range content.GetPlacements() {
		byPath[placement.GetElement().GetFilePath()] = placement
	}
	if overlay := byPath["a.go"].GetOverlay(); overlay == nil || overlay.GetChange() != pb.ChangeKind_CHANGE_KIND_MODIFIED || overlay.GetDistance() != 0 {
		t.Fatalf("changed overlay = %+v", overlay)
	}
	if overlay := byPath["b.go"].GetOverlay(); overlay == nil || overlay.GetDistance() != 1 {
		t.Fatalf("context overlay = %+v", overlay)
	}
	if _, ok := byPath["new.go"]; !ok {
		t.Fatalf("transient change missing: %+v", content.GetPlacements())
	}
	if got := scene.GetNavigations(); len(got) != 0 {
		t.Fatalf("unexpected navigations: %+v", got)
	}
	// A scene is a portable artifact: it names what it compared and carries
	// enough symbol detail to describe the change without the snapshot.
	if scene.GetSchemaVersion() != SceneSchemaVersion || scene.GetRepositoryId() != "repo" ||
		scene.GetComparisonKey() != "key" || scene.GetFromGitRevision() != "base" || scene.GetToGitRevision() != "head" {
		t.Fatalf("scene identity = %+v", scene)
	}
	if scene.GetMaxRadius() != 1 {
		t.Fatalf("scene max radius = %d, want the widest distance present", scene.GetMaxRadius())
	}
	overlay := byPath["a.go"].GetOverlay()
	symbols := overlay.GetSymbols()
	if len(symbols) != 1 || symbols[0].GetName() != "Stable" || symbols[0].GetChange() != pb.ChangeKind_CHANGE_KIND_MODIFIED {
		t.Fatalf("symbol detail = %+v", symbols)
	}
	if symbols[0].GetAnchor().GetPath() != "a.go" || symbols[0].GetKind() != pb.FactKind_FACT_KIND_FUNCTION {
		t.Fatalf("symbol detail lost its anchor or kind: %+v", symbols[0])
	}
	if symbols[0].GetBodyHash() == "" {
		t.Fatalf("symbol detail has no body fingerprint: %+v", symbols[0])
	}
}

func TestSceneMaxRadiusCountsOnlyDistancesPresent(t *testing.T) {
	scene := &pb.ImpactScene{Views: map[string]*pb.SceneViewContent{
		"1": {Placements: []*pb.ScenePlacement{
			{Overlay: &pb.ImpactSceneOverlay{Change: pb.ChangeKind_CHANGE_KIND_MODIFIED, Distance: 0}},
		}},
	}}
	if got := sceneMaxRadius(scene); got != 0 {
		t.Fatalf("direct-changes-only scene max radius = %d", got)
	}
	scene.Views["1"].Placements = append(scene.Views["1"].Placements, &pb.ScenePlacement{
		Overlay: &pb.ImpactSceneOverlay{Distance: 2},
	})
	if got := sceneMaxRadius(scene); got != 2 {
		t.Fatalf("scene max radius = %d, want 2", got)
	}
}
