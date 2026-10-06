package impact

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestImpactRadiusAndScopedMaterialization(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	baselineViews, err := ws.Views(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publish := func(id, code string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for _, path := range []string{"a.go", "b.go", "c.go", "gone.go"} {
			if id == "head" && path == "gone.go" {
				continue
			}
			text := "func Stable() {}"
			if path == "a.go" {
				text = code
			}
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			name := "Stable"
			if path == "a.go" {
				name = "Changed"
			}
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, name, "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		for _, link := range [][2]string{{"a.go", "b.go"}, {"b.go", "c.go"}} {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[link[0]].Id, facts[link[1]].Id, "", facts[link[0]].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", "func Changed() { return 1 }")
	publish("head", "func Changed() { return 2 }")
	contextElement, err := ws.CreateElement(ctx, core.LibraryElement{Name: "B", FilePath: stringPtr("b.go")})
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: "map|file|b", Kind: cstore.MappingElement, ResourceID: contextElement.ID, RepositoryID: "repo", SnapshotID: "head"}}); err != nil {
		t.Fatal(err)
	}
	diagram, err := Save(ctx, ws, idx, "repo", "live", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagram.Nodes) != 2 || diagram.MaxRadius != 1 {
		t.Fatalf("changed-only nodes: %+v", diagram)
	}
	for _, node := range diagram.Nodes {
		if node.Path == "a.go" && len(node.Symbols.Modified) != 1 {
			t.Fatal("modified symbol not shown")
		}
		if node.Path == "gone.go" && len(node.Symbols.Removed) != 1 {
			t.Fatal("removed symbol not shown")
		}
	}
	expanded, err := Save(ctx, ws, idx, "repo", "live", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded.Nodes) != 3 || expanded.ViewId != diagram.ViewId || len(expanded.Edges) != 1 {
		t.Fatalf("expanded impact: %+v", expanded)
	}
	for _, node := range expanded.Nodes {
		if node.Path == "c.go" {
			t.Fatal("unmaterialized unchanged file was introduced")
		}
		if node.Context && node.ElementId != contextElement.ID {
			t.Fatal("context did not reuse existing element")
		}
	}
	if _, err := ws.ElementByID(ctx, contextElement.ID); err != nil {
		t.Fatal("shared context was deleted")
	}
	historical, err := Save(ctx, ws, idx, "repo", "historical", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if historical.ViewId != 0 || diagram.ViewId != 0 {
		t.Fatal("comparison materialized a view")
	}
	empty, err := Save(ctx, ws, idx, "repo", "live", "head", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Nodes) != 0 {
		t.Fatal("clean overlay retained changes")
	}
	views, err := ws.Views(ctx)
	if err != nil || len(views) != len(baselineViews) {
		t.Fatalf("overlay created views: %+v %v", views, err)
	}
	elements, total, err := ws.Elements(ctx, 100, 0, "")
	if err != nil || total != 1 || elements[0].ID != contextElement.ID {
		t.Fatal("overlay mutated workspace elements")
	}
}
func stringPtr(s string) *string { return &s }

