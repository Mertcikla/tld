package impact

import (
	"context"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
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
