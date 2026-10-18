package impact

import (
	"encoding/json"
	"fmt"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

func autoSceneFixture(count int) *model.Assembly {
	scene := &model.Assembly{SchemaVersion: PipelineVersion, RequestedRadius: 2, EffectiveRadius: 2,
		Views: map[string]*model.ViewContent{"1": {}}, Provenance: map[string]*model.Metadata{},
		Tree: []*diagv1.View{{Id: 1, Name: "Changes"}}}
	for i := 1; i <= count; i++ {
		id := int32(i)
		key := PlacementKey(1, int64(id))
		name := fmt.Sprintf("file-%02d.go", i)
		scene.Views["1"].Placements = append(scene.Views["1"].Placements, &model.Placement{Element: &diagv1.PlacedElement{ViewId: 1, ElementId: id, Name: name}})
		scene.Provenance[key] = &model.Metadata{Key: key, ViewId: 1, ResourceId: int64(id), Role: pb.ImpactRole_IMPACT_ROLE_DIRECT}
		scene.Files = append(scene.Files, &pb.ImpactFile{Path: name, Representatives: []*pb.ImpactPlacementRef{{ViewId: 1, ElementId: id}}})
	}
	return scene
}

func autoTestEdge(scene *model.Assembly, from, to int32, tags ...string) {
	id := int32(len(scene.Views["1"].Connectors) + 1)
	scene.Views["1"].Connectors = append(scene.Views["1"].Connectors, &diagv1.Connector{Id: id, ViewId: 1, SourceElementId: from, TargetElementId: to, Tags: tags, Direction: "backward"})
	key := ConnectorKey(1, int64(id))
	scene.Provenance[key] = &model.Metadata{Key: key, EndpointKeys: []string{PlacementKey(1, int64(from)), PlacementKey(1, int64(to))}}
}

func TestAutoContextPreservesChangesAndDependencyPaths(t *testing.T) {
	scene := autoSceneFixture(7)
	scene.Files = scene.Files[:1]
	for i := 2; i <= 7; i++ {
		p := scene.Provenance[PlacementKey(1, int64(i))]
		p.Role, p.Distance = pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT, 1
	}
	scene.Provenance[PlacementKey(1, 4)].Distance = 2
	scene.Provenance[PlacementKey(1, 6)].Role = pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT
	// File representatives must survive even if a future role classification
	// treats one as context.
	scene.Files = append(scene.Files, &pb.ImpactFile{Path: "protected.go", Representatives: []*pb.ImpactPlacementRef{{ViewId: 1, ElementId: 7}}})
	autoTestEdge(scene, 1, 2)
	autoTestEdge(scene, 1, 3)
	autoTestEdge(scene, 2, 3) // Optional same-hop context.
	autoTestEdge(scene, 2, 3, "change:removed")
	autoTestEdge(scene, 3, 4) // Required path to second-hop context.
	before, _ := json.Marshal(scene)
	out := AutoContext(scene)
	if err := ValidateCoverage(out); err != nil {
		t.Fatal(err)
	}
	if SceneNodeCount(out) != 5 || len(out.Views["1"].Connectors) != 4 {
		t.Fatalf("unexpected reduction: nodes=%d edges=%d", SceneNodeCount(out), len(out.Views["1"].Connectors))
	}
	if out.Provenance[ConnectorKey(1, 3)] != nil || out.Provenance[ConnectorKey(1, 4)] == nil || out.Provenance[ConnectorKey(1, 5)] == nil {
		t.Fatal("lost changed relationship or dependency witness path")
	}
	for _, edge := range out.Views["1"].Connectors {
		if edge.Direction != "backward" {
			t.Fatal("rewrote edge direction")
		}
	}
	after, _ := json.Marshal(scene)
	if string(before) != string(after) {
		t.Fatal("mutated immutable assembly")
	}
}

func TestSplitAutoScenePreservesEveryFileAndEdge(t *testing.T) {
	scene := autoSceneFixture(16)
	for i := int32(1); i < 16; i++ {
		if i != 8 {
			autoTestEdge(scene, i, i+1, "change:added")
		}
	}
	before, _ := json.Marshal(scene)
	var previous string
	for range 10 {
		panels := SplitAutoScene(scene)
		if len(panels) != 2 {
			t.Fatalf("panels=%d", len(panels))
		}
		files, edges := map[string]bool{}, 0
		for _, panel := range panels {
			if err := ValidateCoverage(panel); err != nil {
				t.Fatal(err)
			}
			for _, file := range panel.Files {
				if files[file.Path] {
					t.Fatalf("duplicated file %s", file.Path)
				}
				files[file.Path] = true
			}
			edges += len(panel.Views["1"].Connectors)
		}
		if len(files) != 16 || edges != 14 {
			t.Fatalf("lost coverage: files=%d edges=%d", len(files), edges)
		}
		raw, _ := json.Marshal(panels)
		if previous != "" && string(raw) != previous {
			t.Fatal("nondeterministic panel assignment")
		}
		previous = string(raw)
	}
	after, _ := json.Marshal(scene)
	if string(before) != string(after) {
		t.Fatal("split mutated source")
	}
	// A relationship crossing the two groups makes the diagram indivisible.
	autoTestEdge(scene, 8, 9)
	if len(SplitAutoScene(scene)) != 1 {
		t.Fatal("split cut a relationship")
	}
}

func TestSplitAutoSceneKeepsFileRepresentativesTogether(t *testing.T) {
	scene := autoSceneFixture(16)
	for i := int32(1); i < 16; i++ {
		if i != 8 {
			autoTestEdge(scene, i, i+1)
		}
	}
	scene.Files[0].Representatives = append(scene.Files[0].Representatives, &pb.ImpactPlacementRef{ViewId: 1, ElementId: 9})
	if len(SplitAutoScene(scene)) != 1 {
		t.Fatal("split separated two representatives of the same changed file")
	}
}

func TestAutoSelectionsRetainEmptyAncestorViews(t *testing.T) {
	scene := autoSceneFixture(16)
	for i := int32(1); i < 16; i++ {
		if i != 8 {
			autoTestEdge(scene, i, i+1)
		}
	}
	scene.Tree = []*diagv1.View{{Id: 2, Name: "Workspace", Children: scene.Tree}}
	scene.Views["2"] = &model.ViewContent{}
	trimmed := AutoContext(scene)
	separated, _ := SeparateAutoFiles(scene)
	panels := SplitAutoScene(scene)
	if len(panels) != 2 {
		t.Fatal("fixture did not split")
	}
	for _, selected := range append([]*model.Assembly{trimmed, separated}, panels...) {
		ancestor := selected.Views["2"]
		if ancestor == nil || len(ancestor.Placements) != 0 || selected.Tree[0].Name != "Workspace" {
			t.Fatal("lost empty ancestor wrapper")
		}
		if err := ValidateCoverage(selected); err != nil {
			t.Fatal(err)
		}
	}
	if SelectScene(scene, 0, false).Views["2"] != nil {
		t.Fatal("Mermaid hierarchy policy leaked into canonical selection")
	}
}

func TestSeparateAutoFilesListsOnlyIsolatedRawChanges(t *testing.T) {
	scene := autoSceneFixture(5)
	for _, p := range scene.Provenance {
		p.Origin = pb.ImpactOrigin_IMPACT_ORIGIN_RAW
	}
	scene.Provenance[PlacementKey(1, 5)].Origin = pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED
	autoTestEdge(scene, 1, 2)
	autoTestEdge(scene, 4, 4, "change:added") // Even a singleton changed edge is retained.
	out, files := SeparateAutoFiles(scene)
	if len(files) != 1 || files[0].Path != "file-03.go" || SceneNodeCount(out) != 4 {
		t.Fatalf("listed=%v placements=%d", files, SceneNodeCount(out))
	}
	if err := ValidateCoverage(out); err != nil {
		t.Fatal(err)
	}
	if len(scene.Files) != 5 || len(out.Files)+len(files) != 5 || len(out.Views["1"].Connectors) != 2 {
		t.Fatal("lost evidence or mutated input")
	}
	// An unchanged singleton with a self-relationship is not isolated context.
	scene.Provenance[PlacementKey(1, 4)].Role = pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT
	scene.Files = append(scene.Files[:3], scene.Files[4:]...)
	if AutoContext(scene).Provenance[PlacementKey(1, 4)] == nil {
		t.Fatal("pruned endpoint of changed self-relationship")
	}
}

func TestAutoContextTrimsLeavesButKeepsChangedEdgesAndBridges(t *testing.T) {
	scene := autoSceneFixture(12)
	scene.Files = scene.Files[:1]
	for i := 2; i <= 12; i++ {
		p := scene.Provenance[PlacementKey(1, int64(i))]
		p.Role, p.Distance, p.Name = pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT, 1, fmt.Sprintf("file-%02d", i)
		if i < 12 {
			autoTestEdge(scene, 1, int32(i))
		}
	}
	autoTestEdge(scene, 11, 1, "change:added") // Protected despite its late name.
	autoTestEdge(scene, 10, 12)                // A path to second-hop context, not an optional leaf.
	scene.Provenance[PlacementKey(1, 12)].Distance = 2
	out := AutoContext(scene)
	if SceneNodeCount(out) != 7 {
		t.Fatalf("expected changed file + six neighbours, got %d", SceneNodeCount(out))
	}
	if out.Provenance[PlacementKey(1, 11)] == nil || out.Provenance[PlacementKey(1, 10)] == nil {
		t.Fatal("lost changed edge endpoint or bridge")
	}
	if err := ValidateCoverage(out); err != nil {
		t.Fatal(err)
	}
	// Retain changed relationships even when they make the soft target impossible.
	for _, edge := range scene.Views["1"].Connectors {
		edge.Tags = []string{"change:modified"}
	}
	if SceneNodeCount(AutoContext(scene)) != 12 {
		t.Fatal("trimmed a changed relationship to meet a soft neighbour budget")
	}
}

func TestSplitAutoSceneKeepsCrossViewConnectorsAndNavigation(t *testing.T) {
	for _, navigation := range []bool{false, true} {
		scene := autoSceneFixture(16)
		scene.Views["2"] = &model.ViewContent{Placements: scene.Views["1"].Placements[8:]}
		scene.Views["1"].Placements = scene.Views["1"].Placements[:8]
		for _, p := range scene.Views["2"].Placements {
			id := p.Element.ElementId
			p.Element.ViewId = 2
			old := scene.Provenance[PlacementKey(1, int64(id))]
			delete(scene.Provenance, old.Key)
			old.ViewId, old.Key = 2, PlacementKey(2, int64(id))
			scene.Provenance[old.Key] = old
			scene.Files[id-1].Representatives[0].ViewId = 2
		}
		for i := int32(1); i < 8; i++ {
			autoTestEdge(scene, i, i+1)
		}
		for i := int32(9); i < 16; i++ {
			scene.Views["2"].Connectors = append(scene.Views["2"].Connectors, &diagv1.Connector{Id: i, ViewId: 2, SourceElementId: i, TargetElementId: i + 1})
		}
		if navigation {
			owner := int32(8)
			scene.Navigations = []*diagv1.ElementNavigationInfo{{FromViewId: 1, ToViewId: 2, ElementId: &owner, RelationType: "child"}}
		} else {
			autoTestEdge(scene, 8, 9)
			scene.Provenance[ConnectorKey(1, 8)].EndpointKeys[1] = PlacementKey(2, 9)
		}
		if len(SplitAutoScene(scene)) != 1 {
			t.Fatalf("split cut cross-view relation (navigation=%t)", navigation)
		}
	}
}
