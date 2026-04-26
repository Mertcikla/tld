package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/internal/app"
	"github.com/mertcikla/tld/pkg/api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ api.Store = (*APIAdapter)(nil)

// APIAdapter exposes the local SQLite-backed store through the shared
// ConnectRPC-oriented api.Store contract.
type APIAdapter struct {
	Store *SQLiteStore
}

func NewAPIAdapter(store *SQLiteStore) *APIAdapter {
	return &APIAdapter{Store: store}
}

func (a *APIAdapter) ListViews(ctx context.Context, _ uuid.UUID) ([]*diagv1.View, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	flat := flattenViewTreeNodes(nodes)
	out := make([]*diagv1.View, 0, len(flat))
	for _, node := range flat {
		out = append(out, viewNodeToProto(node, api.WorkspaceIDFromCtx(ctx)))
	}
	return out, nil
}

func (a *APIAdapter) GetViews(ctx context.Context, _ uuid.UUID, ownerElementID *int32, isRoot *bool, search string, limit, offset int) ([]*diagv1.View, int, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, 0, err
	}
	flat := flattenViewTreeNodes(nodes)
	filtered := make([]app.ViewTreeNode, 0, len(flat))
	for _, node := range flat {
		if ownerElementID != nil {
			if node.OwnerElementID == nil || int32(*node.OwnerElementID) != *ownerElementID {
				continue
			}
		}
		if isRoot != nil {
			nodeIsRoot := node.ParentViewID == nil
			if nodeIsRoot != *isRoot {
				continue
			}
		}
		if search != "" && !containsFold(node.Name, search) {
			continue
		}
		filtered = append(filtered, node)
	}

	total := len(filtered)
	start := clampOffset(offset, total)
	end := total
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	filtered = filtered[start:end]

	out := make([]*diagv1.View, 0, len(filtered))
	workspaceID := api.WorkspaceIDFromCtx(ctx)
	for _, node := range filtered {
		out = append(out, viewNodeToProto(node, workspaceID))
	}
	return out, total, nil
}

func (a *APIAdapter) GetView(ctx context.Context, id int32, _ uuid.UUID) (*diagv1.View, error) {
	view, err := a.Store.legacy.ViewByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return viewNodeToProto(view, api.WorkspaceIDFromCtx(ctx)), nil
}

func (a *APIAdapter) CreateView(ctx context.Context, _ uuid.UUID, ownerElementID *int32, name string, label *string, _ bool) (*diagv1.View, error) {
	var ownerID *int64
	if ownerElementID != nil {
		v := int64(*ownerElementID)
		ownerID = &v
	}
	view, err := a.Store.legacy.CreateView(ctx, name, label, ownerID)
	if err != nil {
		return nil, err
	}
	return a.GetView(ctx, int32(view.ID), uuid.Nil)
}

func (a *APIAdapter) UpdateView(ctx context.Context, id int32, _ uuid.UUID, name string, label *string) (*diagv1.View, error) {
	nameCopy := name
	view, err := a.Store.legacy.UpdateView(ctx, int64(id), &nameCopy, label)
	if err != nil {
		return nil, err
	}
	return a.GetView(ctx, int32(view.ID), uuid.Nil)
}

func (a *APIAdapter) DeleteView(ctx context.Context, id int32, _ uuid.UUID) error {
	return a.Store.legacy.DeleteView(ctx, int64(id))
}

func (a *APIAdapter) ListElements(ctx context.Context, _ uuid.UUID, limit, offset int32, search string) ([]*diagv1.Element, error) {
	elements, err := a.Store.legacy.Elements(ctx, int(limit), int(offset), search)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.Element, 0, len(elements))
	workspaceID := api.WorkspaceIDFromCtx(ctx)
	for _, element := range elements {
		out = append(out, elementToProto(element, workspaceID))
	}
	return out, nil
}

