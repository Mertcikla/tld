package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	maxFlowSteps       = app.MaxFlowSteps
	maxFlowPaths       = app.MaxFlowPaths
	maxFlowBranchDepth = app.MaxFlowBranchDepth
)

type FlowService struct {
	diagv1connect.UnimplementedFlowServiceHandler
	Store Store
	Hooks WorkspaceHooks
}

func (s *FlowService) hooks() WorkspaceHooks {
	if s.Hooks != nil {
		return s.Hooks
	}
	return NopWorkspaceHooks{}
}

// An omitted workspace uses the host context, including nil for standalone use.
func flowWorkspaceID(ctx context.Context, requested string) (uuid.UUID, error) {
	current := WorkspaceIDFromCtx(ctx)
	if requested == "" {
		return current, nil
	}
	parsed, err := ResolveWorkspaceID(ctx, requested)
	if err != nil {
		return uuid.Nil, err
	}
	if current != uuid.Nil && current != parsed {
		return uuid.Nil, invalidArg("org_id", "does not match the current workspace")
	}
	return parsed, nil
}

// flowRequestContext applies an explicitly requested workspace to both API and
// store context. This keeps hooks and tenant-filtered storage on the same scope
// when a core caller selects a workspace without host middleware.
func flowRequestContext(ctx context.Context, requested string) (context.Context, uuid.UUID, error) {
	workspaceID, err := flowWorkspaceID(ctx, requested)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if workspaceID != uuid.Nil {
		ctx = WithWorkspaceID(ctx, workspaceID)
	}
	return ctx, workspaceID, nil
}

func flowViewError(action string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s: view not found", action))
	}
	return storeErr(action, err)
}

func flowFromRecord(row app.FlowRecord) (*diagv1.Flow, error) {
	steps, err := flowStepsToProto(row.Steps)
	if err != nil {
		return nil, err
	}
	return &diagv1.Flow{Id: row.ID, ViewId: int32(row.ViewID), Revision: row.Revision,
		Content: &diagv1.FlowContent{Name: row.Name, Description: row.Description, Steps: steps}}, nil
}

func flowStepsToProto(steps []app.FlowStep) ([]*diagv1.FlowStep, error) {
	result := make([]*diagv1.FlowStep, 0, len(steps))
	for _, step := range steps {
		item := &diagv1.FlowStep{Id: step.ID, Title: step.Title, Description: step.Description, ElementIds: step.ElementIDs, ConnectorIds: step.ConnectorIDs}
		if step.Branches != nil {
			mode := diagv1.FlowBranchMode_FLOW_BRANCH_MODE_ALTERNATE
			switch step.Branches.Mode {
			case "alternate":
			case "parallel":
				mode = diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL
			default:
				return nil, fmt.Errorf("unknown flow branch mode %q", step.Branches.Mode)
			}
			branches := &diagv1.FlowBranches{Mode: mode}
			for _, path := range step.Branches.Paths {
				children, err := flowStepsToProto(path.Steps)
				if err != nil {
					return nil, err
				}
				branches.Paths = append(branches.Paths, &diagv1.FlowBranch{Id: path.ID, Name: path.Name, Steps: children})
			}
			item.Control = &diagv1.FlowStep_Branches{Branches: branches}
		} else if step.GoToFlowID != "" {
			item.Control = &diagv1.FlowStep_GoToFlowId{GoToFlowId: step.GoToFlowID}
		}
		result = append(result, item)
	}
	return result, nil
}

