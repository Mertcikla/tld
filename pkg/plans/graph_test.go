package plans

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
)

func fixture() Graph {
	return FromExport(&pb.ExportOrganizationResponse{Elements: []*pb.Element{{Id: 1, Name: "API"}, {Id: 2, Name: "DB"}, {Id: 3, Name: "Worker"}}, Views: []*pb.View{{Id: 1, Name: "Root"}}, Connectors: []*pb.Connector{{Id: 1, ViewId: 1, SourceElementId: 1, TargetElementId: 2, Direction: "forward", Style: "bezier"}, {Id: 2, ViewId: 1, SourceElementId: 2, TargetElementId: 3, Direction: "forward", Style: "bezier"}}})
}
func TestRadiusPreservesScope(t *testing.T) {
	g := fixture()
	s := NewState(g)
	s.Members["element:1"] = &pb.PlanMember{ResourceKey: "element:1", Role: "preserve", Explicit: true}
	for radius, want := range []int{1, 2, 3} {
		count := 0
		for _, v := range Project(g, s, uint32(radius), false) {
			if v.Resource.GetElement() != nil {
				count++
			}
			if v.Resource.Key == "element:2" && v.Role != "" {
				t.Fatal("context became a member")
			}
		}
		if count != want {
			t.Fatalf("radius %d got %d elements want %d", radius, count, want)
		}
	}
	if len(s.Members) != 1 {
		t.Fatal("projection mutated membership")
	}
}
func TestHashIgnoresPlanTagsAndTagOrdering(t *testing.T) {
	a := fixture()["element:1"]
	a.GetElement().Tags = []string{"b", "a"}
	b := proto.Clone(a).(*pb.PlanResource)
	b.GetElement().Tags = []string{"plan:modify", "a", "b"}
	if Hash(a) != Hash(b) {
		t.Fatal("derived tag caused conflict")
	}
	b.GetElement().Description = proto.String("different")
	if Hash(a) == Hash(b) {
		t.Fatal("missed metadata difference")
	}
}
func TestNoteDoesNotChangeViewOrExistingDocument(t *testing.T) {
	g := fixture()
	g["view:1"].GetView().Markdown = &pb.ViewMarkdownDocument{Path: "original.md"}
	before := Hash(g["view:1"])
	s := NewState(g)
	err := s.ApplyMetadata(&pb.PlanOperation{Action: &pb.PlanOperation_Note{Note: &pb.PlanNote{ViewKey: "view:1", Content: "Draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	if Hash(g["view:1"]) != before || Hash(s.Graph["view:1"]) != before {
		t.Fatal("note altered live attachment")
	}
	if s.Notes["view:1"].Content != "Draft" {
		t.Fatal("note lost")
	}
}
func TestPromptUsesReadableNames(t *testing.T) {
	g := fixture()
	s := NewState(g)
	for _, key := range []string{"element:1", "connector:1", "view:1"} {
		s.Members[key] = &pb.PlanMember{ResourceKey: key, Role: "change", Explicit: true}
	}
	s.Checklist["claim-uuid"] = &pb.PlanChecklistItem{Text: "Keep retries bounded", VerificationKind: "manual"}
	s.Notes["view:1"] = &pb.PlanNote{ViewKey: "view:1", Content: "Details"}
	text := Prompt(&pb.Plan{Id: "plan-uuid", Name: "Bob's feature", Revision: 3}, g, s)
	for _, unexpected := range []string{"element:1", "connector:1", "claim-uuid"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("prompt leaked %q: %s", unexpected, text)
		}
	}
	for _, want := range []string{"API → DB", "tld plan show 'plan-uuid' --revision 3", "tld plan note 'plan-uuid' 'view:1' --revision 3"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	placement := &pb.PlanResource{Value: &pb.PlanResource_Placement{Placement: &pb.ElementPlacement{ElementId: 1, ViewId: 1}}}
	if Describe(g, placement) != "API in Root" {
		t.Fatal(Describe(g, placement))
	}
}

func TestPromptFoldsCreatedPlacementsAndRedundantRoles(t *testing.T) {
	base := fixture()
	s := NewState(base)
	s.Graph["element:new"] = &pb.PlanResource{Key: "element:new", Value: &pb.PlanResource_Element{Element: &pb.Element{Id: 4, Name: "Cache"}}}
	s.Graph["elementplacement:new"] = &pb.PlanResource{Key: "elementplacement:new", Value: &pb.PlanResource_Placement{Placement: &pb.ElementPlacement{Id: 4, ElementId: 4, ViewId: 1}}}
	for _, key := range []string{"element:new", "elementplacement:new"} {
		s.Members[key] = &pb.PlanMember{ResourceKey: key, Role: "change"}
	}
	s.Members["view:1"] = &pb.PlanMember{ResourceKey: "view:1", Role: "preserve", Explicit: true}
	text := Prompt(&pb.Plan{Name: "Feature"}, base, s)
	if strings.Count(text, "- create:") != 1 || !strings.Contains(text, "- create: Cache in Root\n") || !strings.Contains(text, "- preserve: Root\n") || strings.Contains(text, "(change)") || strings.Contains(text, "(preserve)") {
		t.Fatal(text)
	}
	// A differing role must stay visible rather than being folded away.
	s.Members["elementplacement:new"].Role = "context"
	text = Prompt(&pb.Plan{Name: "Feature"}, base, s)
	if !strings.Contains(text, "- create: Cache in Root (context)\n") || strings.Count(text, "- create:") != 2 {
		t.Fatal(text)
	}
}
