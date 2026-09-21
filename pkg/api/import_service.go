package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/importer"
	"github.com/mertcikla/tld/v2/internal/layout"
	"google.golang.org/protobuf/proto"
)

const rootViewRef = "root"

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

// ImportResources materializes imported elements/connectors through the same
// granular store mutations used by synchronous commands. Placements without
// coordinates are auto-laid out server-side.
func (s *ImportService) ImportResources(ctx context.Context, req *connect.Request[diagv1.ImportResourcesRequest]) (*connect.Response[diagv1.ImportResourcesResponse], error) {
	m := req.Msg
	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckWrite(ctx, workspaceID, "elements"); err != nil {
		return nil, err
	}

	elements := make([]*diagv1.PlanElement, 0, len(m.GetElements()))
	for _, element := range m.GetElements() {
		elements = append(elements, proto.Clone(element).(*diagv1.PlanElement))
	}
	for _, element := range elements {
		if element.BypassNoiseGate == nil {
			bypass := false
			element.BypassNoiseGate = &bypass
		}
	}

	viewID, err := s.importResourcesSafely(ctx, workspaceID, elements, m.GetConnectors())
	if err != nil {
		return nil, storeErr("import resources", err)
	}

	return connect.NewResponse(&diagv1.ImportResourcesResponse{
		ViewId:  viewID,
		ViewUrl: fmt.Sprintf("/view/%d", viewID),
		Message: "Import successful",
	}), nil
}

// pendingWrite defers a realtime hook until the import commits, so rolled-back
// work is never broadcast.
type pendingWrite struct {
	action   string
	resource string
	id       string
	attrs    map[string]any
	response any
}

func (s *ImportService) fireWrites(ctx context.Context, workspaceID uuid.UUID, writes []pendingWrite) {
	for _, w := range writes {
		s.hooks().AfterWrite(ctx, workspaceID, w.action, w.resource, w.id, w.attrs, w.response)
	}
}

// importResourcesSafely applies the import inside a store transaction when the
// store supports one, so a failure rolls back partial work. Stores that cannot
// run transactions fall back to direct mutations.
func (s *ImportService) importResourcesSafely(ctx context.Context, workspaceID uuid.UUID, elements []*diagv1.PlanElement, connectors []*diagv1.PlanConnector) (int32, error) {
	if transactional, ok := s.Store.(TransactionalStore); ok {
		var (
			viewID int32
			writes []pendingWrite
		)
		err := transactional.RunInTransaction(ctx, func(txCtx context.Context, txStore Store) error {
			txService := *s
			if txStore != nil {
				txService.Store = txStore
			}
			var err error
			viewID, writes, err = txService.importPlanResources(txCtx, workspaceID, elements, connectors)
			return err
		})
		switch {
		case err == nil:
			s.fireWrites(ctx, workspaceID, writes)
			return viewID, nil
		case errors.Is(err, ErrUnimplemented):
			// Store cannot run transactions; fall back to direct mutations.
		default:
			return 0, err
		}
	}
	viewID, writes, err := s.importPlanResources(ctx, workspaceID, elements, connectors)
	if err != nil {
		return 0, err
	}
	s.fireWrites(ctx, workspaceID, writes)
	return viewID, nil
}

