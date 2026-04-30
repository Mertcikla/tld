package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

var _ diagv1connect.WorkspaceServiceHandler = (*WorkspaceService)(nil)

// WorkspaceService implements the core workspace ConnectRPC service without any
// auth, billing, or caching concerns.  Both the cloud backend and the offline tld
// app embed or wrap this service and add their own middleware/interceptors.
type WorkspaceService struct {
	Store Store
	Hooks WorkspaceHooks
	diagv1connect.UnimplementedWorkspaceServiceHandler
}

func (s *WorkspaceService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

func intToInt32(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	default:
		return int32(n) //nolint:gosec // clamped above
	}
}

// ─── CLI RPCs ─────────────────────────────────────────────────────────────────

func (s *WorkspaceService) CreateView(
	ctx context.Context,
	req *connect.Request[diagv1.CreateViewRequest],
) (*connect.Response[diagv1.CreateViewResponse], error) {
	m := req.Msg

	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.GetName()) == "" {
		return nil, invalidArg("name", "must not be empty")
	}

	var ownerElementID *int32
	if m.GetOwnerElementId() != 0 {
		pid, err := parseRequiredInt32("owner_element_id", m.GetOwnerElementId())
		if err != nil {
			return nil, err
		}
		ownerElementID = &pid
	}

	label := OptStr(m.GetLevelLabel())

	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}

	v, err := s.Store.CreateView(ctx, workspaceID, ownerElementID, strings.TrimSpace(m.GetName()), label, false)
	if err != nil {
		if strings.Contains(err.Error(), "views_owner_or_root") {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("an owner_element_id is required because a root view already exists"))
		}
		return nil, storeErr("create view", err)
	}

	resp := &diagv1.CreateViewResponse{View: v}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "view", strconv.Itoa(int(v.Id)), map[string]any{"name": v.Name}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) DeleteView(
	ctx context.Context,
	req *connect.Request[diagv1.DeleteViewRequest],
) (*connect.Response[diagv1.DeleteViewResponse], error) {
	m := req.Msg

	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}

	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}

	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	if err := s.Store.DeleteView(ctx, viewID, workspaceID); err != nil {
		return nil, storeErr("delete view", err)
	}

	resp := &diagv1.DeleteViewResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "view", strconv.Itoa(int(viewID)), nil, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) DeleteElement(
	ctx context.Context,
	req *connect.Request[diagv1.DeleteElementRequest],
) (*connect.Response[diagv1.DeleteElementResponse], error) {
	m := req.Msg

	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	elementID, err := parseRequiredInt32("element_id", m.GetElementId())
	if err != nil {
		return nil, err
	}

	if err := s.hooks().CheckWrite(ctx, workspaceID, "elements"); err != nil {
		return nil, err
	}

	if _, err := s.Store.GetElement(ctx, elementID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("element not found"))
	}
	if err := s.Store.DeleteElement(ctx, elementID, workspaceID); err != nil {
		return nil, storeErr("delete element", err)
	}

	resp := &diagv1.DeleteElementResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "element", strconv.Itoa(int(elementID)), nil, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) DeleteConnector(
	ctx context.Context,
	req *connect.Request[diagv1.DeleteConnectorRequest],
) (*connect.Response[diagv1.DeleteConnectorResponse], error) {
	m := req.Msg

	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	connectorID, err := parseRequiredInt32("connector_id", m.GetConnectorId())
	if err != nil {
		return nil, err
	}

	if err := s.hooks().CheckWrite(ctx, workspaceID, "connectors"); err != nil {
		return nil, err
	}

	if _, err := s.Store.GetConnector(ctx, connectorID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("connector not found"))
	}
	if err := s.Store.DeleteConnector(ctx, connectorID, workspaceID); err != nil {
		return nil, storeErr("delete connector", err)
	}

	resp := &diagv1.DeleteConnectorResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "connector", strconv.Itoa(int(connectorID)), nil, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) DeleteElementNavigation(
	ctx context.Context,
	_ *connect.Request[diagv1.DeleteElementNavigationRequest],
) (*connect.Response[diagv1.DeleteElementNavigationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("manual navigation deletion is no longer supported"))
}

