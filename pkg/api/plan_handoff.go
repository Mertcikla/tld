package api

import (
	"context"
	"encoding/json"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/plans"
	"google.golang.org/protobuf/encoding/protojson"
)

// Protobufs are stored using their wire JSON, never encoding/json's oneof layout.
type planHandoff struct {
	Workspace   json.RawMessage
	Notes       map[string]json.RawMessage
	GitRevision string
}

func (s *PlanService) freezeHandoff(ctx context.Context, st *app.Store, p *pb.Plan, gitRevision string) error {
	base, err := s.baseline(ctx, st)
	if err != nil {
		return err
	}
	ops, err := st.PlanOperations(ctx, p.Id, p.Revision)
	if err != nil {
		return err
	}
	state := plans.Recorded(base, ops)
	snapshot := &pb.PlanProjectionResponse{Plan: p}
	for _, item := range plans.Project(base, state, 0, true) {
		// Removed resources are kept in the operation history, not the live baseline.
		if base[item.Resource.Key] != nil {
			snapshot.Resources = append(snapshot.Resources, &pb.PlanProjectedResource{Resource: item.Resource})
		}
	}
	data, err := protojson.Marshal(snapshot)
	if err != nil {
		return err
	}
	h := planHandoff{Workspace: data, Notes: map[string]json.RawMessage{}, GitRevision: gitRevision}
	for key, r := range base {
		view := r.GetView()
		if view == nil || view.Markdown == nil {
			continue
		}
		_, content, e := s.Source(st).GetViewMarkdown(ctx, view.Id, WorkspaceIDFromCtx(ctx))
		if e != nil {
			return fmt.Errorf("read note for %s before starting implementation: %w", view.Name, e)
		}
		note := &pb.PlanNote{ViewKey: key, Content: content, OriginalPath: view.Markdown.Path, OriginalVersion: view.Markdown.GetFileVersion()}
		h.Notes[key], err = protojson.Marshal(note)
		if err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return st.SavePlanHandoff(ctx, p.Id, p.Revision, string(encoded))
}

func readHandoff(ctx context.Context, st *app.Store, p *pb.Plan) (*planHandoff, plans.Graph, error) {
	raw, err := st.PlanHandoff(ctx, p.Id, p.Revision)
	if err != nil || raw == "" {
		return nil, nil, err
	}
	h := &planHandoff{}
	if err = json.Unmarshal([]byte(raw), h); err != nil {
		return nil, nil, err
	}
	snapshot := &pb.PlanProjectionResponse{}
	if err = protojson.Unmarshal(h.Workspace, snapshot); err != nil {
		return nil, nil, err
	}
	base := plans.Graph{}
	for _, r := range snapshot.Resources {
		if r.Resource != nil {
			base[r.Resource.Key] = r.Resource
		}
	}
	return h, base, nil
}
