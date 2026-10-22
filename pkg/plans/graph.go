// Package plans describes workspace changes recorded in a plan.
package plans

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Graph map[string]*pb.PlanResource

type State struct {
	Changes    map[string]string
	Before     Graph
	Graph      Graph
	Members    map[string]*pb.PlanMember
	Checklist  map[string]*pb.PlanChecklistItem
	Notes      map[string]*pb.PlanNote
	Results    []*pb.PlanOperationResult
	Historical Graph
}

func NewState(g Graph) *State {
	c := Graph{}
	for k, r := range g {
		c[k] = proto.Clone(r).(*pb.PlanResource)
	}
	return &State{Before: Graph{}, Graph: c, Historical: Graph{}, Members: map[string]*pb.PlanMember{}, Checklist: map[string]*pb.PlanChecklistItem{}, Notes: map[string]*pb.PlanNote{}}
}
func Value(r *pb.PlanResource) proto.Message {
	if r == nil {
		return nil
	}
	switch v := r.Value.(type) {
	case *pb.PlanResource_Element:
		return v.Element
	case *pb.PlanResource_View:
		return v.View
	case *pb.PlanResource_Connector:
		return v.Connector
	case *pb.PlanResource_Placement:
		return v.Placement
	case *pb.PlanResource_Layer:
		return v.Layer
	}
	return nil
}
func Kind(r *pb.PlanResource) string {
	if Value(r) == nil {
		return ""
	}
	return strings.ToLower(string(Value(r).ProtoReflect().Descriptor().Name()))
}
func ID(r *pb.PlanResource) int32 {
	m := Value(r)
	if m == nil {
		return 0
	}
	return int32(m.ProtoReflect().Get(m.ProtoReflect().Descriptor().Fields().ByName("id")).Int())
}
func Key(kind string, id int32) string { return fmt.Sprintf("%s:%d", kind, id) }
func (g Graph) Find(kind string, id int32) string {
	for k, r := range g {
		if Kind(r) == kind && ID(r) == id {
			return k
		}
	}
	return Key(kind, id)
}
func FromExport(e *pb.ExportOrganizationResponse) Graph {
	g := Graph{}
	for _, v := range e.Elements {
		k := Key("element", v.Id)
		g[k] = &pb.PlanResource{Key: k, Value: &pb.PlanResource_Element{Element: proto.Clone(v).(*pb.Element)}}
	}
	var views func([]*pb.View)
	views = func(vs []*pb.View) {
		for _, v := range vs {
			c := proto.Clone(v).(*pb.View)
			c.Children = nil
			k := Key("view", v.Id)
			g[k] = &pb.PlanResource{Key: k, Value: &pb.PlanResource_View{View: c}}
			views(v.Children)
		}
	}
	views(e.Views)
	for _, v := range e.Connectors {
		k := Key("connector", v.Id)
		g[k] = &pb.PlanResource{Key: k, Value: &pb.PlanResource_Connector{Connector: proto.Clone(v).(*pb.Connector)}}
	}
	for _, v := range e.Placements {
		k := Key("elementplacement", v.Id)
		g[k] = &pb.PlanResource{Key: k, Value: &pb.PlanResource_Placement{Placement: proto.Clone(v).(*pb.ElementPlacement)}}
	}
	for _, v := range e.Layers {
		k := Key("viewlayer", v.Id)
		g[k] = &pb.PlanResource{Key: k, Value: &pb.PlanResource_Layer{Layer: proto.Clone(v).(*pb.ViewLayer)}}
	}
	return g
}

