package impact

import (
	"context"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestAnalysisRadiusAndSymbolChanges(t *testing.T) {
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
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[link[0]].Id, facts[link[1]].Id, "", facts[link[0]].Anchor)
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
	diagram, err := buildAnalysis(ctx, idx, "repo", "live", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagram.Nodes) != 2 {
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
	assertFactFingerprints(t, diagram)
	expanded, err := buildAnalysis(ctx, idx, "repo", "live", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded.Nodes) != 3 || len(expanded.Edges) != 1 {
		t.Fatalf("expanded impact: %+v", expanded)
	}
	for _, node := range expanded.Nodes {
		if node.Path == "c.go" {
			t.Fatal("file beyond the requested radius was introduced")
		}
		if node == nil {
			t.Fatal("source analysis must not choose workspace elements")
		}
	}
	if _, err := ws.ElementByID(ctx, contextElement.ID); err != nil {
		t.Fatal("shared context was deleted")
	}
	historical, err := buildAnalysis(ctx, idx, "repo", "historical", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Scene != nil || diagram.Scene != nil {
		t.Fatal("comparison materialized a view")
	}
	empty, err := buildAnalysis(ctx, idx, "repo", "live", "head", "head", 0)
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

// assertBodyFingerprints verifies impact diagrams carry symbol metadata and a
// content fingerprint, never inline source.
func assertFactFingerprints(t *testing.T, diagram *model.Diagram) {
	t.Helper()
	for _, facts := range [][]*pb.CodeFact{
		diagram.GetDiff().GetFacts().GetAdded(),
		diagram.GetDiff().GetFacts().GetRemoved(),
		diagram.GetDiff().GetFacts().GetModified(),
	} {
		for _, fact := range facts {
			if fact.GetBodyHash() == "" {
				t.Fatalf("impact diff missing body hash for %q", fact.GetName())
			}
		}
	}
	for _, node := range diagram.GetNodes() {
		for _, facts := range [][]*pb.CodeFact{node.GetSymbols().GetAdded(), node.GetSymbols().GetRemoved(), node.GetSymbols().GetModified()} {
			for _, fact := range facts {
				if fact.GetBodyHash() == "" {
					t.Fatalf("impact node missing body hash for %q", fact.GetName())
				}
				if fact.GetName() == "" || fact.GetAnchor().GetPath() == "" {
					t.Fatalf("symbol metadata dropped: %+v", fact)
				}
			}
		}
	}
}
