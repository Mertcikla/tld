package impact

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
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

// TestSceneTagsConnectorsSpanningChangedFilePairs proves workspace
// connectors whose endpoint files share a new dependency carry a change tag,
// while connectors over unchanged pairs stay untagged.
func TestSceneTagsConnectorsSpanningChangedFilePairs(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	mine, err := ws.CreateView(ctx, "mine", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	link := func(name, filePath string) int64 {
		t.Helper()
		element, err := ws.CreateElement(ctx, core.LibraryElement{Name: name, FilePath: stringPtr(filePath), RepositoryID: stringPtr("repo")})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.AddPlacement(ctx, mine.ID, element.ID, 0, 0); err != nil {
			t.Fatal(err)
		}
		return element.ID
	}
	aID := link("a", "a.go")
	bID := link("b", "b.go")
	cID := link("c", "c.go")
	dID := link("d", "d.go")
	// Folder links resolve by prefix: f anchors the pkg/ subtree, h the
	// file it will gain an edge to.
	fID := link("f", "pkg/")
	hID := link("h", "h.go")
	// Unlinked elements carry no source anchor, so their connectors can
	// never resolve to a file pair.
	unlinked, err := ws.CreateElement(ctx, core.LibraryElement{Name: "u", RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, mine.ID, unlinked.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	uID := unlinked.ID
	connect := func(sourceID, targetID int64) {
		t.Helper()
		if _, err := ws.CreateConnector(ctx, core.Connector{ViewID: mine.ID, SourceElementID: sourceID, TargetElementID: targetID, Label: stringPtr("calls")}); err != nil {
			t.Fatal(err)
		}
	}
	connect(aID, bID)
	connect(cID, dID)
	connect(fID, hID)
	connect(uID, aID)
	publish := func(id string, files map[string]string, edges [][2]string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		for _, edge := range edges {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[edge[0]].Id, facts[edge[1]].Id, "", facts[edge[0]].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", map[string]string{
		"a.go":     "func A() { return 1 }",
		"b.go":     "func B() {}",
		"c.go":     "func C() { return 1 }",
		"d.go":     "func D() {}",
		"pkg/i.go": "func I() {}",
		"h.go":     "func H() {}",
	}, nil)
	publish("head", map[string]string{
		"a.go":     "func A() { return 2 }",
		"b.go":     "func B2() {}",
		"c.go":     "func C() { return 2 }",
		"d.go":     "func D2() {}",
		"pkg/i.go": "func I() { return 1 }",
		"h.go":     "func H2() {}",
	}, [][2]string{{"a.go", "b.go"}, {"pkg/i.go", "h.go"}})
	diagram, err := Save(ctx, ws, idx, "repo", "key", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := (Service{Workspace: ws, Index: idx}).Scene(ctx, diagram)
	if err != nil {
		t.Fatal(err)
	}
	content := scene.GetViews()[strconv.FormatInt(mine.ID, 10)]
	if content == nil {
		t.Fatalf("scene missing authored view content: %+v", scene.GetViews())
	}
	tagsOf := map[[2]int32][]string{}
	for _, connector := range content.GetConnectors() {
		tagsOf[[2]int32{connector.GetSourceElementId(), connector.GetTargetElementId()}] = connector.GetTags()
	}
	if tags := tagsOf[[2]int32{int32(aID), int32(bID)}]; !hasTag(tags, "change:added") {
		t.Fatalf("connector over a new file edge is untagged: %+v", tags)
	}
	if tags := tagsOf[[2]int32{int32(cID), int32(dID)}]; hasChangeTag(tags) {
		t.Fatalf("connector over an unchanged pair carries a change tag: %+v", tags)
	}
	if tags := tagsOf[[2]int32{int32(fID), int32(hID)}]; !hasTag(tags, "change:added") {
		t.Fatalf("connector from a folder endpoint over a new file edge is untagged: %+v", tags)
	}
	if tags := tagsOf[[2]int32{int32(uID), int32(aID)}]; hasChangeTag(tags) {
		t.Fatalf("connector from an unlinked element carries a change tag: %+v", tags)
	}
}

func TestTransientNameKeepsParentDirectory(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]string{
		"go/api/api.go": "api/api.go",
		"go/api.go":     "go/api.go",
		"ts/client.ts":  "ts/client.ts",
		"main.go":       "main.go",
		"":              "",
	} {
		if got := transientName(path); got != want {
			t.Fatalf("transientName(%q) = %q, want %q", path, got, want)
		}
	}
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func hasChangeTag(tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "change:") {
			return true
		}
	}
	return false
}

// TestSceneAttachesOrphansToAuthoredNeighbourViews proves a directly changed
// file with no workspace placement is attached to the authored view whose
// linked files it borders in the head file graph, wired to the covering
// elements — instead of falling into the synthetic Changes view. Files with
// no authored neighbour still fall back.
func TestSceneAttachesOrphansToAuthoredNeighbourViews(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	mine, err := ws.CreateView(ctx, "mine", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cover, err := ws.CreateElement(ctx, core.LibraryElement{Name: "cover", FilePath: stringPtr("a.go"), RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, mine.ID, cover.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, files map[string]string, edges [][2]string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		for _, edge := range edges {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[edge[0]].Id, facts[edge[1]].Id, "", facts[edge[0]].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", map[string]string{"a.go": "func A() { return 1 }"}, nil)
	publish("head", map[string]string{
		"a.go":    "func A() { return 2 }",
		"new.go":  "func New() {}",
		"lone.go": "func Lone() {}",
	}, [][2]string{{"new.go", "a.go"}})
	diagram, err := Save(ctx, ws, idx, "repo", "key", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := (Service{Workspace: ws, Index: idx}).Scene(ctx, diagram)
	if err != nil {
		t.Fatal(err)
	}
	content := scene.GetViews()[strconv.FormatInt(mine.ID, 10)]
	if content == nil {
		t.Fatalf("scene missing authored view content: %+v", scene.GetViews())
	}
	var transientID int32
	foundCover := false
	for _, placement := range content.GetPlacements() {
		switch placement.GetElement().GetFilePath() {
		case "a.go":
			foundCover = true
		case "new.go":
			transientID = placement.GetElement().GetElementId()
			if transientID >= 0 {
				t.Fatalf("attached orphan is not transient: %+v", placement.GetElement())
			}
			if overlay := placement.GetOverlay(); overlay == nil || overlay.GetDistance() != 0 {
				t.Fatalf("attached orphan lost its change overlay: %+v", overlay)
			}
		case "lone.go":
			t.Fatalf("unrelated orphan leaked into the authored view: %+v", placement.GetElement())
		}
	}
	if !foundCover || transientID == 0 {
		t.Fatalf("authored view missing cover or attached orphan: %+v", content.GetPlacements())
	}
	wired := false
	for _, connector := range content.GetConnectors() {
		if connector.GetSourceElementId() == int32(cover.ID) && connector.GetTargetElementId() == transientID {
			wired = true
			if label := connector.GetLabel(); !strings.Contains(label, "dependencies") {
				t.Fatalf("attachment connector lost its weight label: %q", label)
			}
		}
	}
	if !wired {
		t.Fatalf("orphan not wired to its covering element: %+v", content.GetConnectors())
	}
	// The file with no authored neighbour still falls back to Changes.
	if scene.GetFallbackViewId() == 0 {
		t.Fatalf("expected a fallback view for the unrelated orphan")
	}
	fallback := scene.GetViews()[strconv.FormatInt(scene.GetFallbackViewId(), 10)]
	if fallback == nil || len(fallback.GetPlacements()) != 1 || fallback.GetPlacements()[0].GetElement().GetFilePath() != "lone.go" {
		t.Fatalf("fallback view = %+v", fallback)
	}
}

// TestSceneAttachesRemovedEdgeCounterparts proves a changed file whose only
// coupling was removed still attaches to the authored view covering its
// former neighbour — wired with the pair's removed kind, so the canvas draws
// the detachment arrow instead of stranding the file in Changes.
func TestSceneAttachesRemovedEdgeCounterparts(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	mine, err := ws.CreateView(ctx, "mine", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cover, err := ws.CreateElement(ctx, core.LibraryElement{Name: "cover", FilePath: stringPtr("a.go"), RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, mine.ID, cover.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, files map[string]string, edges [][2]string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		for _, edge := range edges {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[edge[0]].Id, facts[edge[1]].Id, "", facts[edge[0]].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", map[string]string{
		"a.go": "func A() {}",
		"b.go": "func B() {}",
	}, [][2]string{{"a.go", "b.go"}})
	// Both files change (so both stay diagram nodes) but the edge between
	// them is gone: head-only adjacency would strand b.go in Changes.
	publish("head", map[string]string{
		"a.go": "func A() {} // touched",
		"b.go": "func B() { return 1 }",
	}, nil)
	diagram, err := Save(ctx, ws, idx, "repo", "key", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := (Service{Workspace: ws, Index: idx}).Scene(ctx, diagram)
	if err != nil {
		t.Fatal(err)
	}
	content := scene.GetViews()[strconv.FormatInt(mine.ID, 10)]
	if content == nil {
		t.Fatalf("scene missing authored view content: %+v", scene.GetViews())
	}
	var transientID int32
	for _, placement := range content.GetPlacements() {
		if placement.GetElement().GetFilePath() == "b.go" {
			transientID = placement.GetElement().GetElementId()
			if transientID >= 0 {
				t.Fatalf("attached counterpart is not transient: %+v", placement.GetElement())
			}
		}
	}
	if transientID == 0 {
		t.Fatalf("removed-edge counterpart missing from authored view: %+v", content.GetPlacements())
	}
	for _, connector := range content.GetConnectors() {
		if connector.GetSourceElementId() == int32(cover.ID) && connector.GetTargetElementId() == transientID {
			if !hasTag(connector.GetTags(), "change:removed") {
				t.Fatalf("detachment wire lost its removed kind: %+v", connector.GetTags())
			}
			return
		}
	}
	t.Fatalf("counterpart not wired to its former neighbour: %+v", content.GetConnectors())
}

// TestSceneIncludesChildViewsOfImpactedElements proves an element on the
// impact diagram pulls its drill-down view into the scene recursively: the
// child view's output and navigation must be present so the canvas renders
// nested children instead of a singular flat element.
func TestSceneIncludesChildViewsOfImpactedElements(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	parent, err := ws.CreateView(ctx, "repo map", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ws.CreateElement(ctx, core.LibraryElement{Name: "a.go", FilePath: stringPtr("a.go"), RepositoryID: stringPtr("repo")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, parent.ID, changed.ID, 10, 20); err != nil {
		t.Fatal(err)
	}
	child, err := ws.CreateView(ctx, "a detail", nil, &changed.ID)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := ws.CreateElement(ctx, core.LibraryElement{Name: "inner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, child.ID, inner.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	grandchild, err := ws.CreateView(ctx, "inner detail", nil, &inner.ID)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := ws.CreateElement(ctx, core.LibraryElement{Name: "leaf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, grandchild.ID, leaf.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|view|repo", Kind: cstore.MappingView, ResourceID: parent.ID, RepositoryID: "repo", SnapshotID: "head"},
		{LogicalKey: "map|file|a", Kind: cstore.MappingElement, ResourceID: changed.ID, RepositoryID: "repo", SnapshotID: "head"},
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
	publish("base", map[string]string{"a.go": "func A() { return 1 }"})
	publish("head", map[string]string{"a.go": "func A() { return 2 }"})
	diagram, err := Save(ctx, ws, idx, "repo", "key", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := (Service{Workspace: ws, Index: idx}).Scene(ctx, diagram)
	if err != nil {
		t.Fatal(err)
	}
	// Neither drill-down view holds a repository placement, so without the
	// recursive expansion both would be pruned and the parent would render
	// flat.
	for _, id := range []int64{child.ID, grandchild.ID} {
		content := scene.GetViews()[strconv.FormatInt(id, 10)]
		if content == nil {
			t.Fatalf("scene missing drill-down view %d: %+v", id, scene.GetViews())
		}
		if len(content.GetPlacements()) == 0 {
			t.Fatalf("drill-down view %d has no output", id)
		}
	}
	linked := map[int64]bool{}
	for _, link := range scene.GetNavigations() {
		if link.GetRelationType() != "child" {
			continue
		}
		linked[int64(link.GetToViewId())] = true
	}
	for _, id := range []int64{child.ID, grandchild.ID} {
		if !linked[id] {
			t.Fatalf("scene missing child navigation to view %d: %+v", id, scene.GetNavigations())
		}
	}
}

func scopeScenePlacement(viewID, elementID int32, name string, distance uint32, changed bool) *pb.ScenePlacement {
	placement := &pb.ScenePlacement{
		Element: &diagv1.PlacedElement{
			Id: int32(elementID), ElementId: elementID, ViewId: viewID, Name: name,
		},
	}
	if changed {
		placement.Overlay = &pb.ImpactSceneOverlay{
			Change:   pb.ChangeKind_CHANGE_KIND_MODIFIED,
			Path:     name,
			Distance: distance,
		}
	}
	return placement
}

// ScopeScene at radius 0 keeps directly changed placements only, dropping
// unimpacted neighbours, dangling connectors, and emptied views.
func TestScopeSceneKeepsDirectChangesOnly(t *testing.T) {
	scene := &pb.ImpactScene{
		Views: map[string]*pb.SceneViewContent{
			"7": {
				Placements: []*pb.ScenePlacement{
					scopeScenePlacement(7, 1, "a.go", 0, true),
					scopeScenePlacement(7, 2, "b.go", 1, true),
					scopeScenePlacement(7, 3, "notes", 0, false),
				},
				Connectors: []*diagv1.Connector{
					{Id: 1, ViewId: 7, SourceElementId: 1, TargetElementId: 2},
					{Id: 2, ViewId: 7, SourceElementId: 1, TargetElementId: 3},
				},
			},
			"8": {
				Placements: []*pb.ScenePlacement{
					scopeScenePlacement(8, 9, "z.go", 0, false),
				},
			},
		},
		Tree:            []*diagv1.View{{Id: 7, Name: "kept"}, {Id: 8, Name: "dropped"}},
		AuthoredViewIds: []int64{7, 8},
	}
	scoped := ScopeScene(scene, 0)
	content := scoped.GetViews()["7"]
	if content == nil || len(content.GetPlacements()) != 1 || content.GetPlacements()[0].GetElement().GetElementId() != 1 {
		t.Fatalf("radius 0 placements = %+v", scoped.GetViews())
	}
	if len(content.GetConnectors()) != 0 {
		t.Fatalf("connectors without both endpoints must go: %+v", content.GetConnectors())
	}
	if _, ok := scoped.GetViews()["8"]; ok {
		t.Fatalf("emptied view must go: %+v", scoped.GetViews())
	}
	if len(scoped.GetTree()) != 1 || scoped.GetTree()[0].GetId() != 7 {
		t.Fatalf("tree = %+v", scoped.GetTree())
	}
	if len(scoped.GetAuthoredViewIds()) != 1 || scoped.GetAuthoredViewIds()[0] != 7 {
		t.Fatalf("authored views = %+v", scoped.GetAuthoredViewIds())
	}
	// The input scene is cloned, never mutated.
	if len(scene.GetViews()["7"].GetPlacements()) != 3 {
		t.Fatalf("ScopeScene mutated its input: %+v", scene.GetViews()["7"].GetPlacements())
	}
}

// ScopeScene at radius 1 additionally keeps context neighbours, and keeps
// the container elements that own retained drill-down views so nested change
// evidence never orphans its parent.
func TestScopeSceneKeepsNeighboursAndDrilldownContainers(t *testing.T) {
	scene := &pb.ImpactScene{
		Views: map[string]*pb.SceneViewContent{
			"7": {
				Placements: []*pb.ScenePlacement{
					scopeScenePlacement(7, 10, "service", 0, false),
					scopeScenePlacement(7, 11, "other", 0, false),
				},
			},
			"9": {
				Placements: []*pb.ScenePlacement{
					scopeScenePlacement(9, 1, "a.go", 0, true),
					scopeScenePlacement(9, 2, "b.go", 1, true),
					scopeScenePlacement(9, 3, "far.go", 2, true),
				},
				Connectors: []*diagv1.Connector{
					{Id: 1, ViewId: 9, SourceElementId: 1, TargetElementId: 2},
					{Id: 2, ViewId: 9, SourceElementId: 1, TargetElementId: 3},
				},
			},
		},
		Tree: []*diagv1.View{{Id: 7, Name: "parent", Children: []*diagv1.View{{Id: 9, Name: "child"}}}},
		Navigations: []*diagv1.ElementNavigationInfo{
			{FromViewId: 7, ToViewId: 9, RelationType: "child", ElementId: int32Ptr(10)},
		},
		AuthoredViewIds: []int64{7, 9},
	}
	scoped := ScopeScene(scene, 1)
	child := scoped.GetViews()["9"]
	if child == nil || len(child.GetPlacements()) != 2 {
		t.Fatalf("radius 1 placements = %+v", scoped.GetViews())
	}
	if len(child.GetConnectors()) != 1 || child.GetConnectors()[0].GetTargetElementId() != 2 {
		t.Fatalf("connectors = %+v", child.GetConnectors())
	}
	parent := scoped.GetViews()["7"]
	if parent == nil || len(parent.GetPlacements()) != 1 || parent.GetPlacements()[0].GetElement().GetElementId() != 10 {
		t.Fatalf("drill-down container must survive: %+v", scoped.GetViews())
	}
	if len(scoped.GetNavigations()) != 1 {
		t.Fatalf("navigations = %+v", scoped.GetNavigations())
	}
}

func int32Ptr(value int32) *int32 { return &value }
