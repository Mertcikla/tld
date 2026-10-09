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

// TestSceneAnnotatesAuthoredViewsAtCoarserGranularity proves the scene keeps a
// hand-drawn view that the generated map does not own, annotating its elements
// at whatever granularity they were linked at: an exact file, a symbol or line
// anchor, or a folder. Authored elements carry no repository id, exactly like
// CLI- and YAML-authored links, and changed files they cover must not also be
// emitted as transient placements.
func TestSceneAnnotatesAuthoredViewsAtCoarserGranularity(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	mapped, err := ws.CreateView(ctx, "repo map", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	authored, err := ws.CreateView(ctx, "system overview", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mappedFile, err := ws.CreateElement(ctx, core.LibraryElement{Name: "a.go", FilePath: stringPtr("a.go"), RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	link := func(name, filePath string) int64 {
		t.Helper()
		element, err := ws.CreateElement(ctx, core.LibraryElement{Name: name, FilePath: stringPtr(filePath)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.AddPlacement(ctx, authored.ID, element.ID, 0, 0); err != nil {
			t.Fatal(err)
		}
		return element.ID
	}
	link("service file", "a.go")
	link("stable symbol", "a.go#function:Stable")
	link("first lines", "a.go#L1-L3")
	link("subsystem", "sub/")
	link("subsystem bare", "sub")
	link("missing symbol", "a.go#function:Missing")
	link("unrelated file", "other.go")
	if _, err := ws.AddPlacement(ctx, mapped.ID, mappedFile.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|view|repo", Kind: cstore.MappingView, ResourceID: mapped.ID, RepositoryID: "repo", SnapshotID: "head"},
		{LogicalKey: "map|file|a", Kind: cstore.MappingElement, ResourceID: mappedFile.ID, RepositoryID: "repo", SnapshotID: "head"},
	}); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, files map[string]string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", map[string]string{"a.go": "func A() { return 1 }", "sub/deep.go": "func D() { return 1 }"})
	publish("head", map[string]string{"a.go": "func A() { return 2 }", "sub/deep.go": "func D() { return 2 }", "new.go": "func C() {}"})
	diagram, err := Save(ctx, ws, idx, "repo", "key", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := (Service{Workspace: ws, Index: idx}).Scene(ctx, diagram)
	if err != nil {
		t.Fatal(err)
	}
	if got := scene.GetAuthoredViewIds(); len(got) != 1 || got[0] != authored.ID {
		t.Fatalf("authored view ids = %+v, want [%d]", got, authored.ID)
	}
	content := scene.GetViews()[strconv.FormatInt(authored.ID, 10)]
	if content == nil {
		t.Fatalf("scene missing authored view: %+v", scene.GetViews())
	}
	byName := map[string]*pb.ScenePlacement{}
	for _, placement := range content.GetPlacements() {
		byName[placement.GetElement().GetName()] = placement
	}
	checkDirect := func(name string) {
		t.Helper()
		placement, ok := byName[name]
		if !ok {
			t.Fatalf("authored placement %q missing: %+v", name, content.GetPlacements())
		}
		overlay := placement.GetOverlay()
		if overlay == nil || overlay.GetChange() != pb.ChangeKind_CHANGE_KIND_MODIFIED || overlay.GetReason() != pb.OverlayReason_OVERLAY_REASON_DIRECT {
			t.Fatalf("authored overlay for %q = %+v", name, overlay)
		}
	}
	checkDirect("service file")
	checkDirect("stable symbol")
	checkDirect("first lines")
	for _, name := range []string{"missing symbol", "unrelated file"} {
		placement, ok := byName[name]
		if !ok {
			t.Fatalf("authored placement %q missing: %+v", name, content.GetPlacements())
		}
		if overlay := placement.GetOverlay(); overlay != nil {
			t.Fatalf("authored overlay for %q should be empty, got %+v", name, overlay)
		}
	}
	folder := byName["subsystem"].GetOverlay()
	if folder == nil || folder.GetReason() != pb.OverlayReason_OVERLAY_REASON_CONTAINED || folder.GetChange() != pb.ChangeKind_CHANGE_KIND_MODIFIED {
		t.Fatalf("folder roll-up overlay = %+v", folder)
	}
	if folder.GetPath() != "sub/" || len(folder.GetSymbols()) == 0 {
		t.Fatalf("folder roll-up lost its scope or symbols: %+v", folder)
	}
	// A folder link without the trailing slash rolls up the same way.
	bare := byName["subsystem bare"].GetOverlay()
	if bare == nil || bare.GetReason() != pb.OverlayReason_OVERLAY_REASON_CONTAINED || bare.GetChange() != pb.ChangeKind_CHANGE_KIND_MODIFIED {
		t.Fatalf("bare folder roll-up overlay = %+v", bare)
	}
	// The authored elements cover a.go and sub/deep.go, so only new.go may
	// still be emitted as a transient placement.
	seen := map[string]int{}
	for _, view := range scene.GetViews() {
		for _, placement := range view.GetPlacements() {
			seen[placement.GetElement().GetFilePath()]++
		}
	}
	if seen["a.go"] != 2 || seen["sub/deep.go"] != 0 {
		t.Fatalf("covered files duplicated as transients: %+v", seen)
	}
	if seen["new.go"] != 1 {
		t.Fatalf("uncovered change missing its transient placement: %+v", seen)
	}
}