func (a *APIAdapter) GetElement(ctx context.Context, id int32, _ uuid.UUID) (*diagv1.Element, error) {
	element, err := a.Store.legacy.ElementByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return elementToProto(element, api.WorkspaceIDFromCtx(ctx)), nil
}

func (a *APIAdapter) CreateElement(ctx context.Context, _ uuid.UUID, input api.ElementInput) (*diagv1.Element, error) {
	element, err := a.Store.legacy.CreateElement(ctx, app.LibraryElement{
		Name:                 input.Name,
		Description:          input.Description,
		Kind:                 input.Kind,
		Technology:           input.Technology,
		URL:                  input.URL,
		LogoURL:              input.LogoURL,
		TechnologyConnectors: technologyLinksFromProto(input.TechLinks),
		Tags:                 cloneStrings(input.Tags),
		Repo:                 input.Repo,
		Branch:               input.Branch,
		Language:             input.Language,
		FilePath:             input.FilePath,
		HasView:              input.HasView,
		ViewLabel:            input.ViewLabel,
	})
	if err != nil {
		return nil, err
	}
	return elementToProto(element, api.WorkspaceIDFromCtx(ctx)), nil
}

func (a *APIAdapter) UpdateElement(ctx context.Context, id int32, _ uuid.UUID, input api.ElementInput) (*diagv1.Element, error) {
	element, err := a.Store.legacy.UpdateElement(ctx, int64(id), app.LibraryElement{
		Name:                 input.Name,
		Description:          input.Description,
		Kind:                 input.Kind,
		Technology:           input.Technology,
		URL:                  input.URL,
		LogoURL:              input.LogoURL,
		TechnologyConnectors: technologyLinksFromProto(input.TechLinks),
		Tags:                 cloneStrings(input.Tags),
		Repo:                 input.Repo,
		Branch:               input.Branch,
		Language:             input.Language,
		FilePath:             input.FilePath,
		HasView:              input.HasView,
		ViewLabel:            input.ViewLabel,
	})
	if err != nil {
		return nil, err
	}
	return elementToProto(element, api.WorkspaceIDFromCtx(ctx)), nil
}

func (a *APIAdapter) DeleteElement(ctx context.Context, id int32, _ uuid.UUID) error {
	return a.Store.legacy.DeleteElement(ctx, int64(id))
}

func (a *APIAdapter) ListPlacements(ctx context.Context, viewID int32) ([]*diagv1.PlacedElement, error) {
	placements, err := a.Store.legacy.Placements(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.PlacedElement, 0, len(placements))
	for _, placement := range placements {
		out = append(out, placedElementToProto(placement))
	}
	return out, nil
}