// importPlanResources applies a declarative import using granular store
// mutations (elements, owned views, placements, connectors), mirroring the
// former ApplyPlan semantics but without the bulk plan operation. Realtime
// hooks are collected and fired by the caller after the import commits.
func (s *ImportService) importPlanResources(ctx context.Context, workspaceID uuid.UUID, elements []*diagv1.PlanElement, connectors []*diagv1.PlanConnector) (int32, []pendingWrite, error) {
	viewIDs := map[string]int32{}
	elementIDs := map[string]int32{}
	writes := make([]pendingWrite, 0, len(elements)+len(connectors))
	var firstViewID int32

	if planNeedsRootView(elements, connectors) {
		rootID, err := s.rootViewID(ctx, workspaceID)
		if err != nil {
			return 0, nil, err
		}
		viewIDs[rootViewRef] = rootID
	}

	for _, planned := range elements {
		if strings.TrimSpace(planned.GetRef()) == "" {
			return 0, nil, fmt.Errorf("plan element ref is required")
		}
		if strings.TrimSpace(planned.GetName()) == "" {
			return 0, nil, fmt.Errorf("plan element %q name is required", planned.GetRef())
		}

		input := ElementInput{
			Name:            planned.GetName(),
			Description:     planned.Description,
			Kind:            planned.Kind,
			Technology:      planned.Technology,
			URL:             planned.Url,
			LogoURL:         planned.LogoUrl,
			TechLinks:       planned.GetTechnologyLinks(),
			Tags:            cloneStringSlice(planned.GetTags()),
			Repo:            planned.Repo,
			Branch:          planned.Branch,
			Language:         planned.Language,
			FilePath:        planned.FilePath,
			BypassNoiseGate: planned.BypassNoiseGate,
			HasView:         planned.GetHasView(),
			ViewLabel:       planned.ViewLabel,
		}

		element, err := s.upsertImportedElement(ctx, workspaceID, planned, input, &writes)
		if err != nil {
			return 0, nil, err
		}
		elementIDs[planned.GetRef()] = element.GetId()

		if planned.GetHasView() {
			view, err := s.upsertImportedView(ctx, workspaceID, planned, element, &writes)
			if err != nil {
				return 0, nil, err
			}
			viewIDs[planned.GetRef()] = view.GetId()
			if firstViewID == 0 {
				firstViewID = view.GetId()
			}
		}
	}

	records, err := s.addImportedPlacements(ctx, workspaceID, elements, elementIDs, viewIDs, &writes)
	if err != nil {
		return 0, nil, err
	}
	var firstPlacementViewID int32
	if len(records) > 0 {
		firstPlacementViewID = records[0].viewID
	}

	if err := s.addImportedConnectors(ctx, workspaceID, connectors, elementIDs, viewIDs, &writes); err != nil {
		return 0, nil, err
	}

	if firstViewID != 0 {
		return firstViewID, writes, nil
	}
	return firstPlacementViewID, writes, nil
}

func (s *ImportService) upsertImportedElement(ctx context.Context, workspaceID uuid.UUID, planned *diagv1.PlanElement, input ElementInput, writes *[]pendingWrite) (*diagv1.Element, error) {
	action := "create"
	var (
		element *diagv1.Element
		err     error
	)
	if planned.GetId() != 0 {
		element, err = s.Store.UpdateElement(ctx, planned.GetId(), workspaceID, input)
		if errors.Is(err, sql.ErrNoRows) {
			element, err = s.Store.CreateElement(ctx, workspaceID, input)
		} else if err == nil {
			action = "update"
		}
	} else {
		element, err = s.Store.CreateElement(ctx, workspaceID, input)
	}
	if err != nil {
		return nil, err
	}
	*writes = append(*writes, pendingWrite{
		action: action, resource: "element", id: fmt.Sprintf("%d", element.GetId()),
		attrs:    map[string]any{"name": element.GetName()},
		response: elementResponse(action, element),
	})
	return element, nil
}

func (s *ImportService) upsertImportedView(ctx context.Context, workspaceID uuid.UUID, planned *diagv1.PlanElement, element *diagv1.Element, writes *[]pendingWrite) (*diagv1.View, error) {
	viewName := firstNonEmpty(planned.GetName(), element.GetName())
	action := "create"
	var (
		view *diagv1.View
		err  error
	)
	if planned.GetViewId() != 0 {
		view, err = s.Store.UpdateView(ctx, planned.GetViewId(), workspaceID, viewName, nil, planned.ViewLabel, nil)
		if errors.Is(err, sql.ErrNoRows) {
			ownerID := element.GetId()
			view, err = s.Store.CreateView(ctx, workspaceID, &ownerID, viewName, planned.ViewLabel, false)
		} else if err == nil {
			action = "update"
		}
	} else {
		ownerID := element.GetId()
		view, err = s.Store.CreateView(ctx, workspaceID, &ownerID, viewName, planned.ViewLabel, false)
	}
	if err != nil {
		return nil, err
	}
	*writes = append(*writes, pendingWrite{
		action: action, resource: "view", id: fmt.Sprintf("%d", view.GetId()),
		attrs:    map[string]any{"name": view.GetName()},
		response: viewResponse(action, view),
	})
	return view, nil
}

