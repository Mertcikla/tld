package api

import (
	"context"
	"encoding/json"
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
	maxFlowSteps       = 1000
	maxFlowPaths       = 26
	maxFlowBranchDepth = 8
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

func flowFromRecord(row app.FlowRecord) (*diagv1.Flow, error) {
	flow := &diagv1.Flow{}
	document, err := upgradeFlowDocument([]byte(row.Document))
	if err != nil {
		return nil, err
	}
	if err := protojson.Unmarshal(document, flow); err != nil {
		return nil, err
	}
	flow.Id, flow.ViewId, flow.Revision = row.ID, int32(row.ViewID), row.Revision
	return flow, nil
}

// Early flow documents used a single element/connector per step. Convert those
// references on read, without changing stored data or discarding unknown fields.
// The next explicit save persists the current representation.
func upgradeFlowDocument(document []byte) ([]byte, error) {
	var flow map[string]json.RawMessage
	if err := json.Unmarshal(document, &flow); err != nil {
		return nil, err
	}
	var steps []map[string]json.RawMessage
	if raw, ok := flow["steps"]; ok {
		if err := json.Unmarshal(raw, &steps); err != nil {
			return nil, err
		}
	} else {
		return document, nil
	}
	for _, step := range steps {
		delete(step, "kind")
		for _, field := range []struct{ old, current string }{{"elementId", "elementIds"}, {"element_id", "element_ids"}, {"connectorId", "connectors"}, {"connector_id", "connectors"}} {
			id, exists := step[field.old]
			if !exists {
				continue
			}
			var values []json.RawMessage
			if raw, ok := step[field.current]; ok {
				if err := json.Unmarshal(raw, &values); err != nil {
					return nil, err
				}
			}
			if string(id) != "0" && string(id) != "null" {
				if field.current == "connectors" {
					highlight := map[string]json.RawMessage{"id": id}
					if reverse, ok := step["reverse"]; ok {
						highlight["reverse"] = reverse
					}
					id, _ = json.Marshal(highlight)
				}
				values = append(values, id)
			}
			step[field.current], _ = json.Marshal(values)
			delete(step, field.old)
		}
		delete(step, "reverse")
	}
	flow["steps"], _ = json.Marshal(steps)
	return json.Marshal(flow)
}

func (s *FlowService) ListFlows(ctx context.Context, req *connect.Request[diagv1.ListFlowsRequest]) (*connect.Response[diagv1.ListFlowsResponse], error) {
	workspaceID, err := flowWorkspaceID(ctx, req.Msg.GetOrgId())
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
			return nil, storeErr("get view", err)
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

func validateFlow(flow *diagv1.Flow) error {
	if flow == nil || flow.ViewId <= 0 || strings.TrimSpace(flow.Name) == "" {
		return errors.New("flow requires a view and a name")
	}
	if len(flow.Name) > 200 || len(flow.Description) > 10000 {
		return errors.New("flow name or description is too long")
	}
	if (flow.Id == "" && flow.Revision != 0) || (flow.Id != "" && flow.Revision <= 0) {
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
			controls := 0
			if step.Overview {
				controls++
			}
			if step.GoToFlowId != "" {
				controls++
				if strings.TrimSpace(step.GoToFlowId) == "" || len(step.GoToFlowId) > 100 {
					return errors.New("invalid linked flow id")
				}
			}
			if branch := step.Branches; branch != nil {
				controls++
				if branch.Mode != "alternate" && branch.Mode != "parallel" {
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
			if controls > 1 {
				return errors.New("choose only one of overview, branches, or go to flow")
			}

			if len(step.Title) > 500 || len(step.Description) > 10000 {
				return errors.New("step title or description is too long")
			}
			if len(step.ElementIds) > 500 || len(step.Connectors) > 500 {
				return errors.New("step may highlight at most 500 elements and 500 connections")
			}
			elements := map[int32]bool{}
			for _, id := range step.ElementIds {
				if id <= 0 || elements[id] {
					return errors.New("element ids must be positive and unique within a step")
				}
				elements[id] = true
			}
			connectors := map[int32]bool{}
			for _, highlight := range step.Connectors {
				if highlight == nil || highlight.Id <= 0 || connectors[highlight.Id] {
					return errors.New("connection ids must be positive and unique within a step")
				}
				connectors[highlight.Id] = true
			}
			if len(step.Metadata) > 50 {
				return errors.New("step may contain at most 50 metadata entries")
			}
			for key, value := range step.Metadata {
				if strings.TrimSpace(key) == "" || len(key) > 100 || len(value) > 10000 {
					return errors.New("metadata requires nonempty keys up to 100 bytes and values up to 10000 bytes")
				}
			}
		}
		return nil
	}
	return validateSteps(flow.Steps, 0)
}

func flowStoreError(action string, err error) error {
	if errors.Is(err, app.ErrFlowConflict) {
		return connect.NewError(connect.CodeAborted, err)
	}
	return storeErr(action, err)
}

func (s *FlowService) SaveFlow(ctx context.Context, req *connect.Request[diagv1.SaveFlowRequest]) (*connect.Response[diagv1.SaveFlowResponse], error) {
	workspaceID, err := flowWorkspaceID(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckWrite(ctx, workspaceID, "flows"); err != nil {
		return nil, err
	}
	flow := req.Msg.Flow
	if err := validateFlow(flow); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if _, err := s.Store.GetView(ctx, flow.ViewId, workspaceID); err != nil {
		return nil, storeErr("get view", err)
	}
	links := map[string]bool{}
	var collectLinks func([]*diagv1.FlowStep)
	collectLinks = func(steps []*diagv1.FlowStep) {
		for _, step := range steps {
			if step.GoToFlowId != "" {
				links[step.GoToFlowId] = true
			}
			if step.Branches != nil {
				for _, path := range step.Branches.Paths {
					collectLinks(path.Steps)
				}
			}
		}
	}
	collectLinks(flow.Steps)
	if len(links) > 0 {
		rows, err := s.Store.ListFlows(ctx, 0)
		if err != nil {
			return nil, storeErr("list linked flows", err)
		}
		for _, row := range rows {
			if !links[row.ID] {
				continue
			}
			if _, err := s.Store.GetView(ctx, int32(row.ViewID), workspaceID); err != nil {
				return nil, storeErr("get linked view", err)
			}
			delete(links, row.ID)
		}
		if len(links) > 0 {
			return nil, invalidArg("go_to_flow_id", "linked flow is unavailable")
		}
	}

	document, err := protojson.Marshal(flow)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(document) > 1<<20 {
		return nil, invalidArg("flow", "must be smaller than 1 MiB")
	}
	row, err := s.Store.SaveFlow(ctx, app.FlowRecord{ID: flow.Id, ViewID: int64(flow.ViewId), Document: string(document), Revision: flow.Revision})
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
	workspaceID, err := flowWorkspaceID(ctx, req.Msg.GetOrgId())
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
		return nil, storeErr("get view", err)
	}
	if err := s.Store.DeleteFlow(ctx, req.Msg.Id, int64(req.Msg.ViewId), req.Msg.ExpectedRevision); err != nil {
		return nil, flowStoreError("delete flow", err)
	}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "flow", req.Msg.Id, map[string]any{"view_id": req.Msg.ViewId}, nil)
	return connect.NewResponse(&diagv1.DeleteFlowResponse{}), nil
}