func (a *APIAdapter) ListAllPlacements(ctx context.Context, _ uuid.UUID) ([]*diagv1.PlacedElement, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	var out []*diagv1.PlacedElement
	for _, node := range flattenViewTreeNodes(nodes) {
		items, err := a.ListPlacements(ctx, int32(node.ID))
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (a *APIAdapter) ListElementPlacements(ctx context.Context, elementID int32, _ uuid.UUID) ([]*diagv1.ViewPlacementInfo, error) {
	placements, err := a.Store.legacy.ListElementPlacements(ctx, int64(elementID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.ViewPlacementInfo, 0, len(placements))
	for _, placement := range placements {
		out = append(out, &diagv1.ViewPlacementInfo{
			ViewId:   int32(placement.ViewID),
			ViewName: placement.ViewName,
		})
	}
	return out, nil
}

func (a *APIAdapter) AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error) {
	placement, err := a.Store.legacy.AddPlacement(ctx, int64(viewID), int64(elementID), x, y)
	if err != nil {
		return nil, err
	}
	full, err := a.findPlacedElement(ctx, placement.ViewID, placement.ElementID)
	if err != nil {
		return nil, err
	}
	return full, nil
}

func (a *APIAdapter) UpdatePlacementPosition(ctx context.Context, viewID, elementID int32, x, y float64) error {
	return a.Store.legacy.UpdatePlacement(ctx, int64(viewID), int64(elementID), x, y)
}

func (a *APIAdapter) RemovePlacement(ctx context.Context, viewID, elementID int32) error {
	return a.Store.legacy.DeletePlacement(ctx, int64(viewID), int64(elementID))
}

func (a *APIAdapter) ListConnectors(ctx context.Context, viewID int32, _ uuid.UUID) ([]*diagv1.Connector, error) {
	connectors, err := a.Store.legacy.Connectors(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.Connector, 0, len(connectors))
	for _, connector := range connectors {
		out = append(out, connectorToProto(connector))
	}
	return out, nil
}

func (a *APIAdapter) ListAllConnectors(ctx context.Context, _ uuid.UUID) ([]*diagv1.Connector, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	var out []*diagv1.Connector
	for _, node := range flattenViewTreeNodes(nodes) {
		items, err := a.ListConnectors(ctx, int32(node.ID), uuid.Nil)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (a *APIAdapter) GetConnector(ctx context.Context, id int32, _ uuid.UUID) (*diagv1.Connector, error) {
	connector, err := a.Store.legacy.ConnectorByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIAdapter) CreateConnector(ctx context.Context, _ uuid.UUID, input api.ConnectorInput) (*diagv1.Connector, error) {
	connector, err := a.Store.legacy.CreateConnector(ctx, app.Connector{
		ViewID:          int64(input.ViewID),
		SourceElementID: int64(input.SourceID),
		TargetElementID: int64(input.TargetID),
		Label:           input.Label,
		Description:     input.Description,
		Relationship:    input.Relationship,
		Direction:       input.Direction,
		Style:           input.Style,
		URL:             input.URL,
		SourceHandle:    input.SourceHandle,
		TargetHandle:    input.TargetHandle,
	})
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIAdapter) UpdateConnector(ctx context.Context, id int32, _ uuid.UUID, input api.ConnectorInput) (*diagv1.Connector, error) {
	connector, err := a.Store.legacy.UpdateConnector(ctx, int64(id), app.Connector{
		ID:              int64(id),
		ViewID:          int64(input.ViewID),
		SourceElementID: int64(input.SourceID),
		TargetElementID: int64(input.TargetID),
		Label:           input.Label,
		Description:     input.Description,
		Relationship:    input.Relationship,
		Direction:       input.Direction,
		Style:           input.Style,
		URL:             input.URL,
		SourceHandle:    input.SourceHandle,
		TargetHandle:    input.TargetHandle,
	})
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIAdapter) DeleteConnector(ctx context.Context, id int32, _ uuid.UUID) error {
	return a.Store.legacy.DeleteConnector(ctx, int64(id))
}

func (a *APIAdapter) ListElementNavigations(ctx context.Context, _ uuid.UUID, elementID int32) ([]*diagv1.ElementNavigationInfo, error) {
	navs, err := a.Store.legacy.ListElementNavigations(ctx, int64(elementID), nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.ElementNavigationInfo, 0, len(navs))
	for _, nav := range navs {
		out = append(out, navigationToProto(nav))
	}
	return out, nil
}

func (a *APIAdapter) ListIncomingElementNavigations(ctx context.Context, viewID int32) ([]*diagv1.IncomingElementNavigationInfo, error) {
	navs, err := a.Store.legacy.ListIncomingNavigations(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.IncomingElementNavigationInfo, 0, len(navs))
	for _, nav := range navs {
		out = append(out, &diagv1.IncomingElementNavigationInfo{
			Id:           int32(nav.ID),
			ElementId:    int32(nav.ElementID),
			ElementName:  nav.ElementName,
			FromViewId:   int32(nav.FromViewID),
			FromViewName: nav.FromViewName,
			ToViewId:     int32(nav.ToViewID),
		})
	}
	return out, nil
}

func (a *APIAdapter) ListViewLayers(ctx context.Context, viewID int32) ([]*diagv1.ViewLayer, error) {
	layers, err := a.Store.legacy.Layers(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.ViewLayer, 0, len(layers))
	for _, layer := range layers {
		out = append(out, layerToProto(layer))
	}
	return out, nil
}

func (a *APIAdapter) ListAllViewLayers(ctx context.Context, _ uuid.UUID) ([]*diagv1.ViewLayer, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	var out []*diagv1.ViewLayer
	for _, node := range flattenViewTreeNodes(nodes) {
		items, err := a.ListViewLayers(ctx, int32(node.ID))
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (a *APIAdapter) GetViewLayer(ctx context.Context, id int32) (*diagv1.ViewLayer, error) {
	layer, err := a.Store.legacy.LayerByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return layerToProto(layer), nil
}

func (a *APIAdapter) CreateViewLayer(ctx context.Context, viewID int32, name string, tags []string, color string) (*diagv1.ViewLayer, error) {
	layer, err := a.Store.legacy.CreateLayer(ctx, int64(viewID), name, cloneStrings(tags), &color)
	if err != nil {
		return nil, err
	}
	return layerToProto(layer), nil
}

func (a *APIAdapter) UpdateViewLayer(ctx context.Context, id int32, name *string, tags []string, color *string) (*diagv1.ViewLayer, error) {
	layer, err := a.Store.legacy.UpdateLayer(ctx, int64(id), app.ViewLayer{
		ID:    int64(id),
		Name:  derefString(name),
		Tags:  cloneStrings(tags),
		Color: color,
	})
	if err != nil {
		return nil, err
	}
	return layerToProto(layer), nil
}

func (a *APIAdapter) DeleteViewLayer(ctx context.Context, id int32) error {
	return a.Store.legacy.DeleteLayer(ctx, int64(id))
}

func (a *APIAdapter) ApplyPlan(ctx context.Context, _ uuid.UUID, req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
	if req.GetDryRun() {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("dry_run is not supported by the local sqlite adapter"))
	}

	rootViewID, err := a.ensureRootViewID(ctx)
	if err != nil {
		return nil, err
	}

	resp := &diagv1.ApplyPlanResponse{
		Summary: &diagv1.PlanSummary{
			ElementsPlanned:   int32(len(req.GetElements())),
			ViewsPlanned:      countPlannedViews(req.GetElements()),
			ConnectorsPlanned: int32(len(req.GetConnectors())),
		},
		ElementMetadata:   map[string]*diagv1.ResourceMetadata{},
		ViewMetadata:      map[string]*diagv1.ResourceMetadata{},
		ConnectorMetadata: map[string]*diagv1.ResourceMetadata{},
	}

	elementIDs := make(map[string]int32, len(req.GetElements()))
	viewIDs := map[string]int32{"root": rootViewID}

	for _, planned := range req.GetElements() {
		if planned.GetRef() == "" {
			return nil, fmt.Errorf("plan element ref is required")
		}

		input := api.ElementInput{
			Name:        planned.GetName(),
			Description: planned.Description,
			Kind:        planned.Kind,
			Technology:  planned.Technology,
			URL:         planned.Url,
			LogoURL:     planned.LogoUrl,
			TechLinks:   cloneTechLinks(planned.GetTechnologyLinks()),
			Tags:        cloneStrings(planned.GetTags()),
			Repo:        planned.Repo,
			Branch:      planned.Branch,
			Language:    planned.Language,
			FilePath:    planned.FilePath,
			HasView:     planned.GetHasView(),
			ViewLabel:   planned.ViewLabel,
		}

		var element *diagv1.Element
		if planned.GetId() != 0 {
			element, err = a.UpdateElement(ctx, planned.GetId(), uuid.Nil, input)
		} else {
			element, err = a.CreateElement(ctx, uuid.Nil, input)
		}
		if err != nil {
			return nil, err
		}

		elementIDs[planned.GetRef()] = element.GetId()
		resp.CreatedElements = append(resp.CreatedElements, element)
		resp.Summary.ElementsCreated++
		resp.ElementMetadata[planned.GetRef()] = &diagv1.ResourceMetadata{
			Id:        element.GetId(),
			UpdatedAt: element.UpdatedAt,
		}
		resp.ElementResults = append(resp.ElementResults, &diagv1.ApplyPlanElementResult{
			Ref:          planned.GetRef(),
			CanonicalRef: planned.GetRef(),
			Id:           element.GetId(),
			UpdatedAt:    element.UpdatedAt,
		})

		if planned.GetHasView() {
			viewName := planned.GetName()
			if viewName == "" {
				viewName = element.GetName()
			}
			var view *diagv1.View
			if planned.GetViewId() != 0 {
				view, err = a.UpdateView(ctx, planned.GetViewId(), uuid.Nil, viewName, planned.ViewLabel)
			} else {
				ownerID := element.GetId()
				view, err = a.CreateView(ctx, uuid.Nil, &ownerID, viewName, planned.ViewLabel, false)
			}
			if err != nil {
				return nil, err
			}
			viewIDs[planned.GetRef()] = view.GetId()
			resp.CreatedViews = append(resp.CreatedViews, &diagv1.ViewSummary{
				Id:             view.GetId(),
				OrgId:          view.GetOrgId(),
				OwnerElementId: view.OwnerElementId,
				Name:           view.GetName(),
				Label:          view.LevelLabel,
				IsRoot:         view.ParentViewId == nil,
				CreatedAt:      view.CreatedAt,
				UpdatedAt:      view.UpdatedAt,
			})
			resp.Summary.ViewsCreated++
			resp.ViewMetadata[planned.GetRef()] = &diagv1.ResourceMetadata{
				Id:        view.GetId(),
				UpdatedAt: view.UpdatedAt,
			}
		}
	}

	for _, planned := range req.GetElements() {
		elementID := elementIDs[planned.GetRef()]
		for _, placement := range planned.GetPlacements() {
			parentRef := placement.GetParentRef()
			if parentRef == "" {
				parentRef = "root"
			}
			viewID, ok := viewIDs[parentRef]
			if !ok {
				return nil, fmt.Errorf("unknown placement parent ref %q", parentRef)
			}
			item, err := a.AddPlacement(ctx, viewID, elementID, placement.GetPositionX(), placement.GetPositionY())
			if err != nil {
				return nil, err
			}
			resp.CreatedPlacements = append(resp.CreatedPlacements, &diagv1.ElementPlacement{
				Id:        item.GetId(),
				ViewId:    item.GetViewId(),
				ElementId: item.GetElementId(),
				PositionX: item.GetPositionX(),
				PositionY: item.GetPositionY(),
			})
		}
	}

	for _, planned := range req.GetConnectors() {
		parentRef := planned.GetViewRef()
		if parentRef == "" {
			parentRef = "root"
		}
		viewID, ok := viewIDs[parentRef]
		if !ok {
			return nil, fmt.Errorf("unknown connector view ref %q", parentRef)
		}
		sourceID, ok := elementIDs[planned.GetSourceElementRef()]
		if !ok {
			return nil, fmt.Errorf("unknown source element ref %q", planned.GetSourceElementRef())
		}
		targetID, ok := elementIDs[planned.GetTargetElementRef()]
		if !ok {
			return nil, fmt.Errorf("unknown target element ref %q", planned.GetTargetElementRef())
		}

		input := api.ConnectorInput{
			ViewID:       viewID,
			SourceID:     sourceID,
			TargetID:     targetID,
			Label:        planned.Label,
			Description:  planned.Description,
			Relationship: planned.Relationship,
			Direction:    derefStringDefault(planned.Direction, "forward"),
			Style:        derefStringDefault(planned.Style, "bezier"),
			URL:          planned.Url,
			SourceHandle: planned.SourceHandle,
			TargetHandle: planned.TargetHandle,
		}

		var connector *diagv1.Connector
		if planned.GetId() != 0 {
			connector, err = a.UpdateConnector(ctx, planned.GetId(), uuid.Nil, input)
		} else {
			connector, err = a.CreateConnector(ctx, uuid.Nil, input)
		}
		if err != nil {
			return nil, err
		}

		resp.CreatedConnectors = append(resp.CreatedConnectors, connector)
		resp.Summary.ConnectorsCreated++
		if planned.GetRef() != "" {
			resp.ConnectorMetadata[planned.GetRef()] = &diagv1.ResourceMetadata{
				Id:        connector.GetId(),
				UpdatedAt: connector.UpdatedAt,
			}
			resp.ConnectorResults = append(resp.ConnectorResults, &diagv1.ApplyPlanConnectorResult{
				Ref:          planned.GetRef(),
				CanonicalRef: planned.GetRef(),
				Id:           connector.GetId(),
				UpdatedAt:    connector.UpdatedAt,
			})
		}
	}

	return resp, nil
}

func (a *APIAdapter) ListVersions(context.Context, uuid.UUID, int) ([]*diagv1.WorkspaceVersionInfo, error) {
	return nil, api.ErrUnimplemented
}

func (a *APIAdapter) GetLatestVersion(context.Context, uuid.UUID) (*diagv1.WorkspaceVersionInfo, error) {
	return nil, api.ErrUnimplemented
}

func (a *APIAdapter) CreateVersion(context.Context, uuid.UUID, string, string, *int32, int, int, int, *string, *string) (*diagv1.WorkspaceVersionInfo, error) {
	return nil, api.ErrUnimplemented
}

func (a *APIAdapter) GetVersioningEnabled(context.Context, uuid.UUID) (bool, error) {
	return false, api.ErrUnimplemented
}

func (a *APIAdapter) SetVersioningEnabled(context.Context, uuid.UUID, bool) error {
	return api.ErrUnimplemented
}

func (a *APIAdapter) GetWorkspaceResourceCounts(ctx context.Context, _ uuid.UUID) (views, elements, connectors int, err error) {
	allViews, err := a.Store.ViewTree(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	allElements, err := a.Store.legacy.Elements(ctx, 0, 0, "")
	if err != nil {
		return 0, 0, 0, err
	}
	allConnectors, err := a.ListAllConnectors(ctx, uuid.Nil)
	if err != nil {
		return 0, 0, 0, err
	}
	return len(flattenViewTreeNodes(allViews)), len(allElements), len(allConnectors), nil
}

func (a *APIAdapter) ensureRootViewID(ctx context.Context) (int32, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return 0, err
	}
	for _, node := range flattenViewTreeNodes(nodes) {
		if node.ParentViewID == nil {
			return int32(node.ID), nil
		}
	}
	return 0, fmt.Errorf("root view not found")
}

func (a *APIAdapter) findPlacedElement(ctx context.Context, viewID, elementID int64) (*diagv1.PlacedElement, error) {
	items, err := a.Store.legacy.Placements(ctx, viewID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ViewID == viewID && item.ElementID == elementID {
			return placedElementToProto(item), nil
		}
	}
	return nil, fmt.Errorf("placement %d/%d not found", viewID, elementID)
}

func flattenViewTreeNodes(nodes []app.ViewTreeNode) []app.ViewTreeNode {
	var out []app.ViewTreeNode
	var walk func([]app.ViewTreeNode)
	walk = func(items []app.ViewTreeNode) {
		for _, item := range items {
			children := item.Children
			item.Children = nil
			out = append(out, item)
			walk(children)
		}
	}
	walk(nodes)
	return out
}

func viewNodeToProto(node app.ViewTreeNode, workspaceID uuid.UUID) *diagv1.View {
	p := &diagv1.View{
		Id:        int32(node.ID),
		OrgId:     workspaceID.String(),
		Name:      node.Name,
		Level:     int32(node.Level),
		Depth:     int32(node.Depth),
		CreatedAt: ts(node.CreatedAt),
		UpdatedAt: ts(node.UpdatedAt),
	}
	if node.Description != nil {
		p.Description = node.Description
	}
	if node.LevelLabel != nil {
		p.LevelLabel = node.LevelLabel
	}
	if node.ParentViewID != nil {
		parentID := int32(*node.ParentViewID)
		p.ParentViewId = &parentID
	}
	if node.OwnerElementID != nil {
		ownerID := int32(*node.OwnerElementID)
		p.OwnerElementId = &ownerID
	}
	return p
}

func elementToProto(element app.LibraryElement, workspaceID uuid.UUID) *diagv1.Element {
	p := &diagv1.Element{
		Id:        int32(element.ID),
		OrgId:     workspaceID.String(),
		Name:      element.Name,
		Kind:      element.Kind,
		Tags:      cloneStrings(element.Tags),
		CreatedAt: ts(element.CreatedAt),
		UpdatedAt: ts(element.UpdatedAt),
		HasView:   element.HasView,
		ViewLabel: element.ViewLabel,
	}
	if element.Description != nil {
		p.Description = element.Description
	}
	if element.Technology != nil {
		p.Technology = element.Technology
	}
	if element.URL != nil {
		p.Url = element.URL
	}
	if element.LogoURL != nil {
		p.LogoUrl = element.LogoURL
	}
	if element.Repo != nil {
		p.Repo = element.Repo
	}
	if element.Branch != nil {
		p.Branch = element.Branch
	}
	if element.Language != nil {
		p.Language = element.Language
	}
	if element.FilePath != nil {
		p.FilePath = element.FilePath
	}
	for _, link := range element.TechnologyConnectors {
		item := &diagv1.TechnologyLink{
			Type:          link.Type,
			Label:         link.Label,
			IsPrimaryIcon: link.IsPrimaryIcon,
		}
		if link.Slug != "" {
			slug := link.Slug
			item.Slug = &slug
		}
		p.TechnologyLinks = append(p.TechnologyLinks, item)
	}
	return p
}

func placedElementToProto(item app.PlacedElement) *diagv1.PlacedElement {
	p := &diagv1.PlacedElement{
		Id:        int32(item.ID),
		ViewId:    int32(item.ViewID),
		ElementId: int32(item.ElementID),
		PositionX: item.PositionX,
		PositionY: item.PositionY,
		Name:      item.Name,
		Kind:      item.Kind,
		Tags:      cloneStrings(item.Tags),
	}
	if item.Description != nil {
		p.Description = item.Description
	}
	if item.Technology != nil {
		p.Technology = item.Technology
	}
	if item.URL != nil {
		p.Url = item.URL
	}
	if item.LogoURL != nil {
		p.LogoUrl = item.LogoURL
	}
	if item.Repo != nil {
		p.Repo = item.Repo
	}
	if item.Branch != nil {
		p.Branch = item.Branch
	}
	if item.Language != nil {
		p.Language = item.Language
	}
	if item.FilePath != nil {
		p.FilePath = item.FilePath
	}
	for _, link := range item.TechnologyConnectors {
		entry := &diagv1.TechnologyLink{
			Type:          link.Type,
			Label:         link.Label,
			IsPrimaryIcon: link.IsPrimaryIcon,
		}
		if link.Slug != "" {
			slug := link.Slug
			entry.Slug = &slug
		}
		p.TechnologyLinks = append(p.TechnologyLinks, entry)
	}
	return p
}

func connectorToProto(connector app.Connector) *diagv1.Connector {
	return &diagv1.Connector{
		Id:              int32(connector.ID),
		ViewId:          int32(connector.ViewID),
		SourceElementId: int32(connector.SourceElementID),
		TargetElementId: int32(connector.TargetElementID),
		Label:           connector.Label,
		Description:     connector.Description,
		Relationship:    connector.Relationship,
		Direction:       connector.Direction,
		Style:           connector.Style,
		Url:             connector.URL,
		SourceHandle:    connector.SourceHandle,
		TargetHandle:    connector.TargetHandle,
		CreatedAt:       ts(connector.CreatedAt),
		UpdatedAt:       ts(connector.UpdatedAt),
	}
}

func navigationToProto(nav app.ViewConnector) *diagv1.ElementNavigationInfo {
	out := &diagv1.ElementNavigationInfo{
		Id:           int32(nav.ID),
		FromViewId:   int32(nav.FromViewID),
		ToViewId:     int32(nav.ToViewID),
		ToViewName:   nav.ToViewName,
		RelationType: nav.RelationType,
	}
	if nav.ElementID != nil {
		elementID := int32(*nav.ElementID)
		out.ElementId = &elementID
	}
	return out
}

func layerToProto(layer app.ViewLayer) *diagv1.ViewLayer {
	color := ""
	if layer.Color != nil {
		color = *layer.Color
	}
	return &diagv1.ViewLayer{
		Id:        int32(layer.ID),
		ViewId:    int32(layer.DiagramID),
		Name:      layer.Name,
		Tags:      cloneStrings(layer.Tags),
		Color:     color,
		CreatedAt: ts(layer.CreatedAt),
		UpdatedAt: ts(layer.UpdatedAt),
	}
}

func technologyLinksFromProto(links []*diagv1.TechnologyLink) []app.TechnologyConnector {
	if links == nil {
		return nil
	}
	if len(links) == 0 {
		return []app.TechnologyConnector{}
	}
	out := make([]app.TechnologyConnector, 0, len(links))
	for _, link := range links {
		out = append(out, app.TechnologyConnector{
			Type:          link.GetType(),
			Slug:          link.GetSlug(),
			Label:         link.GetLabel(),
			IsPrimaryIcon: link.GetIsPrimaryIcon(),
		})
	}
	return out
}

func cloneTechLinks(links []*diagv1.TechnologyLink) []*diagv1.TechnologyLink {
	if len(links) == 0 {
		return nil
	}
	out := make([]*diagv1.TechnologyLink, 0, len(links))
	for _, link := range links {
		if link == nil {
			continue
		}
		item := &diagv1.TechnologyLink{
			Type:          link.GetType(),
			Label:         link.GetLabel(),
			IsPrimaryIcon: link.GetIsPrimaryIcon(),
		}
		if link.Slug != nil {
			slug := link.GetSlug()
			item.Slug = &slug
		}
		out = append(out, item)
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func countPlannedViews(elements []*diagv1.PlanElement) int32 {
	var count int32
	for _, element := range elements {
		if element.GetHasView() {
			count++
		}
	}
	return count
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefStringDefault(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func containsFold(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && stringContainsFold(s, substr))
}

func stringContainsFold(s, substr string) bool {
	return len(substr) == 0 || (len(s) > 0 && (indexFold(s, substr) >= 0))
}

func indexFold(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa := a[i]
		bb := b[i]
		if 'A' <= aa && aa <= 'Z' {
			aa += 'a' - 'A'
		}
		if 'A' <= bb && bb <= 'Z' {
			bb += 'a' - 'A'
		}
		if aa != bb {
			return false
		}
	}
	return true
}

func clampOffset(offset, total int) int {
	if offset <= 0 {
		return 0
	}
	if offset > total {
		return total
	}
	return offset
}

func ts(value string) *timestamppb.Timestamp {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return timestamppb.New(t)
}