func (s *WorkspaceService) ApplyWorkspacePlan(
	ctx context.Context,
	req *connect.Request[diagv1.ApplyPlanRequest],
) (*connect.Response[diagv1.ApplyPlanResponse], error) {
	m := req.Msg

	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}

	// Validate elements
	for _, pe := range m.GetElements() {
		if strings.TrimSpace(pe.GetRef()) == "" {
			return nil, invalidArg("elements[].ref", "must not be empty")
		}
		if strings.TrimSpace(pe.GetName()) == "" {
			return nil, invalidArg("elements[].name", "must not be empty")
		}
		if _, err := ConvertTechnologyLinks(pe.GetTechnologyLinks()); err != nil {
			return nil, err
		}
	}
	for i, pc := range m.GetConnectors() {
		if pc.GetViewRef() == "" {
			return nil, invalidArgF(fmt.Sprintf("connectors[%d].view_ref", i), "must not be empty")
		}
		if pc.GetSourceElementRef() == "" {
			return nil, invalidArgF(fmt.Sprintf("connectors[%d].source_element_ref", i), "must not be empty")
		}
		if pc.GetTargetElementRef() == "" {
			return nil, invalidArgF(fmt.Sprintf("connectors[%d].target_element_ref", i), "must not be empty")
		}
		dir := pc.GetDirection()
		if dir == "" {
			dir = "forward"
		}
		if err := validateDirection(dir); err != nil {
			return nil, err
		}
		style := pc.GetStyle()
		if style == "" {
			style = "bezier"
		}
		if err := validateEdgeType(style); err != nil {
			return nil, err
		}
	}

	if err := s.hooks().CheckApplyPlan(ctx, workspaceID, m); err != nil {
		return nil, err
	}

	resp, err := s.Store.ApplyPlan(ctx, workspaceID, req.Msg)
	if err != nil {
		return nil, storeErr("apply plan", err)
	}

	s.hooks().AfterApplyPlan(ctx, workspaceID, m, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) ExportWorkspace(
	ctx context.Context,
	req *connect.Request[diagv1.ExportOrganizationRequest],
) (*connect.Response[diagv1.ExportOrganizationResponse], error) {
	workspaceID, err := ResolveWorkspaceID(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}

	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}

	var (
		views      []*diagv1.View
		elements   []*diagv1.Element
		placements []*diagv1.PlacedElement
		connectors []*diagv1.Connector
		layers     []*diagv1.ViewLayer
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { var e error; views, e = s.Store.ListViews(gctx, workspaceID); return e })
	g.Go(func() error {
		var e error
		elements, _, e = s.Store.ListElements(gctx, workspaceID, 0, 0, "")
		return e
	})
	g.Go(func() error { var e error; placements, e = s.Store.ListAllPlacements(gctx, workspaceID); return e })
	g.Go(func() error { var e error; connectors, e = s.Store.ListAllConnectors(gctx, workspaceID); return e })
	g.Go(func() error { var e error; layers, e = s.Store.ListAllViewLayers(gctx, workspaceID); return e })

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("export workspace: %w", err))
	}

	// Convert PlacedElement → ElementPlacement for export format
	exportPlacements := make([]*diagv1.ElementPlacement, 0, len(placements))
	for _, p := range placements {
		exportPlacements = append(exportPlacements, &diagv1.ElementPlacement{
			Id:        p.Id,
			ViewId:    p.ViewId,
			ElementId: p.ElementId,
			PositionX: p.PositionX,
			PositionY: p.PositionY,
		})
	}

	return connect.NewResponse(&diagv1.ExportOrganizationResponse{
		Views:      views,
		Elements:   elements,
		Placements: exportPlacements,
		Connectors: connectors,
		Layers:     layers,
	}), nil
}

