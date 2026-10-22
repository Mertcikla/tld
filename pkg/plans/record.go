package plans

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"sort"
)

// Diff records completed writes, including cascades and implicit placements.
func Diff(before, after Graph) []*pb.PlanOperation {
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	ordered := []string{}
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	out := []*pb.PlanOperation{}
	for _, k := range ordered {
		a, b := before[k], after[k]
		if Hash(a) == Hash(b) {
			continue
		}
		op := &pb.PlanOperation{SchemaVersion: 2}
		switch {
		case a == nil:
			op.Action = &pb.PlanOperation_Create{Create: proto.Clone(b).(*pb.PlanResource)}
		case b == nil:
			op.Action = &pb.PlanOperation_Remove{Remove: &pb.PlanResourceKey{Key: k}}
		default:
			fields := []string{}
			d := Value(b).ProtoReflect().Descriptor().Fields()
			for i := 0; i < d.Len(); i++ {
				fields = append(fields, string(d.Get(i).Name()))
			}
			op.Action = &pb.PlanOperation_Update{Update: &pb.PlanPatch{Resource: proto.Clone(b).(*pb.PlanResource), Fields: &fieldmaskpb.FieldMask{Paths: fields}}}
		}
		pre := &pb.PlanPrecondition{ResourceKey: k, Exists: a != nil}
		if a != nil {
			pre.Before = proto.Clone(a).(*pb.PlanResource)
		}
		op.Preconditions = []*pb.PlanPrecondition{pre}
		out = append(out, op)
	}
	return out
}

// Recorded reads plan membership and history. It never applies workspace edits.
func Recorded(live Graph, ops []*pb.PlanOperation) *State {
	s := NewState(live)
	s.Changes = map[string]string{}
	seen := map[string]bool{}
	for _, op := range ops {
		if op.SchemaVersion < 2 && (op.GetCreate() != nil || op.GetUpdate() != nil || op.GetRemove() != nil) {
			s.Results = append(s.Results, &pb.PlanOperationResult{OperationId: op.Id, Status: "historical", Reason: "Legacy staged operation; not applied to the workspace"})
			continue
		}
		k := Target(op)
		for _, pre := range op.Preconditions {
			if pre.ResourceKey == k && !seen[k] {
				seen[k] = true
				s.Before[k] = pre.Before
				if pre.Before != nil {
					s.Historical[k] = pre.Before
				}
			}
		}
		switch a := op.Action.(type) {
		case *pb.PlanOperation_Include:
			s.Members[k] = proto.Clone(a.Include).(*pb.PlanMember)
		case *pb.PlanOperation_Exclude:
			delete(s.Members, k)
			delete(s.Changes, k)
		case *pb.PlanOperation_Checklist:
			s.Checklist[a.Checklist.Id] = proto.Clone(a.Checklist).(*pb.PlanChecklistItem)
		case *pb.PlanOperation_RemoveChecklist:
			delete(s.Checklist, a.RemoveChecklist.Key)
		case *pb.PlanOperation_Note:
			s.Notes[k] = proto.Clone(a.Note).(*pb.PlanNote)
			if s.Members[k] == nil {
				s.Members[k] = &pb.PlanMember{ResourceKey: k, Role: "change"}
			}
		default:
			if s.Members[k] == nil {
				s.Members[k] = &pb.PlanMember{ResourceKey: k, Role: "change"}
			}
			switch op.Action.(type) {
			case *pb.PlanOperation_Create:
				s.Changes[k] = "create"
			case *pb.PlanOperation_Update:
				if s.Changes[k] != "create" {
					s.Changes[k] = "modify"
				}
			case *pb.PlanOperation_Remove:
				if s.Changes[k] == "create" {
					delete(s.Changes, k)
					delete(s.Members, k)
				} else {
					s.Changes[k] = "remove"
				}
			}
		}
		s.Results = append(s.Results, &pb.PlanOperationResult{OperationId: op.Id, Status: "recorded"})
	}
	return s
}