type importPlacementRecord struct {
	viewID    int32
	elementID int32
	item      *diagv1.PlacedElement
}

func (s *ImportService) addImportedPlacements(ctx context.Context, workspaceID uuid.UUID, elements []*diagv1.PlanElement, elementIDs, viewIDs map[string]int32, writes *[]pendingWrite) ([]importPlacementRecord, error) {
	var records []importPlacementRecord
	autoLayoutTargets := map[int32]map[int64]struct{}{}

	for _, planned := range elements {
		elementID := elementIDs[planned.GetRef()]
		if elementID == 0 {
			continue
		}
		for _, placement := range planned.GetPlacements() {
			parentRef := placement.GetParentRef()
			if parentRef == "" {
				parentRef = rootViewRef
			}
			viewID, ok := viewIDs[parentRef]
			if !ok {
				return nil, fmt.Errorf("unknown placement parent ref %q", parentRef)
			}
			item, err := s.Store.AddPlacement(ctx, viewID, elementID, placement.GetPositionX(), placement.GetPositionY())
			if err != nil {
				return nil, err
			}
			records = append(records, importPlacementRecord{viewID: viewID, elementID: elementID, item: item})
			if placement.PositionX == nil && placement.PositionY == nil {
				if autoLayoutTargets[viewID] == nil {
					autoLayoutTargets[viewID] = map[int64]struct{}{}
				}
				autoLayoutTargets[viewID][int64(elementID)] = struct{}{}
			}
		}
	}

	if err := s.autoLayoutImports(ctx, workspaceID, autoLayoutTargets, records); err != nil {
		return nil, err
	}
	for _, record := range records {
		*writes = append(*writes, pendingWrite{
			action: "create", resource: "placement", id: "",
			attrs:    map[string]any{"view_id": record.viewID, "element_id": record.elementID},
			response: &diagv1.CreatePlacementResponse{Placement: record.item},
		})
	}
	return records, nil
}

// autoLayoutImports positions freshly added placements that arrived without
// coordinates, anchoring them against the view's existing placements.
func (s *ImportService) autoLayoutImports(ctx context.Context, workspaceID uuid.UUID, targets map[int32]map[int64]struct{}, records []importPlacementRecord) error {
	for viewID, viewTargets := range targets {
		if len(viewTargets) == 0 {
			continue
		}
		placements, err := s.Store.ListPlacements(ctx, viewID)
		if err != nil {
			return err
		}
		connectors, err := s.Store.ListConnectors(ctx, viewID, workspaceID)
		if err != nil {
			return err
		}
		nodes := make([]layout.Placement, 0, len(placements))
		for _, placement := range placements {
			nodes = append(nodes, layout.Placement{
				ElementID: int64(placement.GetElementId()),
				X:         placement.GetPositionX(),
				Y:         placement.GetPositionY(),
			})
		}
		edges := make([]layout.Connector, 0, len(connectors))
		for _, connector := range connectors {
			edges = append(edges, layout.Connector{
				Source: int64(connector.GetSourceElementId()),
				Target: int64(connector.GetTargetElementId()),
			})
		}
		next := layout.LayoutPlacements(nodes, viewTargets, edges, false)
		for elementID, pos := range next {
			if err := s.Store.UpdatePlacementPosition(ctx, viewID, int32(elementID), pos.X, pos.Y); err != nil {
				return err
			}
			for i := range records {
				if records[i].viewID == viewID && records[i].elementID == int32(elementID) && records[i].item != nil {
					records[i].item.PositionX = pos.X
					records[i].item.PositionY = pos.Y
				}
			}
		}
	}
	return nil
}