// ─── Browser RPCs ─────────────────────────────────────────────────────────────

func (s *WorkspaceService) ListViews(
	ctx context.Context,
	_ *connect.Request[diagv1.ListViewsRequest],
) (*connect.Response[diagv1.ListViewsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}

	views, err := s.Store.ListViews(ctx, workspaceID)
	if err != nil {
		return nil, storeErr("list views", err)
	}
	if views == nil {
		views = []*diagv1.View{}
	}
	return connect.NewResponse(&diagv1.ListViewsResponse{Views: views}), nil
}

func (s *WorkspaceService) GetWorkspace(
	ctx context.Context,
	req *connect.Request[diagv1.GetWorkspaceRequest],
) (*connect.Response[diagv1.GetWorkspaceResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	m := req.Msg

	var parentViewID *int32
	if m.GetParentId() != 0 {
		pid := m.GetParentId()
		parentViewID = &pid
	}
	var isRoot *bool
	if m.Level != nil {
		ir := m.GetLevel() == 0
		isRoot = &ir
	}

	views, totalCount, err := s.Store.GetViews(ctx, workspaceID, parentViewID, isRoot, m.GetSearch(), int(m.GetLimit()), int(m.GetOffset()))
	if err != nil {
		return nil, storeErr("get views", err)
	}

	tc := intToInt32(totalCount)

	viewMap := make(map[int32]*diagv1.View)
	for _, v := range views {
		viewMap[v.Id] = v
	}

	var roots []*diagv1.View
	for _, v := range views {
		p := viewMap[v.Id]
		if v.ParentViewId != nil {
			if parent, ok := viewMap[*v.ParentViewId]; ok {
				parent.Children = append(parent.Children, p)
				continue
			}
		}
		roots = append(roots, p)
	}

	var calcDepth func(v *diagv1.View, d int32)
	calcDepth = func(v *diagv1.View, d int32) {
		v.Depth = d
		for _, c := range v.Children {
			calcDepth(c, d+1)
		}
	}
	for _, r := range roots {
		calcDepth(r, 0)
	}

	resp := &diagv1.GetWorkspaceResponse{TotalCount: tc, Views: roots}

	if m.GetIncludeContent() && len(views) > 0 {
		var allPlacements []*diagv1.PlacedElement
		var allConnectors []*diagv1.Connector

		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error { var e error; allPlacements, e = s.Store.ListAllPlacements(gctx, workspaceID); return e })
		g.Go(func() error { var e error; allConnectors, e = s.Store.ListAllConnectors(gctx, workspaceID); return e })
		if err := g.Wait(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch content: %w", err))
		}

		resp.Content = make(map[int32]*diagv1.ViewContent)
		for _, p := range allPlacements {
			if _, ok := resp.Content[p.ViewId]; !ok {
				resp.Content[p.ViewId] = &diagv1.ViewContent{}
			}
			resp.Content[p.ViewId].Placements = append(resp.Content[p.ViewId].Placements, &diagv1.PlacedElement{
				Id: p.Id, ViewId: p.ViewId, ElementId: p.ElementId,
				PositionX: p.PositionX, PositionY: p.PositionY,
				Name: p.Name, Kind: p.Kind, Tags: p.Tags,
				Description: p.Description, Technology: p.Technology,
				Url: p.Url, LogoUrl: p.LogoUrl, TechnologyLinks: p.TechnologyLinks,
				Repo: p.Repo, Branch: p.Branch, Language: p.Language, FilePath: p.FilePath,
			})
		}
		for _, c := range allConnectors {
			if _, ok := resp.Content[c.ViewId]; !ok {
				resp.Content[c.ViewId] = &diagv1.ViewContent{}
			}
			resp.Content[c.ViewId].Connectors = append(resp.Content[c.ViewId].Connectors, c)
		}

		// Derive navigation links from owner_element_id relationships.
		elementToChildView := make(map[int32]*diagv1.View, len(views))
		for _, v := range views {
			if v.OwnerElementId != nil {
				elementToChildView[*v.OwnerElementId] = v
			}
		}
		for _, p := range allPlacements {
			childView, ok := elementToChildView[p.ElementId]
			if !ok {
				continue
			}
			eid := p.ElementId
			resp.Navigations = append(resp.Navigations, &diagv1.ElementNavigationInfo{
				ElementId:    &eid,
				FromViewId:   p.ViewId,
				ToViewId:     childView.Id,
				ToViewName:   childView.Name,
				RelationType: "child",
			})
		}
	}

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) GetView(
	ctx context.Context,
	req *connect.Request[diagv1.GetViewRequest],
) (*connect.Response[diagv1.GetViewResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	v, err := s.Store.GetView(ctx, viewID, workspaceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	return connect.NewResponse(&diagv1.GetViewResponse{View: v}), nil
}

func (s *WorkspaceService) UpdateView(
	ctx context.Context,
	req *connect.Request[diagv1.UpdateViewRequest],
) (*connect.Response[diagv1.UpdateViewResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, invalidArg("name", "must not be empty")
	}

	label := OptStr(req.Msg.GetLevelLabel())
	if label == nil {
		existing, err := s.Store.GetView(ctx, viewID, workspaceID)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
		}
		label = existing.LevelLabel
	}

	v, err := s.Store.UpdateView(ctx, viewID, workspaceID, name, label)
	if err != nil {
		return nil, storeErr("update view", err)
	}

	resp := &diagv1.UpdateViewResponse{View: v}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "view", strconv.Itoa(int(v.Id)), map[string]any{"name": v.Name}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) SetViewLevel(
	_ context.Context,
	_ *connect.Request[diagv1.SetViewLevelRequest],
) (*connect.Response[diagv1.SetViewLevelResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("set view level is deprecated"))
}

func (s *WorkspaceService) ListIncomingElementNavigations(
	ctx context.Context,
	req *connect.Request[diagv1.ListIncomingElementNavigationsRequest],
) (*connect.Response[diagv1.ListIncomingElementNavigationsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	links, err := s.Store.ListIncomingElementNavigations(ctx, viewID)
	if err != nil {
		return nil, storeErr("list incoming navigations", err)
	}
	if links == nil {
		links = []*diagv1.IncomingElementNavigationInfo{}
	}
	return connect.NewResponse(&diagv1.ListIncomingElementNavigationsResponse{Navigations: links}), nil
}

func (s *WorkspaceService) ListElements(
	ctx context.Context,
	req *connect.Request[diagv1.ListElementsRequest],
) (*connect.Response[diagv1.ListElementsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	elements, totalCount, err := s.Store.ListElements(ctx, workspaceID, req.Msg.Limit, req.Msg.Offset, req.Msg.Search)
	if err != nil {
		return nil, storeErr("list elements", err)
	}
	if elements == nil {
		elements = []*diagv1.Element{}
	}
	return connect.NewResponse(&diagv1.ListElementsResponse{
		Elements: elements,
		Pagination: &diagv1.Pagination{
			TotalCount: intToInt32(totalCount),
		},
	}), nil
}

func (s *WorkspaceService) GetElement(
	ctx context.Context,
	req *connect.Request[diagv1.GetElementRequest],
) (*connect.Response[diagv1.GetElementResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	elementID, err := parseRequiredInt32("element_id", req.Msg.GetElementId())
	if err != nil {
		return nil, err
	}
	e, err := s.Store.GetElement(ctx, elementID, workspaceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("element not found"))
	}
	return connect.NewResponse(&diagv1.GetElementResponse{Element: e}), nil
}

func (s *WorkspaceService) CreateElement(
	ctx context.Context,
	req *connect.Request[diagv1.CreateElementRequest],
) (*connect.Response[diagv1.CreateElementResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "elements"); err != nil {
		return nil, err
	}
	m := req.Msg
	if strings.TrimSpace(m.GetName()) == "" {
		return nil, invalidArg("name", "must not be empty")
	}
	techLinks, err := ConvertTechnologyLinks(m.GetTechnologyLinks())
	if err != nil {
		return nil, err
	}
	tags := m.GetTags()
	if tags == nil {
		tags = []string{}
	}
	input := ElementInput{
		Name:        strings.TrimSpace(m.GetName()),
		Description: OptStr(m.GetDescription()),
		Kind:        OptStr(m.GetKind()),
		Technology:  OptStr(m.GetTechnology()),
		URL:         OptStr(m.GetUrl()),
		LogoURL:     OptStr(m.GetLogoUrl()),
		TechLinks:   techLinks,
		Tags:        tags,
		Repo:        OptStr(m.GetRepo()),
		Branch:      OptStr(m.GetBranch()),
		Language:    OptStr(m.GetLanguage()),
		FilePath:    OptStr(m.GetFilePath()),
	}
	e, err := s.Store.CreateElement(ctx, workspaceID, input)
	if err != nil {
		return nil, storeErr("create element", err)
	}

	resp := &diagv1.CreateElementResponse{Element: e}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "element", strconv.Itoa(int(e.Id)), map[string]any{"name": e.Name}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) UpdateElement(
	ctx context.Context,
	req *connect.Request[diagv1.UpdateElementRequest],
) (*connect.Response[diagv1.UpdateElementResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "elements"); err != nil {
		return nil, err
	}
	m := req.Msg
	elementID, err := parseRequiredInt32("element_id", m.GetElementId())
	if err != nil {
		return nil, err
	}
	existing, err := s.Store.GetElement(ctx, elementID, workspaceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("element not found"))
	}

	techLinks, err := ConvertTechnologyLinks(m.GetTechnologyLinks())
	if err != nil {
		return nil, err
	}

	finalTechLinks := techLinks
	if finalTechLinks == nil {
		if m.Technology != nil && *m.Technology == "" {
			finalTechLinks = []*diagv1.TechnologyLink{}
			techLinks = finalTechLinks
		} else {
			finalTechLinks = existing.TechnologyLinks
		}
	}

	logoURL := m.LogoUrl
	hasPrimary := false
	for _, tl := range finalTechLinks {
		if tl.GetIsPrimaryIcon() {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		empty := ""
		logoURL = &empty
	}

	tags := m.GetTags()
	if tags == nil {
		tags = []string{}
	}

	input := ElementInput{
		Name:        m.GetName(),
		Description: m.Description,
		Kind:        m.Kind,
		Technology:  m.Technology,
		URL:         m.Url,
		LogoURL:     logoURL,
		TechLinks:   techLinks,
		Tags:        tags,
		Repo:        m.Repo,
		Branch:      m.Branch,
		Language:    m.Language,
		FilePath:    m.FilePath,
		HasView:     existing.HasView,
		ViewLabel:   existing.ViewLabel,
	}
	e, err := s.Store.UpdateElement(ctx, elementID, workspaceID, input)
	if err != nil {
		return nil, storeErr("update element", err)
	}

	resp := &diagv1.UpdateElementResponse{Element: e}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "element", strconv.Itoa(int(elementID)), map[string]any{"name": e.Name}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) ListElementPlacements(
	ctx context.Context,
	req *connect.Request[diagv1.ListElementPlacementsRequest],
) (*connect.Response[diagv1.ListElementPlacementsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	elementID, err := parseRequiredInt32("element_id", req.Msg.GetElementId())
	if err != nil {
		return nil, err
	}
	placements, err := s.Store.ListElementPlacements(ctx, elementID, workspaceID)
	if err != nil {
		return nil, storeErr("list element placements", err)
	}
	if placements == nil {
		placements = []*diagv1.ViewPlacementInfo{}
	}
	return connect.NewResponse(&diagv1.ListElementPlacementsResponse{Placements: placements}), nil
}

func (s *WorkspaceService) ListPlacements(
	ctx context.Context,
	req *connect.Request[diagv1.ListPlacementsRequest],
) (*connect.Response[diagv1.ListPlacementsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	items, err := s.Store.ListPlacements(ctx, viewID)
	if err != nil {
		return nil, storeErr("list placements", err)
	}
	return connect.NewResponse(&diagv1.ListPlacementsResponse{Placements: items}), nil
}

func (s *WorkspaceService) CreatePlacement(
	ctx context.Context,
	req *connect.Request[diagv1.CreatePlacementRequest],
) (*connect.Response[diagv1.CreatePlacementResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	m := req.Msg
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	elementID, err := parseRequiredInt32("element_id", m.GetElementId())
	if err != nil {
		return nil, err
	}
	ve, err := s.Store.AddPlacement(ctx, viewID, elementID, m.GetPositionX(), m.GetPositionY())
	if err != nil {
		return nil, storeErr("add placement", err)
	}

	resp := &diagv1.CreatePlacementResponse{Placement: &diagv1.PlacedElement{
		Id:        ve.Id,
		ViewId:    ve.ViewId,
		ElementId: ve.ElementId,
		PositionX: ve.PositionX,
		PositionY: ve.PositionY,
	}}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "placement", strconv.Itoa(int(ve.Id)), map[string]any{"view_id": viewID, "element_id": elementID}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) UpdatePlacementPosition(
	ctx context.Context,
	req *connect.Request[diagv1.UpdatePlacementPositionRequest],
) (*connect.Response[diagv1.UpdatePlacementPositionResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	m := req.Msg
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	elementID, err := parseRequiredInt32("element_id", m.GetElementId())
	if err != nil {
		return nil, err
	}
	if err := s.Store.UpdatePlacementPosition(ctx, viewID, elementID, m.GetPositionX(), m.GetPositionY()); err != nil {
		return nil, storeErr("update placement position", err)
	}

	resp := &diagv1.UpdatePlacementPositionResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "placement", "", map[string]any{"view_id": viewID, "element_id": elementID, "position_x": m.GetPositionX(), "position_y": m.GetPositionY()}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) DeletePlacement(
	ctx context.Context,
	req *connect.Request[diagv1.DeletePlacementRequest],
) (*connect.Response[diagv1.DeletePlacementResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	m := req.Msg
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	elementID, err := parseRequiredInt32("element_id", m.GetElementId())
	if err != nil {
		return nil, err
	}
	if err := s.Store.RemovePlacement(ctx, viewID, elementID); err != nil {
		return nil, storeErr("remove placement", err)
	}

	resp := &diagv1.DeletePlacementResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "placement", "", map[string]any{"view_id": viewID, "element_id": elementID}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) ListConnectors(
	ctx context.Context,
	req *connect.Request[diagv1.ListConnectorsRequest],
) (*connect.Response[diagv1.ListConnectorsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID := req.Msg.GetViewId()

	var connectors []*diagv1.Connector
	var err error
	if viewID == 0 {
		connectors, err = s.Store.ListAllConnectors(ctx, workspaceID)
	} else {
		connectors, err = s.Store.ListConnectors(ctx, viewID, workspaceID)
	}
	if err != nil {
		return nil, storeErr("list connectors", err)
	}
	return connect.NewResponse(&diagv1.ListConnectorsResponse{Connectors: connectors}), nil
}

func (s *WorkspaceService) CreateConnector(
	ctx context.Context,
	req *connect.Request[diagv1.CreateConnectorRequest],
) (*connect.Response[diagv1.CreateConnectorResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "connectors"); err != nil {
		return nil, err
	}
	m := req.Msg
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	sourceID, err := parseRequiredInt32("source_element_id", m.GetSourceElementId())
	if err != nil {
		return nil, err
	}
	targetID, err := parseRequiredInt32("target_element_id", m.GetTargetElementId())
	if err != nil {
		return nil, err
	}
	direction := m.GetDirection()
	if direction == "" {
		direction = "forward"
	}
	if err := validateDirection(direction); err != nil {
		return nil, err
	}
	style := m.GetStyle()
	if style == "" {
		style = "bezier"
	}
	if err := validateEdgeType(style); err != nil {
		return nil, err
	}
	c, err := s.Store.CreateConnector(ctx, workspaceID, ConnectorInput{
		ViewID: viewID, SourceID: sourceID, TargetID: targetID,
		Label: OptStr(m.GetLabel()), Description: OptStr(m.GetDescription()),
		Relationship: OptStr(m.GetRelationship()), Direction: direction, Style: style,
		URL: OptStr(m.GetUrl()), SourceHandle: OptStr(m.GetSourceHandle()),
		TargetHandle: OptStr(m.GetTargetHandle()),
	})
	if err != nil {
		return nil, storeErr("create connector", err)
	}

	resp := &diagv1.CreateConnectorResponse{Connector: c}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "connector", strconv.Itoa(int(c.Id)), map[string]any{"view_id": viewID, "label": c.Label}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) UpdateConnector(
	ctx context.Context,
	req *connect.Request[diagv1.UpdateConnectorRequest],
) (*connect.Response[diagv1.UpdateConnectorResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "connectors"); err != nil {
		return nil, err
	}
	m := req.Msg
	connectorID, err := parseRequiredInt32("connector_id", m.GetConnectorId())
	if err != nil {
		return nil, err
	}
	existing, err := s.Store.GetConnector(ctx, connectorID, workspaceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("connector not found"))
	}

	sourceID := existing.SourceElementId
	if m.GetSourceElementId() != 0 {
		sid, err := parseRequiredInt32("source_element_id", m.GetSourceElementId())
		if err != nil {
			return nil, err
		}
		sourceID = sid
	}
	targetID := existing.TargetElementId
	if m.GetTargetElementId() != 0 {
		tid, err := parseRequiredInt32("target_element_id", m.GetTargetElementId())
		if err != nil {
			return nil, err
		}
		targetID = tid
	}

	sourceHandle := existing.SourceHandle
	if m.GetSourceHandle() != "" {
		sourceHandle = OptStr(m.GetSourceHandle())
	}
	targetHandle := existing.TargetHandle
	if m.GetTargetHandle() != "" {
		targetHandle = OptStr(m.GetTargetHandle())
	}

	direction := m.GetDirection()
	if direction == "" {
		direction = existing.Direction
	}
	if err := validateDirection(direction); err != nil {
		return nil, err
	}
	style := m.GetStyle()
	if style == "" {
		style = existing.Style
	}
	if err := validateEdgeType(style); err != nil {
		return nil, err
	}

	c, err := s.Store.UpdateConnector(ctx, connectorID, workspaceID, ConnectorInput{
		ViewID: existing.ViewId, SourceID: sourceID, TargetID: targetID,
		Label: OptStr(m.GetLabel()), Description: OptStr(m.GetDescription()),
		Relationship: OptStr(m.GetRelationship()), Direction: direction, Style: style,
		URL: OptStr(m.GetUrl()), SourceHandle: sourceHandle, TargetHandle: targetHandle,
	})
	if err != nil {
		return nil, storeErr("update connector", err)
	}

	resp := &diagv1.UpdateConnectorResponse{Connector: c}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "connector", strconv.Itoa(int(connectorID)), map[string]any{"view_id": existing.ViewId, "label": c.Label}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) ListElementNavigations(
	ctx context.Context,
	req *connect.Request[diagv1.ListElementNavigationsRequest],
) (*connect.Response[diagv1.ListElementNavigationsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	elementID, err := parseRequiredInt32("element_id", req.Msg.GetElementId())
	if err != nil {
		return nil, err
	}
	links, err := s.Store.ListElementNavigations(ctx, workspaceID, elementID)
	if err != nil {
		return nil, storeErr("list element navigations", err)
	}
	if links == nil {
		links = []*diagv1.ElementNavigationInfo{}
	}
	return connect.NewResponse(&diagv1.ListElementNavigationsResponse{Navigations: links}), nil
}

func (s *WorkspaceService) CreateElementNavigation(
	_ context.Context,
	_ *connect.Request[diagv1.CreateElementNavigationRequest],
) (*connect.Response[diagv1.CreateElementNavigationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("manual navigation creation is no longer supported"))
}

func (s *WorkspaceService) DeleteElementNavigationById(
	_ context.Context,
	_ *connect.Request[diagv1.DeleteElementNavigationByIdRequest],
) (*connect.Response[diagv1.DeleteElementNavigationByIdResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("manual navigation deletion is no longer supported"))
}

// ─── View layers ─────────────────────────────────────────────────────────────

func (s *WorkspaceService) ListViewLayers(
	ctx context.Context,
	req *connect.Request[diagv1.ListViewLayersRequest],
) (*connect.Response[diagv1.ListViewLayersResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	layers, err := s.Store.ListViewLayers(ctx, viewID)
	if err != nil {
		return nil, storeErr("list layers", err)
	}
	return connect.NewResponse(&diagv1.ListViewLayersResponse{Layers: layers}), nil
}

func (s *WorkspaceService) CreateViewLayer(
	ctx context.Context,
	req *connect.Request[diagv1.CreateViewLayerRequest],
) (*connect.Response[diagv1.CreateViewLayerResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	m := req.Msg
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	name := strings.TrimSpace(m.GetName())
	if name == "" {
		return nil, invalidArg("name", "must not be empty")
	}
	color := strings.TrimSpace(m.GetColor())
	l, err := s.Store.CreateViewLayer(ctx, viewID, name, m.GetTags(), color)
	if err != nil {
		return nil, storeErr("create layer", err)
	}

	resp := &diagv1.CreateViewLayerResponse{Layer: l}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "layer", strconv.Itoa(int(l.Id)), map[string]any{"view_id": viewID, "name": name}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) UpdateViewLayer(
	ctx context.Context,
	req *connect.Request[diagv1.UpdateViewLayerRequest],
) (*connect.Response[diagv1.UpdateViewLayerResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	m := req.Msg
	layerID, err := parseRequiredInt32("layer_id", m.GetLayerId())
	if err != nil {
		return nil, err
	}
	existing, err := s.Store.GetViewLayer(ctx, layerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("layer not found"))
	}
	if _, err := s.Store.GetView(ctx, existing.ViewId, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	l, err := s.Store.UpdateViewLayer(ctx, layerID, m.Name, m.GetTags(), m.Color)
	if err != nil {
		return nil, storeErr("update layer", err)
	}

	resp := &diagv1.UpdateViewLayerResponse{Layer: l}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "layer", strconv.Itoa(int(layerID)), map[string]any{"view_id": existing.ViewId, "name": l.Name}, resp)

	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) DeleteViewLayer(
	ctx context.Context,
	req *connect.Request[diagv1.DeleteViewLayerRequest],
) (*connect.Response[diagv1.DeleteViewLayerResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	layerID, err := parseRequiredInt32("layer_id", req.Msg.GetLayerId())
	if err != nil {
		return nil, err
	}
	existing, err := s.Store.GetViewLayer(ctx, layerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("layer not found"))
	}
	if _, err := s.Store.GetView(ctx, existing.ViewId, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	if err := s.Store.DeleteViewLayer(ctx, layerID); err != nil {
		return nil, storeErr("delete layer", err)
	}

	resp := &diagv1.DeleteViewLayerResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "layer", strconv.Itoa(int(layerID)), map[string]any{"view_id": existing.ViewId}, resp)

	return connect.NewResponse(resp), nil
}

// uuid is imported for uuid.UUID type in Store interface, keep it used via WorkspaceIDFromCtx
var _ = uuid.Nil
