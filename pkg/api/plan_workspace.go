package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"

	rpc "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/plans"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func planJSON(m proto.Message) map[string]any {
	b, _ := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(m)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// Run the ordinary handler so plan mode has exactly the normal mutation semantics.
func (s *PlanService) liveWorkspace(ctx context.Context, st *app.Store, method string, in map[string]any) (map[string]any, error) {
	fields := map[string]any{}
	for k, v := range in {
		if k == "tags" {
			if tags, ok := v.([]any); ok {
				clean := []any{}
				for _, tag := range tags {
					if text, ok := tag.(string); ok && !strings.HasPrefix(text, "plan:") {
						clean = append(clean, text)
					}
				}
				v = clean
			}
		}
		fields[k] = v
	}
	descriptor := pb.File_diag_v1_workspace_service_proto.Services().ByName("WorkspaceService").Methods().ByName(protoreflect.Name(method))
	if descriptor != nil && descriptor.Input().Fields().ByName("org_id") != nil {
		scope := WorkspaceIDFromCtx(ctx)
		// Local APIAdapter is unscoped; CLI uses the nil context UUID.
		if scope == uuid.Nil {
			scope = uuid.NameSpaceOID
		}
		fields["org_id"] = scope.String()
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	hooks := s.Hooks
	if pending, ok := ctx.Value(planHooksKey{}).(*planCommitHooks); ok {
		hooks = pending
	}
	path, h := rpc.NewWorkspaceServiceHandler(&WorkspaceService{Store: s.Source(st), Hooks: hooks})
	req := httptest.NewRequest("POST", path+method, bytes.NewReader(data)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code >= 400 {
		var failure struct {
			Code    string
			Message string
		}
		_ = json.Unmarshal(w.Body.Bytes(), &failure)
		return nil, fmt.Errorf("%s: %s", failure.Code, failure.Message)
	}
	out := map[string]any{}
	err = json.Unmarshal(w.Body.Bytes(), &out)
	// Use proto field names just like the existing plan transport.
	if err == nil {
		data, _ = json.Marshal(out)
		desc := pb.File_diag_v1_workspace_service_proto.Services().ByName("WorkspaceService").Methods().ByName(protoreflect.Name(method))
		if desc != nil {
			m := dynamicpb.NewMessage(desc.Output())
			if err = protojson.Unmarshal(data, m); err == nil {
				out = planJSON(m)
			}
		}
	}
	return out, err
}

func (s *PlanService) PlanWorkspace(ctx context.Context, id, method string, in map[string]any) (map[string]any, error) {
	if result, handled, err := s.lockedWorkspace(ctx, method, in); handled || err != nil {
		return result, err
	}
	read := strings.HasPrefix(method, "Get") || strings.HasPrefix(method, "List") || method == "ExportWorkspace"
	if err := s.access(ctx, !read); err != nil {
		return nil, err
	}
	ctx, notifications := s.commitHooks(ctx)
	var out map[string]any
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		p, err := st.Plan(ctx, planScope(ctx), id)
		if err != nil {
			return err
		}
		wc, _ := ctx.Value(planWriteContextKey{}).(*planWriteContext)
		if wc != nil {
			wc.observed = p.Revision
		}
		if read {
			out, err = s.liveWorkspace(ctx, st, method, in)
			if err != nil {
				return err
			}
			if method == "ExportWorkspace" {
				return nil
			}
			return s.decoratePlanRead(ctx, st, p, out)
		}
		if p.Status == "archived" || p.Status == "completed" {
			return fmt.Errorf("archived plan is read-only")
		}
		request := uuid.NewString()
		if wc != nil && wc.requestID != "" {
			request = wc.requestID
		}
		data, _ := json.Marshal(in)
		hash := fmt.Sprintf("%x", sha256.Sum256(append([]byte(method), data...)))
		cached, err := st.PlanWorkspaceRequest(ctx, p.Id, request, hash)
		if err != nil {
			return err
		}
		if cached != "" {
			return json.Unmarshal([]byte(cached), &out)
		}
		if wc != nil && wc.expected != nil && p.Revision != *wc.expected {
			return app.ErrPlanConflict
		}
		before, err := s.baseline(ctx, st)
		if err != nil {
			return err
		}
		out, err = s.liveWorkspace(ctx, st, method, in)
		if err != nil {
			return err
		}
		after, err := s.baseline(ctx, st)
		if err != nil {
			return err
		}
		ops := plans.Diff(before, after)
		if method == "SaveViewMarkdown" || method == "CreateViewMarkdown" {
			if n, ok := in["view_id"].(float64); ok {
				content, _ := in["content"].(string)
				if method == "CreateViewMarkdown" {
					content, _ = in["initial_content"].(string)
				}
				ops = append(ops, &pb.PlanOperation{Action: &pb.PlanOperation_Note{Note: &pb.PlanNote{ViewKey: plans.Key("view", int32(n)), Content: content}}})
			}
		}
		if err = s.recordOperations(ctx, st, p, request, hash, ops); err != nil {
			return err
		}
		if wc != nil {
			wc.observed = p.Revision
		}
		if err = s.decoratePlanRead(ctx, st, p, out); err != nil {
			return err
		}
		response, _ := json.Marshal(out)
		return st.SavePlanWorkspaceRequest(ctx, p.Id, request, hash, string(response))
	})
	if err == nil {
		notifications.flush()
	}
	return out, planErr(err)
}

func (s *PlanService) decoratePlanRead(ctx context.Context, st *app.Store, p *pb.Plan, out map[string]any) error {
	g, err := s.baseline(ctx, st)
	if err != nil {
		return err
	}
	ops, err := st.PlanOperations(ctx, p.Id, p.Revision)
	if err != nil {
		return err
	}
	state := plans.Recorded(g, ops)
	tags := map[string][]string{}
	for _, r := range plans.Project(g, state, 0, true) {
		tags[r.Resource.Key] = r.Tags
	}
	var visit func(any, string)
	visit = func(value any, kind string) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				visit(item, kind)
			}
		case map[string]any:
			if n, ok := v["id"].(float64); ok {
				k := kind
				if _, placed := v["element_id"]; placed && kind == "elementplacement" {
					n, _ = v["element_id"].(float64)
					k = "element"
				}
				if ts := tags[plans.Key(k, int32(n))]; len(ts) > 0 {
					existing, _ := v["tags"].([]any)
					for _, tag := range ts {
						existing = append(existing, tag)
					}
					v["tags"] = existing
				}
			}
			for key, item := range v {
				next := ""
				switch key {
				case "element", "elements":
					next = "element"
				case "view", "views":
					next = "view"
				case "connector", "connectors":
					next = "connector"
				case "placement", "placements":
					next = "elementplacement"
				case "layer", "layers":
					next = "viewlayer"
				}
				visit(item, next)
			}
		}
	}
	visit(out, "")
	return nil
}

