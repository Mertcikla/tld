package server

import (
	"context"
	"strconv"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/api"
)

type collaborationHooks struct {
	base  api.WorkspaceHooks
	store api.Store
	hub   *api.CollaborationHub
}

var _ api.WorkspaceHooks = (*collaborationHooks)(nil)

func (h collaborationHooks) baseHooks() api.WorkspaceHooks {
	if h.base == nil {
		return api.NopWorkspaceHooks{}
	}
	return h.base
}

func (h collaborationHooks) CheckRead(ctx context.Context, workspaceID uuid.UUID) error {
	return h.baseHooks().CheckRead(ctx, workspaceID)
}

func (h collaborationHooks) CheckWrite(ctx context.Context, workspaceID uuid.UUID, resourceType string) error {
	return h.baseHooks().CheckWrite(ctx, workspaceID, resourceType)
}

func (h collaborationHooks) CheckApplyPlan(ctx context.Context, workspaceID uuid.UUID, req *diagv1.ApplyPlanRequest) error {
	return h.baseHooks().CheckApplyPlan(ctx, workspaceID, req)
}

func (h collaborationHooks) AfterWrite(ctx context.Context, workspaceID uuid.UUID, action string, resourceType string, resourceID string, details map[string]any, response any) {
	h.baseHooks().AfterWrite(ctx, workspaceID, action, resourceType, resourceID, details, response)
	if h.hub == nil || h.store == nil {
		return
	}
	eventType := collaborationViewEventType(action, resourceType)
	if eventType == "" {
		return
	}
	viewIDs := h.affectedViewIDs(ctx, workspaceID, resourceType, resourceID, details, response)
	for _, viewID := range viewIDs {
		h.broadcastViewState(ctx, workspaceID, viewID, map[string]any{
			"type":        eventType,
			"resource_id": resourceID,
			"details":     details,
		})
	}
}

func (h collaborationHooks) AfterApplyPlan(ctx context.Context, workspaceID uuid.UUID, req *diagv1.ApplyPlanRequest, resp *diagv1.ApplyPlanResponse) {
	h.baseHooks().AfterApplyPlan(ctx, workspaceID, req, resp)
	if h.hub == nil || h.store == nil || resp == nil {
		return
	}
	seen := map[int32]struct{}{}
	for _, placement := range resp.GetCreatedPlacements() {
		addViewID(seen, placement.GetViewId())
	}
	for _, connector := range resp.GetCreatedConnectors() {
		addViewID(seen, connector.GetViewId())
	}
	for viewID := range seen {
		h.broadcastViewState(ctx, workspaceID, viewID, map[string]any{
			"type": "workspace_changed",
		})
	}
}

func (h collaborationHooks) broadcastViewState(ctx context.Context, workspaceID uuid.UUID, viewID int32, payload any) {
	h.hub.RefreshViewState(ctx, h.store, workspaceID, viewID)
	h.hub.BroadcastWorkspaceViewEvent(workspaceID, viewID, payload)
}

func collaborationViewEventType(action, resourceType string) string {
	switch resourceType {
	case "placement", "connector", "element", "view", "layer", "view_markdown":
		switch action {
		case "create", "update", "delete":
			return resourceType + "_" + action
		}
	}
	return ""
}

func (h collaborationHooks) affectedViewIDs(ctx context.Context, workspaceID uuid.UUID, resourceType, resourceID string, details map[string]any, response any) []int32 {
	seen := map[int32]struct{}{}
	if details != nil {
		addAnyViewID(seen, details["view_id"])
		addAnyViewIDs(seen, details["view_ids"])
	}
	addResponseViewID(seen, response)

	switch resourceType {
	case "view", "view_markdown":
		if id, ok := parseInt32(resourceID); ok {
			addViewID(seen, id)
		}
	case "element":
		if len(seen) == 0 {
			if id, ok := parseInt32(resourceID); ok {
				if placements, err := h.store.ListElementPlacements(ctx, id, workspaceID); err == nil {
					for _, placement := range placements {
						addViewID(seen, placement.GetViewId())
					}
				}
			}
		}
	case "connector":
		if len(seen) == 0 {
			if id, ok := parseInt32(resourceID); ok {
				if connector, err := h.store.GetConnector(ctx, id, workspaceID); err == nil {
					addViewID(seen, connector.GetViewId())
				}
			}
		}
	}

	return viewIDSetValues(seen)
}

func addResponseViewID(seen map[int32]struct{}, response any) {
	switch resp := response.(type) {
	case *diagv1.CreatePlacementResponse:
		if resp.GetPlacement() != nil {
			addViewID(seen, resp.GetPlacement().GetViewId())
		}
	case *diagv1.CreateConnectorResponse:
		if resp.GetConnector() != nil {
			addViewID(seen, resp.GetConnector().GetViewId())
		}
	case *diagv1.UpdateConnectorResponse:
		if resp.GetConnector() != nil {
			addViewID(seen, resp.GetConnector().GetViewId())
		}
	case *diagv1.CreateViewLayerResponse:
		if resp.GetLayer() != nil {
			addViewID(seen, resp.GetLayer().GetViewId())
		}
	case *diagv1.UpdateViewLayerResponse:
		if resp.GetLayer() != nil {
			addViewID(seen, resp.GetLayer().GetViewId())
		}
	}
}

func addAnyViewIDs(seen map[int32]struct{}, value any) {
	switch ids := value.(type) {
	case []int32:
		for _, id := range ids {
			addViewID(seen, id)
		}
	case []int:
		for _, id := range ids {
			addViewID(seen, int32(id))
		}
	case []int64:
		for _, id := range ids {
			addViewID(seen, int32(id))
		}
	case []any:
		for _, id := range ids {
			addAnyViewID(seen, id)
		}
	}
}

func addAnyViewID(seen map[int32]struct{}, value any) {
	if id, ok := anyToInt32(value); ok {
		addViewID(seen, id)
	}
}

func addViewID(seen map[int32]struct{}, id int32) {
	if id > 0 {
		seen[id] = struct{}{}
	}
}

func viewIDSetValues(seen map[int32]struct{}) []int32 {
	if len(seen) == 0 {
		return nil
	}
	out := make([]int32, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func parseInt32(value string) (int32, bool) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return int32(parsed), true
}

func anyToInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return int32(v), true
		}
	case int32:
		if v > 0 {
			return v, true
		}
	case int64:
		if v > 0 {
			return int32(v), true
		}
	case float64:
		if v > 0 {
			return int32(v), true
		}
	case string:
		return parseInt32(v)
	}
	return 0, false
}
