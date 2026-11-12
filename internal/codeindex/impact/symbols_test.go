package impact

import (
	"strconv"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"google.golang.org/protobuf/proto"
)

func TestSceneSymbolCallsKeepLineEndpoints(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "old", "b.go": "target"}, nil)
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "new", "b.go": "target"}, [][2]string{{"a.go", "b.go"}})
	diagram, err := compareSceneSnapshots(ctx, ws, idx, "repo", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, content := range diagram.Scene.Views {
		for _, connector := range content.Connectors {
			if connector.GetLabel() != "calls" {
				continue
			}
			found = true
			if !strings.HasSuffix(connector.GetSourceHandle(), "-line-1") || !strings.HasSuffix(connector.GetTargetHandle(), "-line-1") {
				t.Fatalf("lost line endpoints: %v", connector)
			}
			if !hasTag(connector.Tags, "change:added") {
				t.Fatalf("lost added state: %v", connector.Tags)
			}
			targetKey := diagram.Scene.Provenance[ConnectorKey(int64(connector.ViewId), int64(connector.Id))].EndpointKeys[1]
			target := diagram.Scene.Provenance[targetKey]
			captured := false
			for _, placement := range diagram.Scene.Views[strconv.FormatInt(target.ViewId, 10)].Placements {
				if int64(placement.Element.ElementId) == target.ResourceId {
					for _, symbol := range placement.GetOverlay().GetSymbols() {
						if symbol.Change == pb.ChangeKind_CHANGE_KIND_UNSPECIFIED {
							captured = true
						}
					}
				}
			}
			if !captured {
				t.Fatal("unchanged target symbol was not captured")
			}
		}
	}
	if !found {
		t.Fatal("missing symbol call connector")
	}
}

func TestAdjustSceneHandlesPreservesSymbolEndpoints(t *testing.T) {
	placements := []*model.Placement{
		{Element: &diagv1.PlacedElement{ElementId: 1}},
		{Element: &diagv1.PlacedElement{ElementId: 2, PositionX: 500}},
	}
	connector := &diagv1.Connector{SourceElementId: 1, TargetElementId: 2, SourceHandle: proto.String("right-line-12"), TargetHandle: proto.String("left-symbol-Run")}
	adjustSceneConnectorHandles(placements, []*diagv1.Connector{connector})
	if connector.GetSourceHandle() != "right-line-12" || connector.GetTargetHandle() != "left-symbol-Run" {
		t.Fatalf("semantic handles changed: %v", connector)
	}
}

func TestAdjustSceneSymbolHandlesFaceLeftAndKeepIdentity(t *testing.T) {
	placements := []*model.Placement{
		{Element: &diagv1.PlacedElement{ElementId: 1, PositionX: 500}},
		{Element: &diagv1.PlacedElement{ElementId: 2}},
	}
	connector := &diagv1.Connector{SourceElementId: 1, TargetElementId: 2, SourceHandle: proto.String("right-line-12"), TargetHandle: proto.String("left-symbol-Run")}
	adjustSceneConnectorHandles(placements, []*diagv1.Connector{connector})
	if connector.GetSourceHandle() != "left-line-12" || connector.GetTargetHandle() != "right-symbol-Run" {
		t.Fatalf("wrong facing handles: %v", connector)
	}
}

func TestSceneOmitsSameFileSymbolLinks(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	for _, id := range []string{"base", "head"} {
		g := graph.NewGraph("repo", id)
		text := "func A() {}\nfunc B() {}\n// " + id
		source := &graph.Source{Path: "a.go", Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
		other := &graph.Source{Path: "b.go", Language: "go", Text: []byte("func C() {}"), Hash: graph.Hash([]byte("func C() {}"))}
		g.Sources[source.Path], g.Sources[other.Path] = source, other
		a := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", source.Anchor(0, 11), text, "", nil)
		b := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", source.Anchor(12, 23), "func B() {}", "", nil)
		c := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "C", "go", other.Anchor(0, len(other.Text)), "func C() {}", "", nil)
		if id == "head" {
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", a.Anchor)
			g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, c.Id, "", a.Anchor)
		}
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", IngestionStatus: "complete", Manifest: &pb.SnapshotManifest{Complete: true}}
		for _, source := range []*graph.Source{source, other} {
			file := &pb.SourceFile{Path: source.Path, Hash: source.Hash, Mode: "100644"}
			snap.Sources = append(snap.Sources, file)
			snap.Manifest.Files = append(snap.Manifest.Files, file)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	diagram, err := compareSceneSnapshots(ctx, ws, idx, "repo", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, content := range diagram.Scene.Views {
		for _, connector := range content.Connectors {
			if connector.GetLabel() != "calls" {
				continue
			}
			calls++
			metadata := diagram.Scene.Provenance[ConnectorKey(int64(connector.ViewId), int64(connector.Id))]
			if len(metadata.Paths) != 2 || metadata.Paths[0] == metadata.Paths[1] {
				t.Fatalf("same-file symbol link leaked: %v", metadata)
			}
		}
	}
	if calls != 1 {
		t.Fatalf("got %d calls, want only the cross-file call", calls)
	}
}
