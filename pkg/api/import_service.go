package api

import (
	"context"
	"fmt"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/diag/tld/internal/importer"
)

// ImportService implements import-related RPCs.
type ImportService struct {
	Store Store
	Hooks WorkspaceHooks
}

func (s *ImportService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

// ImportResources delegates to ApplyPlan on the Store.
func (s *ImportService) ImportResources(ctx context.Context, req *connect.Request[diagv1.ImportResourcesRequest]) (*connect.Response[diagv1.ImportResourcesResponse], error) {
	m := req.Msg
	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}

	planReq := &diagv1.ApplyPlanRequest{
		OrgId:      m.GetOrgId(),
		Elements:   m.GetElements(),
		Connectors: m.GetConnectors(),
	}

	if err := s.hooks().CheckApplyPlan(ctx, workspaceID, planReq); err != nil {
		return nil, err
	}

	resp, err := s.Store.ApplyPlan(ctx, workspaceID, planReq)
	if err != nil {
		return nil, storeErr("apply plan", err)
	}

	s.hooks().AfterApplyPlan(ctx, workspaceID, planReq, resp)

	var viewID int32
	if len(resp.GetCreatedViews()) > 0 {
		viewID = resp.GetCreatedViews()[0].GetId()
	} else if len(resp.GetCreatedPlacements()) > 0 {
		viewID = resp.GetCreatedPlacements()[0].GetViewId()
	}

	return connect.NewResponse(&diagv1.ImportResourcesResponse{
		ViewId:  viewID,
		ViewUrl: fmt.Sprintf("/view/%d", viewID),
		Message: "Import successful",
	}), nil
}

// ParseStructurizr parses Structurizr DSL into plan elements and connectors.
func (s *ImportService) ParseStructurizr(ctx context.Context, req *connect.Request[diagv1.ParseStructurizrRequest]) (*connect.Response[diagv1.ParseStructurizrResponse], error) {
	parsed, err := importer.ParseStructurizr(req.Msg.GetCode())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	viewRef := "imported_view"

	elements := make([]*diagv1.PlanElement, 0, len(parsed.Elements))
	for _, e := range parsed.Elements {
		el := &diagv1.PlanElement{
			Ref:  e.ID,
			Name: e.Name,
			Placements: []*diagv1.PlanViewPlacement{
				{ParentRef: "root"},
			},
		}
		if e.Kind != "" {
			el.Kind = &e.Kind
		}
		if e.Description != "" {
			el.Description = &e.Description
		}
		if e.Technology != "" {
			el.Technology = &e.Technology
		}
		elements = append(elements, el)
	}

	connectors := make([]*diagv1.PlanConnector, 0, len(parsed.Connectors))
	for _, c := range parsed.Connectors {
		pc := &diagv1.PlanConnector{
			Ref:              c.SourceID + ":" + c.TargetID,
			ViewRef:          viewRef,
			SourceElementRef: c.SourceID,
			TargetElementRef: c.TargetID,
		}
		if c.Label != "" {
			pc.Label = &c.Label
		}
		if c.Technology != "" {
			pc.Relationship = &c.Technology
		}
		connectors = append(connectors, pc)
	}

	return connect.NewResponse(&diagv1.ParseStructurizrResponse{
		Elements:   elements,
		Connectors: connectors,
		Warnings:   parsed.Warnings,
	}), nil
}
