package grounded

import (
	"reflect"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

func TestFromSceneProjectsRecordedRolesAndCoverage(t *testing.T) {
	scene := &pb.ImpactScene{
		AuthoredViewIds: []int64{1},
		Provenance: map[string]*pb.ImpactProvenance{
			"view|1":         {ObjectKind: "view", ViewId: 1, Name: "Architecture"},
			"placement|1|1":  {ObjectKind: "placement", ViewId: 1, ResourceId: 1, Name: "Subsystem", Origin: pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED, Grounding: pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED, Role: pb.ImpactRole_IMPACT_ROLE_CONTAINED, Paths: []string{"src/a.go"}},
			"placement|1|2":  {ObjectKind: "placement", ViewId: 1, ResourceId: 2, Name: "Neighbour", Origin: pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED, Grounding: pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED, Role: pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT, Paths: []string{"src/b.go"}, Witnesses: []*pb.ImpactWitness{{Paths: []string{"src/a.go", "src/b.go"}}}},
			"placement|1|3":  {ObjectKind: "placement", ViewId: 1, ResourceId: 3, Name: "Concept", Origin: pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED, Grounding: pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED, Role: pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT},
			"placement|1|4":  {ObjectKind: "placement", ViewId: 1, ResourceId: 4, Origin: pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED, Paths: []string{"mapped.go"}},
			"placement|1|-1": {ObjectKind: "placement", ViewId: 1, ResourceId: -1, Origin: pb.ImpactOrigin_IMPACT_ORIGIN_RAW, Paths: []string{"raw.go"}},
			"connector|1|1":  {ObjectKind: "connector", Origin: pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED},
			"connector|1|2":  {ObjectKind: "connector", Origin: pb.ImpactOrigin_IMPACT_ORIGIN_RAW},
		},
		Views: map[string]*pb.SceneViewContent{"1": {Connectors: []*diagv1.Connector{
			{Id: 1, ViewId: 1, SourceElementId: 1, TargetElementId: 2},
			{Id: 2, ViewId: 1, SourceElementId: 1, TargetElementId: -1},
		}}},
		Coverage: []*pb.SourceChange{
			{Path: "src/a.go", PlacementKeys: []string{"placement|1|1"}},
			{Path: "mapped.go", PlacementKeys: []string{"placement|1|4"}},
			{Path: "raw.go", PlacementKeys: []string{"placement|1|-1"}},
		},
	}
	summary := FromScene(scene, "Architecture", true)
	if summary.Affected != 1 || summary.Context != 1 || summary.Ungrounded != 1 || summary.FallbackToRaw {
		t.Fatalf("counts: %+v", summary)
	}
	if !reflect.DeepEqual(summary.Views, []ViewMatch{{ViewID: 1, ViewName: "Architecture", Affected: 1, Total: 3, Indirect: 1}}) {
		t.Fatalf("views: %+v", summary.Views)
	}
	if !reflect.DeepEqual(summary.Uncovered, []string{"raw.go"}) {
		t.Fatalf("coverage: %+v", summary.Uncovered)
	}
	if len(summary.Elements) != 3 || summary.Elements[0].Link != "src/a.go" || !reflect.DeepEqual(summary.Elements[1].Near, []string{"src/a.go"}) {
		t.Fatalf("elements: %+v", summary.Elements)
	}
	if len(summary.Connectors) != 1 || summary.Connectors[0].Target != "2" {
		t.Fatalf("connectors: %+v", summary.Connectors)
	}
}