func (s *ImportService) addImportedConnectors(ctx context.Context, workspaceID uuid.UUID, connectors []*diagv1.PlanConnector, elementIDs, viewIDs map[string]int32, writes *[]pendingWrite) error {
	for _, planned := range connectors {
		parentRef := planned.GetViewRef()
		if parentRef == "" {
			parentRef = rootViewRef
		}
		viewID, ok := viewIDs[parentRef]
		if !ok {
			return fmt.Errorf("unknown connector view ref %q", parentRef)
		}
		sourceID, ok := elementIDs[planned.GetSourceElementRef()]
		if !ok {
			return fmt.Errorf("unknown source element ref %q", planned.GetSourceElementRef())
		}
		targetID, ok := elementIDs[planned.GetTargetElementRef()]
		if !ok {
			return fmt.Errorf("unknown target element ref %q", planned.GetTargetElementRef())
		}

		input := ConnectorInput{
			ViewID:       viewID,
			SourceID:     sourceID,
			TargetID:     targetID,
			Label:        planned.Label,
			Description:  planned.Description,
			Relationship: planned.Relationship,
			Direction:    stringOrDefault(planned.Direction, "forward"),
			Style:        stringOrDefault(planned.Style, "bezier"),
			URL:          planned.Url,
			SourceHandle: planned.SourceHandle,
			TargetHandle: planned.TargetHandle,
		}

		action := "create"
		var (
			connector *diagv1.Connector
			err       error
		)
		if planned.GetId() != 0 {
			connector, err = s.Store.UpdateConnector(ctx, planned.GetId(), workspaceID, input)
			if errors.Is(err, sql.ErrNoRows) {
				connector, err = s.Store.CreateConnector(ctx, workspaceID, input)
			} else if err == nil {
				action = "update"
			}
		} else {
			connector, err = s.Store.CreateConnector(ctx, workspaceID, input)
		}
		if err != nil {
			return err
		}
		*writes = append(*writes, pendingWrite{
			action: action, resource: "connector", id: fmt.Sprintf("%d", connector.GetId()),
			attrs:    map[string]any{"view_id": viewID, "label": connector.GetLabel()},
			response: connectorResponse(action, connector),
		})
	}
	return nil
}

func (s *ImportService) rootViewID(ctx context.Context, workspaceID uuid.UUID) (int32, error) {
	isRoot := true
	views, _, err := s.Store.GetViews(ctx, workspaceID, nil, &isRoot, "", 1, 0)
	if err != nil {
		return 0, err
	}
	for _, view := range views {
		if view.ParentViewId == nil {
			return view.GetId(), nil
		}
	}
	return 0, fmt.Errorf("root view not found")
}

func planNeedsRootView(elements []*diagv1.PlanElement, connectors []*diagv1.PlanConnector) bool {
	for _, element := range elements {
		for _, placement := range element.GetPlacements() {
			if ref := placement.GetParentRef(); ref == "" || ref == rootViewRef {
				return true
			}
		}
	}
	for _, connector := range connectors {
		if ref := connector.GetViewRef(); ref == "" || ref == rootViewRef {
			return true
		}
	}
	return false
}

func stringOrDefault(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func elementResponse(action string, element *diagv1.Element) any {
	if action == "update" {
		return &diagv1.UpdateElementResponse{Element: element}
	}
	return &diagv1.CreateElementResponse{Element: element}
}

func viewResponse(action string, view *diagv1.View) any {
	if action == "update" {
		return &diagv1.UpdateViewResponse{View: view}
	}
	return &diagv1.CreateViewResponse{View: view}
}

func connectorResponse(action string, connector *diagv1.Connector) any {
	if action == "update" {
		return &diagv1.UpdateConnectorResponse{Connector: connector}
	}
	return &diagv1.CreateConnectorResponse{Connector: connector}
}

// ParseStructurizr parses Structurizr DSL into plan elements and connectors.
func (s *ImportService) ParseStructurizr(ctx context.Context, req *connect.Request[diagv1.ParseStructurizrRequest]) (*connect.Response[diagv1.ParseStructurizrResponse], error) {
	parsed, err := importer.ParseStructurizr(req.Msg.GetCode())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	viewRef := rootViewRef

	elements := make([]*diagv1.PlanElement, 0, len(parsed.Elements))
	for _, e := range parsed.Elements {
		el := &diagv1.PlanElement{
			Ref:  e.ID,
			Name: e.Name,
			Placements: []*diagv1.PlanViewPlacement{
				{ParentRef: rootViewRef},
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