func (s *PlanService) recordOperations(ctx context.Context, st *app.Store, p *pb.Plan, request, hash string, ops []*pb.PlanOperation) error {
	if len(ops) == 0 {
		return nil
	}
	old := p.Revision
	p.Revision++
	p.Status = "draft"
	p.UpdatedAt = nowPlan()
	if err := st.SavePlan(ctx, planScope(ctx), p, old); err != nil {
		return err
	}
	for i, op := range ops {
		op.Id = uuid.NewString()
		op.Revision = p.Revision
		op.Index = uint32(i)
		op.SchemaVersion = 2
		if op.GestureId == "" {
			op.GestureId = request
		}
		if op.AuthorKind == "" {
			op.AuthorKind = "user"
		}
		if err := st.SavePlanOperation(ctx, p.Id, request, hash, op); err != nil {
			return err
		}
	}
	g, err := s.baseline(ctx, st)
	if err != nil {
		return err
	}
	all, err := st.PlanOperations(ctx, p.Id, p.Revision)
	if err != nil {
		return err
	}
	return st.SavePlanState(ctx, p.Id, plans.Recorded(g, all))
}

func (s *PlanService) applyLiveOperation(ctx context.Context, st *app.Store, op *pb.PlanOperation, g plans.Graph) error {
	if n := op.GetNote(); n != nil {
		r := g[n.ViewKey]
		if r == nil || r.GetView() == nil {
			return fmt.Errorf("note requires an existing view")
		}
		method := "SaveViewMarkdown"
		in := map[string]any{"view_id": r.GetView().Id, "content": n.Content}
		if r.GetView().Markdown == nil {
			if strings.TrimSpace(n.Content) == "" {
				return nil
			}
			method = "CreateViewMarkdown"
			in = map[string]any{"view_id": r.GetView().Id, "initial_content": n.Content, "target_kind": "PRIVATE_APP"}
		}
		_, err := s.liveWorkspace(ctx, st, method, in)
		return err
	}
	r := op.GetCreate()
	prefix := "Create"
	if patch := op.GetUpdate(); patch != nil {
		r = g[patch.GetResource().GetKey()]
		prefix = "Update"
	}
	if remove := op.GetRemove(); remove != nil {
		r = g[remove.Key]
		prefix = "Delete"
	}
	if r == nil {
		return fmt.Errorf("resource no longer exists")
	}
	kind := plans.Kind(r)
	suffix, idField := "", ""
	switch kind {
	case "element":
		suffix, idField = "Element", "element_id"
	case "view":
		suffix, idField = "View", "view_id"
	case "connector":
		suffix, idField = "Connector", "connector_id"
	case "elementplacement":
		suffix, idField = "Placement", "placement_id"
	case "viewlayer":
		suffix, idField = "ViewLayer", "layer_id"
	default:
		return fmt.Errorf("unsupported resource %s", kind)
	}
	fields := planJSON(plans.Value(r))
	if patch := op.GetUpdate(); patch != nil {
		if len(patch.GetFields().GetPaths()) == 0 {
			return fmt.Errorf("update requires fields")
		}
		value := planJSON(plans.Value(patch.Resource))
		for _, field := range patch.Fields.Paths {
			if field == "id" || field == "org_id" || field == "created_at" || field == "updated_at" {
				return fmt.Errorf("field %s cannot be updated", field)
			}
			if _, ok := value[field]; !ok {
				return fmt.Errorf("unknown field %s", field)
			}
			fields[field] = value[field]
		}
	}
	delete(fields, "id")
	delete(fields, "org_id")
	delete(fields, "created_at")
	delete(fields, "updated_at")
	if prefix != "Create" {
		fields[idField] = plans.ID(r)
	}
	if placement := r.GetPlacement(); placement != nil && prefix != "Create" {
		fields["view_id"] = placement.ViewId
		fields["element_id"] = placement.ElementId
		if prefix == "Update" {
			suffix = "PlacementPosition"
		}
	}
	// Ignore output-only primitive fields when calling the existing request contract.
	desc := pb.File_diag_v1_workspace_service_proto.Services().ByName("WorkspaceService").Methods().ByName(protoreflect.Name(prefix + suffix))
	if desc == nil {
		return fmt.Errorf("unsupported workspace operation %s%s", prefix, suffix)
	}
	for key := range fields {
		if desc.Input().Fields().ByName(protoreflect.Name(key)) == nil {
			delete(fields, key)
		}
	}
	_, err := s.liveWorkspace(ctx, st, prefix+suffix, fields)
	return err
}
