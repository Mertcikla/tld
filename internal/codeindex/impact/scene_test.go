package impact

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
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
	connect := func(sourceID, targetID int64) {
		t.Helper()
		if _, err := ws.CreateConnector(ctx, core.Connector{ViewID: mine.ID, SourceElementID: sourceID, TargetElementID: targetID, Label: stringPtr("calls")}); err != nil {
			t.Fatal(err)
		}
	}
	connect(aID, bID)
	connect(cID, dID)
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
		"a.go": "func A() { return 1 }",
		"b.go": "func B() {}",
		"c.go": "func C() { return 1 }",
		"d.go": "func D() {}",
	}, nil)
	publish("head", map[string]string{
		"a.go": "func A() { return 2 }",
		"b.go": "func B2() {}",
		"c.go": "func C() { return 2 }",
		"d.go": "func D2() {}",
	}, [][2]string{{"a.go", "b.go"}})
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
