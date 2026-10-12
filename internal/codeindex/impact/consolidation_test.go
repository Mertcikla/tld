package impact

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"github.com/mertcikla/tld/v2/internal/core"
)

func TestConsolidateRawDestinationUsesSiblingSourceIdentity(t *testing.T) {
	parent := int64(2)
	scene := &model.Assembly{
		RepositoryId: "repo",
		Views: map[string]*model.ViewContent{
			"7": {
				Placements: []*model.Placement{{Element: &diagv1.PlacedElement{ViewId: 7, ElementId: 63, FilePath: stringPointerForConsolidation("middleware/wrap_writer.go"), RepositoryId: stringPointerForConsolidation("repo")}}},
				Connectors: []*diagv1.Connector{{Id: 8, ViewId: 7, SourceElementId: 63, TargetElementId: -1}},
			},
		},
		Provenance: map[string]*model.Metadata{
			PlacementKey(7, 63): {Key: PlacementKey(7, 63), Origin: pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED, Role: pb.ImpactRole_IMPACT_ROLE_DIRECT},
			ConnectorKey(7, 8):  {Key: ConnectorKey(7, 8), ObjectKind: "connector", EndpointKeys: []string{PlacementKey(7, 63), PlacementKey(7, -1)}},
			ViewKey(7):          {Key: ViewKey(7), ObjectKind: "view"},
		},
		Files: []*pb.ImpactFile{{Path: "middleware/wrap_writer.go"}, {Path: "middleware/wrap_writer_test.go"}},
	}
	a := &sceneAssembly{
		helper: &sceneEvidence{diagram: &model.Diagram{Nodes: []*model.Node{{Path: "middleware/wrap_writer_test.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED}}}, sources: map[string]*pb.SourceChange{}},
		scene:  scene,
		views: map[int64]core.ViewTreeNode{
			4: {ID: 4, Name: "middleware", ParentViewID: &parent},
			7: {ID: 7, Name: "middleware", ParentViewID: &parent},
		},
		mapViews:   map[int64]bool{4: true, 7: true},
		viewIdx:    &viewIndex{dirs: map[string]int64{"middleware": 4}},
		edgeScores: map[string]map[string]int{},
		full:       map[string][]string{"middleware/wrap_writer.go": {PlacementKey(7, 63)}},
		all:        map[string][]string{},
	}
	a.fillRaw(false)
	content := scene.Views["7"]
	if len(content.Placements) != 2 || content.Placements[1].Element.GetViewId() != 7 || content.Placements[1].Element.GetElementId() != -1 {
		t.Fatalf("raw fallback was not added to the compatible selected container: %+v", content.Placements)
	}
	if scene.Provenance[PlacementKey(7, 63)] == nil {
		t.Fatal("consolidation changed the existing placement key")
	}
	if err := a.validate(); err != nil {
		t.Fatalf("assembled consolidated scene invalid: %v", err)
	}
	if err := ValidateCoverage(scene); err != nil {
		t.Fatalf("consolidated scene coverage invalid: %v", err)
	}
	connector := content.Connectors[0]
	if connector.SourceElementId != 63 || connector.TargetElementId != -1 || scene.Provenance[ConnectorKey(7, 8)].EndpointKeys[1] != PlacementKey(7, -1) {
		t.Fatalf("consolidation changed connector endpoints: connector=%+v provenance=%+v", connector, scene.Provenance[ConnectorKey(7, 8)])
	}
}

func TestConsolidateRawDestinationRetainsPartitions(t *testing.T) {
	tests := []struct {
		name      string
		views     []core.ViewTreeNode
		large     bool
		authored  bool
		otherRepo bool
		wantID    int64
	}{
		{name: "different source directory despite same name", views: []core.ViewTreeNode{{ID: 7, Name: "middleware", ParentViewID: ptrInt64ForConsolidation(2)}}, wantID: 0},
		{name: "large selected sibling", views: []core.ViewTreeNode{{ID: 7, Name: "other label", ParentViewID: ptrInt64ForConsolidation(2)}}, large: true, wantID: 0},
		{name: "ambiguous selected siblings", views: []core.ViewTreeNode{{ID: 7, Name: "one", ParentViewID: ptrInt64ForConsolidation(2)}, {ID: 8, Name: "two", ParentViewID: ptrInt64ForConsolidation(2)}}, wantID: 0},
		{name: "authored sibling", views: []core.ViewTreeNode{{ID: 7, Name: "middleware", ParentViewID: ptrInt64ForConsolidation(2)}}, authored: true, wantID: 0},
		{name: "different repository source", views: []core.ViewTreeNode{{ID: 7, Name: "middleware", ParentViewID: ptrInt64ForConsolidation(2)}}, otherRepo: true, wantID: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scene := &model.Assembly{RepositoryId: "repo", Views: map[string]*model.ViewContent{}, Provenance: map[string]*model.Metadata{}}
			a := &sceneAssembly{scene: scene, views: map[int64]core.ViewTreeNode{4: {ID: 4, ParentViewID: ptrInt64ForConsolidation(2)}}, mapViews: map[int64]bool{4: true}}
			for _, view := range tt.views {
				a.views[view.ID] = view
				if !tt.authored {
					a.mapViews[view.ID] = true
				}
				path := "other/area.go"
				if tt.name != "different source directory despite same name" {
					path = "middleware/wrap_writer.go"
				}
				placements := make([]*model.Placement, 1)
				repositoryID := "repo"
				if tt.otherRepo {
					repositoryID = "other"
				}
				placements[0] = &model.Placement{Element: &diagv1.PlacedElement{ViewId: int32(view.ID), ElementId: int32(view.ID), FilePath: stringPointerForConsolidation(path), RepositoryId: stringPointerForConsolidation(repositoryID)}}
				if tt.large {
					for i := 0; i < 100; i++ {
						placements = append(placements, &model.Placement{Element: &diagv1.PlacedElement{ViewId: int32(view.ID), ElementId: int32(100 + i)}})
					}
				}
				scene.Views[itoa(view.ID)] = &model.ViewContent{Placements: placements}
				scene.Provenance[PlacementKey(view.ID, int64(view.ID))] = &model.Metadata{Key: PlacementKey(view.ID, int64(view.ID)), Origin: pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED, Role: pb.ImpactRole_IMPACT_ROLE_DIRECT}
			}
			if got := a.consolidateRawDestination(&model.Node{Path: "middleware/wrap_writer_test.go"}, 4); got != tt.wantID {
				t.Fatalf("destination = %d, want %d", got, tt.wantID)
			}
		})
	}
}

func stringPointerForConsolidation(value string) *string { return &value }
func ptrInt64ForConsolidation(value int64) *int64        { return &value }