// Hash excludes transport metadata and plan annotations. Protobuf deterministic
// encoding is versioned by the operation schema version.
func Hash(r *pb.PlanResource) string {
	if r == nil {
		return "absent"
	}
	c := proto.Clone(r).(*pb.PlanResource)
	m := Value(c)
	if m == nil {
		return "invalid"
	}
	p := m.ProtoReflect()
	for _, name := range []protoreflect.Name{"created_at", "updated_at", "org_id", "ref", "children", "depth"} {
		if f := p.Descriptor().Fields().ByName(name); f != nil {
			p.Clear(f)
		}
	}
	if f := p.Descriptor().Fields().ByName("tags"); f != nil {
		l := p.Get(f).List()
		tags := []string{}
		for i := 0; i < l.Len(); i++ {
			t := l.Get(i).String()
			if !strings.HasPrefix(t, "plan:") {
				tags = append(tags, t)
			}
		}
		sort.Strings(tags)
		p.Clear(f)
		for _, t := range tags {
			p.Mutable(f).List().Append(protoreflect.ValueOfString(t))
		}
	}
	if view := c.GetView(); view != nil && view.Markdown != nil {
		// Only the persisted attachment belongs to the view primitive. Runtime
		// permissions and filesystem status are not workspace edits.
		note := view.Markdown
		view.Markdown = &pb.ViewMarkdownDocument{Path: note.Path, IsManaged: note.IsManaged}
	}
	b, _ := proto.MarshalOptions{Deterministic: true}.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Dependencies(g Graph, r *pb.PlanResource) []string {
	out := []string{}
	add := func(k string, id int32) {
		if id != 0 {
			out = append(out, g.Find(k, id))
		}
	}
	switch v := r.GetValue().(type) {
	case *pb.PlanResource_Connector:
		add("view", v.Connector.ViewId)
		add("element", v.Connector.SourceElementId)
		add("element", v.Connector.TargetElementId)
	case *pb.PlanResource_Placement:
		add("view", v.Placement.ViewId)
		add("element", v.Placement.ElementId)
	case *pb.PlanResource_View:
		add("element", v.View.GetOwnerElementId())
	case *pb.PlanResource_Layer:
		add("view", v.Layer.ViewId)
	}
	return out
}
func Target(op *pb.PlanOperation) string {
	switch a := op.Action.(type) {
	case *pb.PlanOperation_Create:
		return a.Create.Key
	case *pb.PlanOperation_Update:
		return a.Update.GetResource().GetKey()
	case *pb.PlanOperation_Remove:
		return a.Remove.Key
	case *pb.PlanOperation_Include:
		return a.Include.ResourceKey
	case *pb.PlanOperation_Exclude:
		return a.Exclude.Key
	case *pb.PlanOperation_Note:
		return a.Note.ViewKey
	}
	return ""
}

// ApplyMetadata validates and records plan-only scope and checklist edits.
func (s *State) ApplyMetadata(op *pb.PlanOperation) error {
	k := Target(op)
	switch a := op.Action.(type) {
	case *pb.PlanOperation_Include:
		if s.Graph[k] == nil {
			return fmt.Errorf("included resource does not exist")
		}
		switch a.Include.Role {
		case "change", "preserve", "context":
		default:
			return fmt.Errorf("invalid member role")
		}
		s.Members[k] = proto.Clone(a.Include).(*pb.PlanMember)
		s.Members[k].Explicit = true
		return nil
	case *pb.PlanOperation_Exclude:
		delete(s.Members, k)
		return nil
	case *pb.PlanOperation_Checklist:
		if a.Checklist.Id == "" || strings.TrimSpace(a.Checklist.Text) == "" {
			return fmt.Errorf("checklist ID and text are required")
		}
		for _, key := range a.Checklist.ResourceKeys {
			if s.Members[key] == nil {
				return fmt.Errorf("checklist target %s is outside plan", key)
			}
		}
		s.Checklist[a.Checklist.Id] = proto.Clone(a.Checklist).(*pb.PlanChecklistItem)
		return nil
	case *pb.PlanOperation_RemoveChecklist:
		delete(s.Checklist, a.RemoveChecklist.Key)
		return nil
	case *pb.PlanOperation_Note:
		if s.Graph[k] == nil || s.Graph[k].GetView() == nil {
			return fmt.Errorf("note requires a view")
		}
		if strings.TrimSpace(a.Note.Content) == "" && s.Notes[k] == nil {
			return nil
		}
		s.Notes[k] = proto.Clone(a.Note).(*pb.PlanNote)
		if s.Members[k] == nil {
			s.Members[k] = &pb.PlanMember{ResourceKey: k, Role: "change"}
		}
		return nil
	default:
		return fmt.Errorf("unsupported plan operation")
	}
}
