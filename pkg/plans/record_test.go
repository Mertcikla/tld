package plans

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestRecordedUsesLiveStateWithoutReapplyingHistory(t *testing.T) {
	before := fixture()
	after := NewState(before).Graph
	after["element:1"].GetElement().Name = "Planned name"
	ops := Diff(before, after)
	after["element:1"].GetElement().Name = "Later live name"
	s := Recorded(after, ops)
	if s.Graph["element:1"].GetElement().Name != "Later live name" || s.Changes["element:1"] != "modify" {
		t.Fatal("history altered live state")
	}
	if s.Before["element:1"].GetElement().Name != "API" {
		t.Fatal("before value missing")
	}
	legacy := proto.Clone(ops[0]).(*pb.PlanOperation)
	legacy.SchemaVersion = 1
	s = Recorded(after, []*pb.PlanOperation{legacy})
	if len(s.Changes) != 0 || s.Graph["element:1"].GetElement().Name != "Later live name" {
		t.Fatal("legacy proposal applied")
	}
}
func TestRecordedCreateThenDeleteHasNoNetScope(t *testing.T) {
	before := fixture()
	after := NewState(before).Graph
	after["element:4"] = &pb.PlanResource{Key: "element:4", Value: &pb.PlanResource_Element{Element: &pb.Element{Id: 4, Name: "Temporary"}}}
	ops := append(Diff(before, after), Diff(after, before)...)
	s := Recorded(before, ops)
	if len(s.Changes) != 0 || s.Members["element:4"] != nil {
		t.Fatal("temporary element retained in scope")
	}
}

func TestRecordedExcludeRemovesChangedResourceFromScope(t *testing.T) {
	before := fixture()
	after := NewState(before).Graph
	after["element:1"].GetElement().Name = "Updated API"
	ops := append(Diff(before, after), &pb.PlanOperation{
		Action: &pb.PlanOperation_Exclude{Exclude: &pb.PlanResourceKey{Key: "element:1"}},
	})
	state := Recorded(after, ops)
	if state.Members["element:1"] != nil || state.Changes["element:1"] != "" {
		t.Fatal("excluded change retained in plan scope")
	}
	if state.Graph["element:1"].GetElement().Name != "Updated API" {
		t.Fatal("excluding plan scope changed the workspace")
	}
	for _, resource := range Project(after, state, 0, false) {
		if resource.Resource.Key == "element:1" {
			t.Fatal("excluded change still projected as plan scope")
		}
	}
	// A later workspace edit can bring the resource back into scope.
	later := NewState(after).Graph
	later["element:1"].GetElement().Name = "Changed again"
	state = Recorded(later, append(ops, Diff(after, later)...))
	if state.Members["element:1"] == nil || state.Changes["element:1"] != "modify" {
		t.Fatal("later edit did not restore plan membership")
	}
}