func flowStepsFromProto(steps []*diagv1.FlowStep) []app.FlowStep {
	result := make([]app.FlowStep, 0, len(steps))
	for _, step := range steps {
		item := app.FlowStep{ID: step.Id, Title: step.Title, Description: step.Description, ElementIDs: step.ElementIds, ConnectorIDs: step.ConnectorIds, GoToFlowID: step.GetGoToFlowId()}
		if branches := step.GetBranches(); branches != nil {
			mode := "alternate"
			if branches.Mode == diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL {
				mode = "parallel"
			}
			item.Branches = &app.FlowBranches{Mode: mode}
			for _, path := range branches.Paths {
				item.Branches.Paths = append(item.Branches.Paths, app.FlowPath{ID: path.Id, Name: path.Name, Steps: flowStepsFromProto(path.Steps)})
			}
		}
		result = append(result, item)
	}
	return result
}

func (s *FlowService) ListFlows(ctx context.Context, req *connect.Request[diagv1.ListFlowsRequest]) (*connect.Response[diagv1.ListFlowsResponse], error) {
	ctx, workspaceID, err := flowRequestContext(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	if req.Msg.ViewId < 0 {
		return nil, invalidArg("view_id", "must be zero or positive")
	}
	if req.Msg.ViewId != 0 {
		if _, err := s.Store.GetView(ctx, req.Msg.ViewId, workspaceID); err != nil {
			return nil, flowViewError("get view", err)
		}
	}
	rows, err := s.Store.ListFlows(ctx, int64(req.Msg.ViewId))
	if err != nil {
		return nil, storeErr("list flows", err)
	}
	response := &diagv1.ListFlowsResponse{}
	for _, row := range rows {
		flow, err := flowFromRecord(row)
		if err != nil {
			return nil, storeErr("read flow", err)
		}
		response.Flows = append(response.Flows, flow)
	}
	return connect.NewResponse(response), nil
}

// ValidateFlowRequest validates a proposed save without performing any target writes.
// CLI previews use the same validation as FlowService.SaveFlow.
func ValidateFlowRequest(req *diagv1.SaveFlowRequest) error {
	if err := validateFlow(req); err != nil {
		return err
	}
	document, err := protojson.Marshal(req.Content)
	if err != nil {
		return err
	}
	if len(document) > 1<<20 {
		return errors.New("flow must be at most 1 MiB")
	}
	return nil
}

func validateFlow(req *diagv1.SaveFlowRequest) error {
	content := req.GetContent()
	if req.ViewId <= 0 || content == nil || strings.TrimSpace(content.Name) == "" {
		return errors.New("flow requires a view and a name")
	}
	if len(content.Name) > 200 || len(content.Description) > 10000 {
		return errors.New("flow name or description is too long")
	}
	if (req.Id == "" && req.ExpectedRevision != 0) || (req.Id != "" && req.ExpectedRevision <= 0) {
		return errors.New("invalid flow revision")
	}
	ids := map[string]bool{}
	count := 0
	var validateSteps func([]*diagv1.FlowStep, int) error
	validateSteps = func(steps []*diagv1.FlowStep, depth int) error {
		if depth > maxFlowBranchDepth {
			return errors.New("branches may be nested at most eight levels")
		}
		for _, step := range steps {
			count++
			if count > maxFlowSteps {
				return fmt.Errorf("flow may contain at most %d steps", maxFlowSteps)
			}
			if step == nil || strings.TrimSpace(step.Id) == "" || len(step.Id) > 100 || ids[step.Id] {
				return errors.New("step ids must be unique and nonempty")
			}
			ids[step.Id] = true
			switch control := step.Control.(type) {
			case *diagv1.FlowStep_GoToFlowId:
				if strings.TrimSpace(control.GoToFlowId) == "" || len(control.GoToFlowId) > 100 {
					return errors.New("invalid linked flow id")
				}
			case *diagv1.FlowStep_Branches:
				branch := control.Branches
				if branch == nil || (branch.Mode != diagv1.FlowBranchMode_FLOW_BRANCH_MODE_ALTERNATE && branch.Mode != diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL) {
					return errors.New("branches mode must be alternate or parallel")
				}
				if len(branch.Paths) < 2 || len(branch.Paths) > maxFlowPaths {
					return fmt.Errorf("branches require two to %d paths", maxFlowPaths)
				}
				for _, path := range branch.Paths {
					if path == nil || strings.TrimSpace(path.Id) == "" || len(path.Id) > 100 || ids[path.Id] || strings.TrimSpace(path.Name) == "" || len(path.Name) > 200 {
						return errors.New("paths require unique ids and nonempty names")
					}
					ids[path.Id] = true
					if err := validateSteps(path.Steps, depth+1); err != nil {
						return err
					}
				}
			}
			if len(step.Title) > 500 || len(step.Description) > 10000 {
				return errors.New("step title or description is too long")
			}
			for _, highlights := range [][]int32{step.ElementIds, step.ConnectorIds} {
				if len(highlights) > 500 {
					return errors.New("step may highlight at most 500 elements and 500 connections")
				}
				seen := map[int32]bool{}
				for _, id := range highlights {
					if id <= 0 || seen[id] {
						return errors.New("highlight ids must be positive and unique within a step")
					}
					seen[id] = true
				}
			}
		}
		return nil
	}
	return validateSteps(content.Steps, 0)
}

func flowStoreError(action string, err error) error {
	if errors.Is(err, app.ErrFlowConflict) {
		return connect.NewError(connect.CodeAborted, err)
	}
	return storeErr(action, err)
}

func (s *FlowService) SaveFlow(ctx context.Context, req *connect.Request[diagv1.SaveFlowRequest]) (*connect.Response[diagv1.SaveFlowResponse], error) {
	ctx, workspaceID, err := flowRequestContext(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckWrite(ctx, workspaceID, "flows"); err != nil {
		return nil, err
	}
	flow := req.Msg
	if err := validateFlow(flow); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if _, err := s.Store.GetView(ctx, flow.ViewId, workspaceID); err != nil {
		return nil, flowViewError("get view", err)
	}

	document, err := protojson.Marshal(flow.Content)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(document) > 1<<20 {
		return nil, invalidArg("flow", "must be at most 1 MiB")
	}
	row, err := s.Store.SaveFlow(ctx, app.FlowRecord{ID: flow.Id, ViewID: int64(flow.ViewId), Name: flow.Content.Name, Description: flow.Content.Description, Steps: flowStepsFromProto(flow.Content.Steps), Revision: flow.ExpectedRevision})
	if err != nil {
		return nil, flowStoreError("save flow", err)
	}
	saved, err := flowFromRecord(row)
	if err != nil {
		return nil, storeErr("read flow", err)
	}
	response := &diagv1.SaveFlowResponse{Flow: saved}
	action := "update"
	if flow.Id == "" {
		action = "create"
	}
	s.hooks().AfterWrite(ctx, workspaceID, action, "flow", saved.Id, map[string]any{"view_id": saved.ViewId}, response)
	return connect.NewResponse(response), nil
}

func (s *FlowService) DeleteFlow(ctx context.Context, req *connect.Request[diagv1.DeleteFlowRequest]) (*connect.Response[diagv1.DeleteFlowResponse], error) {
	ctx, workspaceID, err := flowRequestContext(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckWrite(ctx, workspaceID, "flows"); err != nil {
		return nil, err
	}
	if req.Msg.Id == "" || req.Msg.ViewId <= 0 || req.Msg.ExpectedRevision <= 0 {
		return nil, invalidArg("flow", "requires id, view and revision")
	}
	if _, err := s.Store.GetView(ctx, req.Msg.ViewId, workspaceID); err != nil {
		return nil, flowViewError("get view", err)
	}
	if err := s.Store.DeleteFlow(ctx, req.Msg.Id, int64(req.Msg.ViewId), req.Msg.ExpectedRevision); err != nil {
		return nil, flowStoreError("delete flow", err)
	}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "flow", req.Msg.Id, map[string]any{"view_id": req.Msg.ViewId}, nil)
	return connect.NewResponse(&diagv1.DeleteFlowResponse{}), nil
}
