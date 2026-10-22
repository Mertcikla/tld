package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/plans"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PlanService shares the core store and the host's existing workspace hooks.
// Source supplies a transaction-bound workspace adapter; it must not cache it.
type PlanService struct {
	RepositoryRevision func(context.Context, string) (string, error)
	Store              *app.Store
	Source             func(*app.Store) Store
	Hooks              WorkspaceHooks
	NotesDir           string
}

func (s *PlanService) access(ctx context.Context, write bool) error {
	h := s.Hooks
	if h == nil {
		h = NopWorkspaceHooks{}
	}
	if write {
		return h.CheckWrite(ctx, WorkspaceIDFromCtx(ctx), "plans")
	}
	return h.CheckRead(ctx, WorkspaceIDFromCtx(ctx))
}
func planErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, app.ErrPlanConflict) {
		return connect.NewError(connect.CodeAborted, err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return err
	}
	return connect.NewError(connect.CodeInvalidArgument, err)
}
func planScope(ctx context.Context) string { return WorkspaceIDFromCtx(ctx).String() }
func nowPlan() string                      { return time.Now().UTC().Format(time.RFC3339Nano) }
func (s *PlanService) CreatePlan(ctx context.Context, req *connect.Request[pb.CreatePlanRequest]) (*connect.Response[pb.PlanResponse], error) {
	if err := s.access(ctx, true); err != nil {
		return nil, err
	}
	m := req.Msg
	if strings.TrimSpace(m.RepositoryId) == "" {
		return nil, invalidArg("repository_id", "required")
	}
	var p *pb.Plan
	resumed := false
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		if m.Resume {
			items, e := st.Plans(ctx, planScope(ctx), m.RepositoryId, false)
			if e != nil {
				return e
			}
			for _, v := range items {
				if v.Status == "draft" {
					p = v
					resumed = true
					return nil
				}
			}
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = "New plan"
		}
		p = &pb.Plan{Id: uuid.NewString(), RepositoryId: m.RepositoryId, Name: name, Objective: m.Objective, Status: "draft", CreatedAt: nowPlan(), UpdatedAt: nowPlan()}
		return st.CreatePlan(ctx, planScope(ctx), p)
	})
	if err != nil {
		return nil, planErr(err)
	}
	return connect.NewResponse(&pb.PlanResponse{Plan: p, Resumed: resumed}), nil
}
func (s *PlanService) ListPlans(ctx context.Context, req *connect.Request[pb.ListPlansRequest]) (*connect.Response[pb.ListPlansResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	p, err := s.Store.Plans(ctx, planScope(ctx), req.Msg.RepositoryId, req.Msg.IncludeArchived)
	if err != nil {
		return nil, planErr(err)
	}
	return connect.NewResponse(&pb.ListPlansResponse{Plans: p}), nil
}
func (s *PlanService) GetPlan(ctx context.Context, req *connect.Request[pb.GetPlanRequest]) (*connect.Response[pb.PlanResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	p, err := s.Store.Plan(ctx, planScope(ctx), req.Msg.PlanId)
	if err == nil && req.Msg.Revision != nil {
		p, err = s.Store.PlanRevision(ctx, planScope(ctx), req.Msg.PlanId, *req.Msg.Revision)
	}
	if err != nil {
		return nil, planErr(err)
	}
	return connect.NewResponse(&pb.PlanResponse{Plan: p}), nil
}
func (s *PlanService) UpdatePlan(ctx context.Context, req *connect.Request[pb.UpdatePlanRequest]) (*connect.Response[pb.PlanResponse], error) {
	if err := s.access(ctx, true); err != nil {
		return nil, err
	}
	m := req.Msg
	var p *pb.Plan
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		lock, lockErr := readImplementation(ctx, st)
		if lockErr != nil {
			return lockErr
		}
		var err error
		p, err = st.Plan(ctx, planScope(ctx), m.PlanId)
		if err != nil {
			return err
		}
		if lock.Locked && lock.PlanID == p.Id {
			return fmt.Errorf("unlock implementation before changing the reviewed plan")
		}
		if p.Revision != m.ExpectedRevision {
			return app.ErrPlanConflict
		}
		if (p.Status == "archived" || p.Status == "completed") && (m.Status == nil || *m.Status != "draft") {
			return fmt.Errorf("reopen this plan as a draft before editing it")
		}
		if m.Name != nil {
			if strings.TrimSpace(*m.Name) == "" {
				return fmt.Errorf("name is required")
			}
			p.Name = *m.Name
			p.Status = "draft"
		}
		if m.Objective != nil {
			p.Objective = *m.Objective
			p.Status = "draft"
		}
		p.Revision++
		if m.Status != nil {
			switch *m.Status {
			case "draft", "ready", "archived", "completed":
				p.Status = *m.Status
			default:
				return fmt.Errorf("invalid plan status")
			}
			if p.Status == "ready" {
				p.ReadyRevision = p.Revision
			}
		}
		p.UpdatedAt = nowPlan()
		return st.SavePlan(ctx, planScope(ctx), p, m.ExpectedRevision)
	})
	if err != nil {
		return nil, planErr(err)
	}
	return connect.NewResponse(&pb.PlanResponse{Plan: p}), nil
}
func (s *PlanService) baseline(ctx context.Context, st *app.Store) (plans.Graph, error) {
	source := s.Source(st)
	scope := WorkspaceIDFromCtx(ctx)
	views, err := source.ListViews(ctx, scope)
	if err != nil {
		return nil, err
	}
	elements, _, err := source.ListElements(ctx, scope, 0, 0, "")
	if err != nil {
		return nil, err
	}
	connectors, err := source.ListAllConnectors(ctx, scope)
	if err != nil {
		return nil, err
	}
	placements, err := source.ListAllPlacements(ctx, scope)
	if err != nil {
		return nil, err
	}
	layers, err := source.ListAllViewLayers(ctx, scope)
	if err != nil {
		return nil, err
	}
	exported := &pb.ExportOrganizationResponse{Views: views, Elements: elements, Connectors: connectors, Layers: layers}
	for _, v := range placements {
		exported.Placements = append(exported.Placements, &pb.ElementPlacement{Id: v.Id, ViewId: v.ViewId, ElementId: v.ElementId, PositionX: v.PositionX, PositionY: v.PositionY})
	}
	return plans.FromExport(exported), nil
}
func (s *PlanService) AppendOperations(ctx context.Context, req *connect.Request[pb.AppendPlanOperationsRequest]) (*connect.Response[pb.PlanResponse], error) {
	if err := s.access(ctx, true); err != nil {
		return nil, err
	}
	m := req.Msg
	if m.RequestId == "" || len(m.Operations) == 0 {
		return nil, invalidArg("request_id/operations", "required")
	}
	ctx, notifications := s.commitHooks(ctx)
	var p *pb.Plan
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		lock, lockErr := readImplementation(ctx, st)
		if lockErr != nil {
			return lockErr
		}
		if lock.Locked {
			return fmt.Errorf("workspace is locked for implementation; use ordinary workspace commands to stage proof-of-work")
		}
		var err error
		p, err = st.Plan(ctx, planScope(ctx), m.PlanId)
		if err != nil {
			return err
		}
		requestBytes, _ := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		requestHash := fmt.Sprintf("%x", sha256.Sum256(requestBytes))
		seen, err := st.PlanRequestExists(ctx, p.Id, m.RequestId, requestHash)
		if err != nil || seen {
			return err
		}
		if p.Revision != m.ExpectedRevision {
			return app.ErrPlanConflict
		}
		if p.Status == "archived" || p.Status == "completed" {
			return fmt.Errorf("archived plan is read-only")
		}
		base, err := s.baseline(ctx, st)
		if err != nil {
			return err
		}
		existing, err := st.PlanOperations(ctx, p.Id, p.Revision)
		if err != nil {
			return err
		}
		state := plans.Recorded(base, existing)
		recorded := []*pb.PlanOperation{}
		for _, raw := range m.Operations {
			op := proto.Clone(raw).(*pb.PlanOperation)
			op.SchemaVersion = 2
			if c := op.GetChecklist(); c != nil && c.Id == "" {
				c.Id = uuid.NewString()
			}
			if op.GetCreate() != nil || op.GetUpdate() != nil || op.GetRemove() != nil || op.GetNote() != nil {
				before, err := s.baseline(ctx, st)
				if err != nil {
					return err
				}
				if err = s.applyLiveOperation(ctx, st, op, before); err != nil {
					return err
				}
				after, err := s.baseline(ctx, st)
				if err != nil {
					return err
				}
				recorded = append(recorded, plans.Diff(before, after)...)
				if op.GetNote() != nil {
					recorded = append(recorded, op)
				}
				state = plans.Recorded(after, append(append([]*pb.PlanOperation{}, existing...), recorded...))
			} else {
				if err = state.ApplyMetadata(op); err != nil {
					return err
				}
				recorded = append(recorded, op)
			}
		}
		return s.recordOperations(ctx, st, p, m.RequestId, requestHash, recorded)
	})
	if err != nil {
		return nil, planErr(err)
	}
	notifications.flush()
	return connect.NewResponse(&pb.PlanResponse{Plan: p}), nil
}
func (s *PlanService) ListOperations(ctx context.Context, req *connect.Request[pb.GetPlanRequest]) (*connect.Response[pb.ListPlanOperationsResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	p, err := s.Store.Plan(ctx, planScope(ctx), req.Msg.PlanId)
	if err != nil {
		return nil, planErr(err)
	}
	rev := p.Revision
	if req.Msg.Revision != nil {
		rev = *req.Msg.Revision
	}
	if rev > p.Revision {
		return nil, invalidArg("revision", "beyond current plan")
	}
	ops, err := s.Store.PlanOperations(ctx, p.Id, rev)
	if err != nil {
		return nil, planErr(err)
	}
	return connect.NewResponse(&pb.ListPlanOperationsResponse{Operations: ops, Revision: rev}), nil
}
func (s *PlanService) state(ctx context.Context, id string, revision *uint32) (*pb.Plan, plans.Graph, *plans.State, error) {
	var p *pb.Plan
	var base plans.Graph
	var state *plans.State
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		var err error
		p, err = st.Plan(ctx, planScope(ctx), id)
		if err != nil {
			return err
		}
		id = p.Id
		if revision != nil {
			if *revision > p.Revision {
				return fmt.Errorf("revision beyond current plan")
			}
			p, err = st.PlanRevision(ctx, planScope(ctx), id, *revision)
			if err != nil {
				return err
			}
		}
		// A handed-off revision reads its frozen baseline, even after implementation
		// finishes or a different plan starts. Drafts continue to use the live diagram.
		_, base, err = readHandoff(ctx, st, p)
		if err != nil {
			return err
		}
		if base == nil {
			base, err = s.baseline(ctx, st)
		}
		if err != nil {
			return err
		}
		ops, err := st.PlanOperations(ctx, id, p.Revision)
		if err != nil {
			return err
		}
		state = plans.Recorded(base, ops)
		return nil
	})
	return p, base, state, err
}
func (s *PlanService) GetProjection(ctx context.Context, req *connect.Request[pb.PlanProjectionRequest]) (*connect.Response[pb.PlanProjectionResponse], error) {
	return s.Preview(ctx, req)
}
func (s *PlanService) Preview(ctx context.Context, req *connect.Request[pb.PlanProjectionRequest]) (*connect.Response[pb.PlanProjectionResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	m := req.Msg
	p, base, state, err := s.state(ctx, m.PlanId, m.Revision)
	if err != nil {
		return nil, planErr(err)
	}
	if m.Radius > 20 {
		return nil, invalidArg("radius", "maximum is 20")
	}
	resources := plans.Project(base, state, m.Radius, m.FullWorkspace)
	cursorInput := proto.Clone(m).(*pb.PlanProjectionRequest)
	cursorInput.Cursor = ""
	cursorInput.PageSize = 0
	cursorInput.Revision = proto.Uint32(p.Revision)
	cursorBytes, _ := proto.MarshalOptions{Deterministic: true}.Marshal(cursorInput)
	cursorHash := fmt.Sprintf("%x", sha256.Sum256(cursorBytes))
	start := 0
	if m.Cursor != "" {
		parts := strings.Split(m.Cursor, ":")
		if len(parts) != 3 || parts[0] != strconv.Itoa(int(p.Revision)) || parts[1] != cursorHash {
			return nil, connect.NewError(connect.CodeAborted, errors.New("projection changed; restart pagination"))
		}
		start, err = strconv.Atoi(parts[2])
		if err != nil || start < 0 || start > len(resources) {
			return nil, invalidArg("cursor", "invalid")
		}
	}
	size := int(m.PageSize)
	if size == 0 {
		size = 500
	}
	if size > 2000 {
		size = 2000
	}
	end := min(start+size, len(resources))
	next := ""
	if end < len(resources) {
		next = fmt.Sprintf("%d:%s:%d", p.Revision, cursorHash, end)
	}
	return connect.NewResponse(&pb.PlanProjectionResponse{Plan: p, Revision: p.Revision, Resources: resources[start:end], Operations: state.Results, NextCursor: next, Complete: next == ""}), nil
}
func (s *PlanService) GetChecklist(ctx context.Context, req *connect.Request[pb.GetPlanRequest]) (*connect.Response[pb.PlanChecklistResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	p, _, state, err := s.state(ctx, req.Msg.PlanId, req.Msg.Revision)
	if err != nil {
		return nil, planErr(err)
	}
	items := []*pb.PlanChecklistItem{}
	for _, v := range state.Checklist {
		items = append(items, v)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Id < items[j].Id })
	return connect.NewResponse(&pb.PlanChecklistResponse{Items: items, Revision: p.Revision}), nil
}
func (s *PlanService) GetPrompt(ctx context.Context, req *connect.Request[pb.GetPlanRequest]) (*connect.Response[pb.PlanPromptResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	p, base, state, err := s.state(ctx, req.Msg.PlanId, req.Msg.Revision)
	if err != nil {
		return nil, planErr(err)
	}
	text := plans.Prompt(p, base, state)
	h, _, err := readHandoff(ctx, s.Store, p)
	if err != nil {
		return nil, planErr(err)
	}
	if h != nil && h.GitRevision != "" {
		text += "\nRepository baseline: " + h.GitRevision + "\n"
	}
	return connect.NewResponse(&pb.PlanPromptResponse{Text: text, Revision: p.Revision}), nil
}
func (s *PlanService) GetNote(ctx context.Context, req *connect.Request[pb.PlanNoteRequest]) (*connect.Response[pb.PlanNoteResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	p, _, state, err := s.state(ctx, req.Msg.PlanId, req.Msg.Revision)
	if err != nil {
		return nil, planErr(err)
	}
	r := state.Graph[req.Msg.ViewKey]
	if r == nil || r.GetView() == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	h, _, err := readHandoff(ctx, s.Store, p)
	if err != nil {
		return nil, planErr(err)
	}
	if h != nil {
		note := &pb.PlanNote{ViewKey: r.Key}
		if raw := h.Notes[r.Key]; len(raw) > 0 {
			if err = protojson.Unmarshal(raw, note); err != nil {
				return nil, planErr(err)
			}
		}
		return connect.NewResponse(&pb.PlanNoteResponse{Note: note, Revision: p.Revision}), nil
	}
	_, content, e := s.Source(s.Store).GetViewMarkdown(ctx, r.GetView().Id, WorkspaceIDFromCtx(ctx))
	if e != nil {
		return nil, planErr(e)
	}
	n := &pb.PlanNote{ViewKey: r.Key, Content: content}
	if r.GetView().Markdown != nil {
		n.OriginalPath = r.GetView().Markdown.Path
		n.OriginalVersion = r.GetView().Markdown.GetFileVersion()
	}
	return connect.NewResponse(&pb.PlanNoteResponse{Note: n, Revision: p.Revision}), nil
}