func TestImpactPlacesAddedFilesInClosestView(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	// Materialize a realistic Workspace -> repository -> Pkg view chain so the
	// impact hierarchy skips the two always-present top levels.
	root, err := ws.CreateView(ctx, "Workspace", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	repoOwner, err := ws.CreateElement(ctx, core.LibraryElement{Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, root.ID, repoOwner.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	repoView, err := ws.CreateView(ctx, "repo", nil, &repoOwner.ID)
	if err != nil {
		t.Fatal(err)
	}
	pkgOwner, err := ws.CreateElement(ctx, core.LibraryElement{Name: "Pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, repoView.ID, pkgOwner.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	view, err := ws.CreateView(ctx, "Pkg", nil, &pkgOwner.ID)
	if err != nil {
		t.Fatal(err)
	}
	element, err := ws.CreateElement(ctx, core.LibraryElement{Name: "a.go", FilePath: stringPtr("pkg/a.go")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, view.ID, element.ID, 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: "map|pkg|a", Kind: cstore.MappingElement, ResourceID: element.ID, RepositoryID: "repo", SnapshotID: "head"}}); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, paths []string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		for _, p := range paths {
			text := "func Stable() {}"
			src := &graph.Source{Path: p, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[p] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: p, Hash: src.Hash, Size: uint64(len(src.Text))})
			g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", []string{"pkg/a.go"})
	publish("head", []string{"pkg/a.go", "pkg/new.go", "pkg/other.go"})
	diagram, err := Save(ctx, ws, idx, "repo", "live", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if diagram.ViewId != view.ID {
		t.Fatalf("added files not placed in closest view: got %d want %d", diagram.ViewId, view.ID)
	}
	formed := map[string]*pb.ImpactNode{}
	for _, node := range diagram.Nodes {
		formed[node.Path] = node
	}
	if _, ok := formed["pkg/new.go"]; !ok {
		t.Fatalf("added node missing: %+v", diagram.Nodes)
	}
	for _, path := range []string{"pkg/new.go", "pkg/other.go"} {
		node := formed[path]
		if node == nil {
			t.Fatalf("added node %s missing: %+v", path, diagram.Nodes)
		}
		if node.X == 10 && node.Y == 20 {
			t.Fatalf("added node %s overlapped the existing placement: %+v", path, node)
		}
	}
	if formed["pkg/new.go"].X == formed["pkg/other.go"].X && formed["pkg/new.go"].Y == formed["pkg/other.go"].Y {
		t.Fatal("added nodes overlapped each other")
	}
	if len(diagram.Groups) != 1 || diagram.Groups[0].ViewId != view.ID || diagram.Groups[0].Source != "view" {
		t.Fatalf("added files not grouped under the closest view: %+v", diagram.Groups)
	}
	if diagram.Groups[0].Name != "Pkg" {
		t.Fatalf("top two view levels were not skipped: %+v", diagram.Groups)
	}
	if got := diagram.Groups[0].NodeKeys; len(got) != 2 {
		t.Fatalf("group node keys = %v", got)
	}
}

func TestImpactSynthesizesCommunityHierarchy(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	// No workspace views or element mappings: the repository is unmapped, so the
	// hierarchy must fall back to the dependency-graph grouping pipeline.
	publish := func(id string, code map[string]string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for path, text := range code {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		for _, link := range [][2]string{{"alpha/a.go", "alpha/b.go"}, {"beta/c.go", "beta/d.go"}} {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[link[0]].Id, facts[link[1]].Id, "", facts[link[0]].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{"alpha/a.go", "alpha/b.go", "beta/c.go", "beta/d.go"}
	base := map[string]string{}
	head := map[string]string{}
	for _, path := range paths {
		base[path] = "func Stable() { return 1 }"
		head[path] = "func Stable() { return 2 }"
	}
	publish("base", base)
	publish("head", head)
	diagram, err := Save(ctx, ws, idx, "repo", "live", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if diagram.ViewId != 0 {
		t.Fatalf("unmapped comparison set view id %d", diagram.ViewId)
	}
	if len(diagram.Groups) == 0 {
		t.Fatalf("no community hierarchy synthesized: %+v", diagram)
	}
	grouped := map[string]bool{}
	var walk func(groups []*pb.ImpactGroup)
	walk = func(groups []*pb.ImpactGroup) {
		for _, group := range groups {
			if group.Source != "community" {
				t.Fatalf("group source = %q", group.Source)
			}
			for _, key := range group.NodeKeys {
				grouped[key] = true
			}
			walk(group.Children)
		}
	}
	walk(diagram.Groups)
	if len(grouped) != len(diagram.Nodes) {
		t.Fatalf("grouped %d nodes, want %d: %+v", len(grouped), len(diagram.Nodes), diagram.Groups)
	}
}

func TestImpactPlacesAddedFilesNearConnectedElements(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	view, err := ws.CreateView(ctx, "Pkg", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	contextElement, err := ws.CreateElement(ctx, core.LibraryElement{Name: "ctx.go", FilePath: stringPtr("pkg/ctx.go")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, view.ID, contextElement.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: "map|pkg|ctx", Kind: cstore.MappingElement, ResourceID: contextElement.ID, RepositoryID: "repo", SnapshotID: "head"}}); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, paths []string, linked bool) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		facts := map[string]*pb.CodeFact{}
		for _, p := range paths {
			text := "func Stable() {}"
			src := &graph.Source{Path: p, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[p] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: p, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[p] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		if linked {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts["pkg/new.go"].Id, facts["pkg/ctx.go"].Id, "", facts["pkg/new.go"].Anchor, nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", []string{"pkg/ctx.go"}, false)
	publish("head", []string{"pkg/ctx.go", "pkg/new.go"}, true)
	diagram, err := Save(ctx, ws, idx, "repo", "live", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	if diagram.ViewId != view.ID {
		t.Fatalf("connected added file not placed in closest view: got %d want %d", diagram.ViewId, view.ID)
	}
	var added, existing *pb.ImpactNode
	for _, node := range diagram.Nodes {
		switch node.Path {
		case "pkg/new.go":
			added = node
		case "pkg/ctx.go":
			existing = node
		}
	}
	if existing == nil || !existing.Context || existing.ElementId != contextElement.ID {
		t.Fatalf("connected element missing from context: %+v", diagram.Nodes)
	}
	if added == nil {
		t.Fatalf("added node missing: %+v", diagram.Nodes)
	}
	if math.Abs(added.X) > layout.PlacementGapX || math.Abs(added.Y) > layout.PlacementGapY {
		t.Fatalf("added node not placed adjacent to its neighbor: %+v", added)
	}
	if added.X == existing.X && added.Y == existing.Y {
		t.Fatalf("added node overlapped its neighbor: %+v", added)
	}
}

func TestImpactFallsBackToAncestorFolderView(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	// The map owns a folder view for internal/codeindex but no file is placed
	// directly in internal/ or internal/codeindex.
	folderView, err := ws.CreateView(ctx, "codeindex", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clusterView, err := ws.CreateView(ctx, "Compose", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rootFile, err := ws.CreateElement(ctx, core.LibraryElement{Name: "docker-compose.yml", FilePath: stringPtr("docker-compose.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, clusterView.ID, rootFile.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|folderview|repo|internal/codeindex", Kind: cstore.MappingView, ResourceID: folderView.ID, RepositoryID: "repo", SnapshotID: "head"},
		{LogicalKey: "map|fact|repo|docker-compose", Kind: cstore.MappingElement, ResourceID: rootFile.ID, RepositoryID: "repo", SnapshotID: "head"},
	}); err != nil {
		t.Fatal(err)
	}
	publish := func(id string, paths []string) {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: id, Provenance: "commit", IngestionStatus: "complete"}
		g := graph.NewGraph("repo", id)
		for _, p := range paths {
			text := "func Stable() {}"
			src := &graph.Source{Path: p, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[p] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: p, Hash: src.Hash, Size: uint64(len(src.Text))})
			g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Stable", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", []string{"docker-compose.yml"})
	publish("head", []string{"docker-compose.yml", "internal/codeindex/impact/placement.go"})
	diagram, err := Save(ctx, ws, idx, "repo", "live", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if diagram.ViewId != folderView.ID {
		t.Fatalf("added file fell to %d, want ancestor folder view %d", diagram.ViewId, folderView.ID)
	}
}

func TestRetireLegacyMaterializationPreservesSharedResources(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	snap := &pb.Snapshot{Id: "same", RepositoryId: "repo"}
	if err := idx.Publish(ctx, "/repo", snap, graph.NewGraph("repo", "same")); err != nil {
		t.Fatal(err)
	}
	owned, err := ws.CreateElement(ctx, core.LibraryElement{Name: "Old impact file"})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ws.CreateElement(ctx, core.LibraryElement{Name: "Shared full map element"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := ws.CreateView(ctx, "repo impact · Live changes", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, view.ID, shared.ID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "impact|old|view", Kind: cstore.MappingView, ResourceID: view.ID, RepositoryID: "repo"},
		{LogicalKey: "impact|old|file", Kind: cstore.MappingElement, ResourceID: owned.ID, RepositoryID: "repo"},
		{LogicalKey: "map|shared", Kind: cstore.MappingElement, ResourceID: shared.ID, RepositoryID: "repo"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(ctx, ws, idx, "repo", "live", "same", "same", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.ElementByID(ctx, shared.ID); err != nil {
		t.Fatal("shared element deleted", err)
	}
	if _, err := ws.ElementByID(ctx, owned.ID); err == nil {
		t.Fatal("owned impact element retained")
	}
	if _, err := ws.ViewByID(ctx, view.ID); err == nil {
		t.Fatal("owned impact view retained")
	}
	mappings, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil || len(mappings) != 1 || mappings[0].LogicalKey != "map|shared" {
		t.Fatalf("legacy mappings: %+v %v", mappings, err)
	}
}
