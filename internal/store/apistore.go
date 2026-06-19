package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func jsonString(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

var _ api.Store = (*APIAdapter)(nil)

// APIAdapter exposes the local SQLite-backed store through the shared
// ConnectRPC-oriented api.Store contract.
type APIAdapter struct {
	Store        *SQLiteStore
	DataDir      string
	WorkspaceDir string
}

func NewAPIAdapter(store *SQLiteStore, paths ...string) *APIAdapter {
	adapter := &APIAdapter{Store: store}
	if len(paths) > 0 {
		adapter.DataDir = paths[0]
	}
	if len(paths) > 1 {
		adapter.WorkspaceDir = paths[1]
	}
	return adapter
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

func (a *APIAdapter) GetViews(ctx context.Context, _ uuid.UUID, parentViewID *int32, isRoot *bool, search string, limit, offset int) ([]*diagv1.View, int, error) {
	var flat []app.ViewTreeNode
	switch {
	case parentViewID != nil:
		nodes, err := a.Store.legacy.ChildViews(ctx, int64(*parentViewID))
		if err != nil {
			return nil, 0, err
		}
		flat = nodes
	case isRoot != nil && *isRoot:
		nodes, err := a.Store.legacy.RootViews(ctx)
		if err != nil {
			return nil, 0, err
		}
		flat = nodes
	default:
		nodes, err := a.Store.ViewTree(ctx)
		if err != nil {
			return nil, 0, err
		}
		flat = flattenViewTreeNodes(nodes)
	}
	filtered := make([]app.ViewTreeNode, 0, len(flat))
	for _, node := range flat {
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

func (a *APIAdapter) UpdateView(ctx context.Context, id int32, _ uuid.UUID, name string, description *string, label *string, tags []string) (*diagv1.View, error) {
	nameCopy := name
	view, err := a.Store.legacy.UpdateView(ctx, int64(id), &nameCopy, description, label, tags)
	if err != nil {
		return nil, err
	}
	return a.GetView(ctx, int32(view.ID), uuid.Nil)
}

func (a *APIAdapter) DeleteView(ctx context.Context, id int32, _ uuid.UUID) error {
	return a.Store.legacy.DeleteView(ctx, int64(id))
}

func (a *APIAdapter) ListElements(ctx context.Context, _ uuid.UUID, limit, offset int32, search string) ([]*diagv1.Element, int, error) {
	elements, total, err := a.Store.legacy.Elements(ctx, int(limit), int(offset), search)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*diagv1.Element, 0, len(elements))
	workspaceID := api.WorkspaceIDFromCtx(ctx)
	for _, element := range elements {
		out = append(out, elementToProto(element, workspaceID))
	}
	return out, total, nil
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
		BypassNoiseGate:      boolValue(input.BypassNoiseGate),
		BypassNoiseGateSet:   input.BypassNoiseGate != nil,
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
		BypassNoiseGate:      boolValue(input.BypassNoiseGate),
		BypassNoiseGateSet:   input.BypassNoiseGate != nil,
		HasView:              input.HasView,
		ViewLabel:            input.ViewLabel,
	})
	if err != nil {
		return nil, err
	}
	return elementToProto(element, api.WorkspaceIDFromCtx(ctx)), nil
}

func (a *APIAdapter) DeleteElement(ctx context.Context, id int32, _ uuid.UUID) error {
	if err := a.Store.DeleteResourceVisibilityOverrides(ctx, "element", int64(id)); err != nil {
		return err
	}
	return a.Store.legacy.DeleteElement(ctx, int64(id))
}

func (a *APIAdapter) ListPlacements(ctx context.Context, viewID int32) ([]*diagv1.PlacedElement, error) {
	content, err := a.Store.ProjectedViewContent(ctx, int64(viewID), nil)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.PlacedElement, 0, len(content.Placements))
	for _, placement := range content.Placements {
		out = append(out, placedElementToProto(placement))
	}
	return out, nil
}

func (a *APIAdapter) ListAllPlacements(ctx context.Context, _ uuid.UUID) ([]*diagv1.PlacedElement, error) {
	placements, err := a.Store.legacy.AllPlacements(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.PlacedElement, 0, len(placements))
	for _, placement := range placements {
		out = append(out, placedElementToProto(placement))
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
	content, err := a.Store.ProjectedViewContent(ctx, int64(viewID), nil)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.Connector, 0, len(content.Connectors))
	for _, connector := range content.Connectors {
		out = append(out, connectorToProto(connector))
	}
	return out, nil
}

func (a *APIAdapter) GetProjectedViewContent(ctx context.Context, viewID int32, _ uuid.UUID, densityOverride *int32) (*diagv1.ViewContent, error) {
	var override *int
	if densityOverride != nil {
		value := int(*densityOverride)
		override = &value
	}
	content, err := a.Store.ProjectedViewContent(ctx, int64(viewID), override)
	if err != nil {
		return nil, err
	}
	out := &diagv1.ViewContent{
		Placements: make([]*diagv1.PlacedElement, 0, len(content.Placements)),
		Connectors: make([]*diagv1.Connector, 0, len(content.Connectors)),
	}
	for _, placement := range content.Placements {
		out.Placements = append(out.Placements, placedElementToProto(placement))
	}
	for _, connector := range content.Connectors {
		out.Connectors = append(out.Connectors, connectorToProto(connector))
	}
	return out, nil
}

func (a *APIAdapter) ListAllConnectors(ctx context.Context, _ uuid.UUID) ([]*diagv1.Connector, error) {
	connectors, err := a.Store.legacy.AllConnectors(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.Connector, 0, len(connectors))
	for _, connector := range connectors {
		out = append(out, connectorToProto(connector))
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
		Tags:            cloneStrings(input.Tags),
	})
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIAdapter) createConnectorWithID(ctx context.Context, id int32, input api.ConnectorInput) (*diagv1.Connector, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	row := &connectorInsertModel{
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
		Tags:            jsonString(input.Tags, "[]"),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := a.Store.legacy.BunDB().NewInsert().
		Model(row).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	connector, err := a.Store.legacy.ConnectorByID(ctx, int64(id))
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
		Tags:            cloneStrings(input.Tags),
	})
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIAdapter) DeleteConnector(ctx context.Context, id int32, _ uuid.UUID) error {
	if err := a.Store.DeleteResourceVisibilityOverrides(ctx, "connector", int64(id)); err != nil {
		return err
	}
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
	layers, err := a.Store.legacy.AllLayers(ctx)
	if err != nil {
		return nil, err
	}
	byView := make(map[int64][]app.ViewLayer)
	for _, layer := range layers {
		byView[layer.DiagramID] = append(byView[layer.DiagramID], layer)
	}
	var out []*diagv1.ViewLayer
	for _, node := range flattenViewTreeNodes(nodes) {
		for _, layer := range byView[node.ID] {
			out = append(out, layerToProto(layer))
		}
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

func (a *APIAdapter) Tags(ctx context.Context, _ uuid.UUID) (map[string]*diagv1.Tag, error) {
	tags, err := a.Store.Tags(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*diagv1.Tag, len(tags))
	for name, tag := range tags {
		out[name] = &diagv1.Tag{
			Color:       tag.Color,
			Description: tag.Description,
		}
	}
	return out, nil
}

func (a *APIAdapter) UpdateTag(ctx context.Context, _ uuid.UUID, name, color string, description *string) error {
	return a.Store.UpdateTag(ctx, name, color, description)
}

func (a *APIAdapter) DeleteTag(ctx context.Context, _ uuid.UUID, name string) error {
	return a.Store.DeleteTag(ctx, name)
}

func collaborationWorkspaceKey(workspaceID uuid.UUID) string {
	if workspaceID == uuid.Nil {
		return "local"
	}
	return workspaceID.String()
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func parseStoredTime(value string) *timestamppb.Timestamp {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return timestamppb.New(time.Now().UTC())
	}
	return timestamppb.New(parsed)
}

func (a *APIAdapter) ListViewThreads(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32) ([]*diagv1.ThreadInfo, error) {
	where := `workspace_id = ? AND view_id = ? AND element_id = ?`
	targetID := any(elementID)
	if connectorID != nil {
		where = `workspace_id = ? AND view_id = ? AND connector_id = ?`
		targetID = connectorID
	}
	var rows []struct {
		ID                int32   `bun:"id"`
		WorkspaceID       string  `bun:"workspace_id"`
		ViewID            int32   `bun:"view_id"`
		ElementID         *int32  `bun:"element_id"`
		ConnectorID       *int32  `bun:"connector_id"`
		CreatedBy         string  `bun:"created_by"`
		CreatedByUsername string  `bun:"created_by_username"`
		Status            string  `bun:"status"`
		CreatedAt         string  `bun:"created_at"`
		ResolvedAt        *string `bun:"resolved_at"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`SELECT t.id, t.workspace_id, t.view_id, t.element_id, t.connector_id, t.created_by,
		        COALESCE(NULLIF(t.created_by_username, ''), t.created_by) AS created_by_username,
		        t.status, t.created_at, t.resolved_at
		 FROM view_threads t
		 WHERE `+where+`
		 ORDER BY t.created_at ASC`,
		collaborationWorkspaceKey(workspaceID), viewID, targetID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	threads := make([]*diagv1.ThreadInfo, 0, len(rows))
	for _, row := range rows {
		thread := threadRowToProto(row.ID, row.WorkspaceID, row.ViewID, row.ElementID, row.ConnectorID, row.CreatedBy, row.CreatedByUsername, row.Status, row.CreatedAt, row.ResolvedAt)
		comments, err := a.listViewComments(ctx, workspaceID, viewID, row.ID)
		if err != nil {
			return nil, err
		}
		thread.Comments = comments
		threads = append(threads, thread)
	}
	return threads, nil
}

func (a *APIAdapter) CreateViewThread(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32, createdBy, createdByUsername string) (*diagv1.ThreadInfo, error) {
	now := nowRFC3339()
	var row struct {
		ID int32 `bun:"id"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`INSERT INTO view_threads (workspace_id, view_id, element_id, connector_id, created_by, created_by_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'open', ?)
		 RETURNING id`,
		collaborationWorkspaceKey(workspaceID), viewID, elementID, connectorID, createdBy, createdByUsername, now,
	).Scan(ctx, &row)
	if err != nil {
		return nil, err
	}
	return a.GetViewThread(ctx, workspaceID, viewID, row.ID)
}

func (a *APIAdapter) GetViewThread(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) (*diagv1.ThreadInfo, error) {
	var row struct {
		ID                int32   `bun:"id"`
		WorkspaceID       string  `bun:"workspace_id"`
		ViewID            int32   `bun:"view_id"`
		ElementID         *int32  `bun:"element_id"`
		ConnectorID       *int32  `bun:"connector_id"`
		CreatedBy         string  `bun:"created_by"`
		CreatedByUsername string  `bun:"created_by_username"`
		Status            string  `bun:"status"`
		CreatedAt         string  `bun:"created_at"`
		ResolvedAt        *string `bun:"resolved_at"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`SELECT t.id, t.workspace_id, t.view_id, t.element_id, t.connector_id, t.created_by,
		        COALESCE(NULLIF(t.created_by_username, ''), t.created_by) AS created_by_username,
		        t.status, t.created_at, t.resolved_at
		 FROM view_threads t
		 WHERE t.workspace_id = ? AND t.view_id = ? AND t.id = ?`,
		collaborationWorkspaceKey(workspaceID), viewID, threadID,
	).Scan(ctx, &row)
	if err != nil {
		return nil, err
	}
	thread := threadRowToProto(row.ID, row.WorkspaceID, row.ViewID, row.ElementID, row.ConnectorID, row.CreatedBy, row.CreatedByUsername, row.Status, row.CreatedAt, row.ResolvedAt)
	comments, err := a.listViewComments(ctx, workspaceID, viewID, threadID)
	if err != nil {
		return nil, err
	}
	thread.Comments = comments
	return thread, nil
}

func (a *APIAdapter) SetViewThreadResolved(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, resolved bool) error {
	status := "open"
	var resolvedAt *string
	if resolved {
		status = "resolved"
		now := nowRFC3339()
		resolvedAt = &now
	}
	_, err := a.Store.legacy.BunDB().NewRaw(
		`UPDATE view_threads
		 SET status = ?, resolved_at = ?
		 WHERE workspace_id = ? AND view_id = ? AND id = ?`,
		status, resolvedAt, collaborationWorkspaceKey(workspaceID), viewID, threadID,
	).Exec(ctx)
	return err
}

func (a *APIAdapter) CreateViewComment(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, authorID, authorUsername, body string) (*diagv1.CommentInfo, error) {
	now := nowRFC3339()
	var row struct {
		ID int32 `bun:"id"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`INSERT INTO view_comments (workspace_id, view_id, thread_id, author_id, author_username, body, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		collaborationWorkspaceKey(workspaceID), viewID, threadID, authorID, authorUsername, body, now, now,
	).Scan(ctx, &row)
	if err != nil {
		return nil, err
	}
	comments, err := a.listViewComments(ctx, workspaceID, viewID, threadID)
	if err != nil {
		return nil, err
	}
	for _, comment := range comments {
		if comment.Id == row.ID {
			return comment, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (a *APIAdapter) listViewComments(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) ([]*diagv1.CommentInfo, error) {
	var rows []struct {
		ID             int32  `bun:"id"`
		WorkspaceID    string `bun:"workspace_id"`
		ViewID         int32  `bun:"view_id"`
		ThreadID       int32  `bun:"thread_id"`
		AuthorID       string `bun:"author_id"`
		AuthorUsername string `bun:"author_username"`
		Body           string `bun:"body"`
		CreatedAt      string `bun:"created_at"`
		UpdatedAt      string `bun:"updated_at"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`SELECT c.id, c.workspace_id, c.view_id, c.thread_id, c.author_id,
		        COALESCE(NULLIF(c.author_username, ''), c.author_id) AS author_username,
		        c.body, c.created_at, c.updated_at
		 FROM view_comments c
		 WHERE c.workspace_id = ? AND c.view_id = ? AND c.thread_id = ?
		 ORDER BY c.created_at ASC`,
		collaborationWorkspaceKey(workspaceID), viewID, threadID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.CommentInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, &diagv1.CommentInfo{
			Id:             row.ID,
			OrgId:          row.WorkspaceID,
			ViewId:         row.ViewID,
			ThreadId:       row.ThreadID,
			AuthorId:       row.AuthorID,
			AuthorUsername: row.AuthorUsername,
			Body:           row.Body,
			CreatedAt:      parseStoredTime(row.CreatedAt),
			UpdatedAt:      parseStoredTime(row.UpdatedAt),
		})
	}
	return out, nil
}

func threadRowToProto(id int32, workspaceID string, viewID int32, elementID, connectorID *int32, createdBy, createdByUsername, status, createdAt string, resolvedAt *string) *diagv1.ThreadInfo {
	thread := &diagv1.ThreadInfo{
		Id:                id,
		OrgId:             workspaceID,
		ViewId:            viewID,
		ElementId:         elementID,
		ConnectorId:       connectorID,
		CreatedBy:         createdBy,
		CreatedByUsername: createdByUsername,
		Status:            status,
		CreatedAt:         parseStoredTime(createdAt),
		Comments:          []*diagv1.CommentInfo{},
	}
	if resolvedAt != nil {
		thread.ResolvedAt = parseStoredTime(*resolvedAt)
	}
	return thread
}

func (a *APIAdapter) ListViewElementReactions(ctx context.Context, workspaceID uuid.UUID, viewID int32, userID string) ([]*diagv1.NodeReactionSummary, error) {
	var rows []struct {
		ElementID   int32  `bun:"element_id"`
		Emoji       string `bun:"emoji"`
		Count       int32  `bun:"reaction_count"`
		ReactedByMe int32  `bun:"reacted_by_me"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`SELECT element_id, emoji, COUNT(*) AS reaction_count,
		        MAX(CASE WHEN user_id = ? THEN 1 ELSE 0 END) AS reacted_by_me
		 FROM element_reactions
		 WHERE workspace_id = ? AND view_id = ?
		 GROUP BY element_id, emoji
		 ORDER BY element_id ASC, emoji ASC`,
		userID, collaborationWorkspaceKey(workspaceID), viewID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.NodeReactionSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, &diagv1.NodeReactionSummary{
			ElementId:   row.ElementID,
			Emoji:       row.Emoji,
			Count:       row.Count,
			ReactedByMe: row.ReactedByMe > 0,
		})
	}
	return out, nil
}

func (a *APIAdapter) ToggleElementReaction(ctx context.Context, workspaceID uuid.UUID, viewID, elementID int32, userID, emoji string) (bool, error) {
	result, err := a.Store.legacy.BunDB().NewRaw(
		`DELETE FROM element_reactions
		 WHERE workspace_id = ? AND view_id = ? AND element_id = ? AND user_id = ? AND emoji = ?`,
		collaborationWorkspaceKey(workspaceID), viewID, elementID, userID, emoji,
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return false, nil
	}
	now := nowRFC3339()
	_, err = a.Store.legacy.BunDB().NewRaw(
		`INSERT INTO element_reactions (workspace_id, view_id, element_id, user_id, emoji, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		collaborationWorkspaceKey(workspaceID), viewID, elementID, userID, emoji, now,
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *APIAdapter) ListDrawings(ctx context.Context, workspaceID uuid.UUID, viewID int32) ([]api.RealtimeDrawingInput, error) {
	var rows []struct {
		PathID   string  `bun:"path_id"`
		UserID   string  `bun:"user_id"`
		Points   string  `bun:"points"`
		Color    string  `bun:"color"`
		Width    float64 `bun:"width"`
		Text     string  `bun:"text"`
		FontSize float64 `bun:"font_size"`
	}
	err := a.Store.legacy.BunDB().NewRaw(
		`SELECT path_id, user_id, points, color, width, text, font_size
		 FROM drawings
		 WHERE workspace_id = ? AND view_id = ?
		 ORDER BY created_at ASC`,
		collaborationWorkspaceKey(workspaceID), viewID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]api.RealtimeDrawingInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.RealtimeDrawingInput{
			PathID:   row.PathID,
			UserID:   row.UserID,
			Points:   json.RawMessage(row.Points),
			Color:    row.Color,
			Width:    row.Width,
			Text:     row.Text,
			FontSize: row.FontSize,
		})
	}
	return out, nil
}

func (a *APIAdapter) UpsertDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, input api.RealtimeDrawingInput) error {
	now := nowRFC3339()
	_, err := a.Store.legacy.BunDB().NewRaw(
		`INSERT INTO drawings (workspace_id, view_id, user_id, path_id, points, color, width, text, font_size, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (workspace_id, view_id, path_id) DO UPDATE SET
		   user_id = excluded.user_id,
		   points = excluded.points,
		   color = excluded.color,
		   width = excluded.width,
		   text = excluded.text,
		   font_size = excluded.font_size,
		   updated_at = excluded.updated_at`,
		collaborationWorkspaceKey(workspaceID), viewID, input.UserID, input.PathID, string(input.Points), input.Color, input.Width, input.Text, input.FontSize, now, now,
	).Exec(ctx)
	return err
}

func (a *APIAdapter) DeleteDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, pathID string) error {
	_, err := a.Store.legacy.BunDB().NewRaw(
		`DELETE FROM drawings WHERE workspace_id = ? AND view_id = ? AND path_id = ?`,
		collaborationWorkspaceKey(workspaceID), viewID, pathID,
	).Exec(ctx)
	return err
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
	autoLayoutTargets := map[int64]map[int64]struct{}{}

	for _, planned := range req.GetElements() {
		if planned.GetRef() == "" {
			return nil, fmt.Errorf("plan element ref is required")
		}
		bypassNoiseGate := true
		if planned.BypassNoiseGate != nil {
			bypassNoiseGate = planned.GetBypassNoiseGate()
		}

		input := api.ElementInput{
			Name:            planned.GetName(),
			Description:     planned.Description,
			Kind:            planned.Kind,
			Technology:      planned.Technology,
			URL:             planned.Url,
			LogoURL:         planned.LogoUrl,
			TechLinks:       cloneTechLinks(planned.GetTechnologyLinks()),
			Tags:            cloneStrings(planned.GetTags()),
			Repo:            planned.Repo,
			Branch:          planned.Branch,
			Language:        planned.Language,
			FilePath:        planned.FilePath,
			BypassNoiseGate: &bypassNoiseGate,
			HasView:         planned.GetHasView(),
			ViewLabel:       planned.ViewLabel,
		}

		var element *diagv1.Element
		if planned.GetId() != 0 {
			element, err = a.UpdateElement(ctx, planned.GetId(), uuid.Nil, input)
			if errors.Is(err, sql.ErrNoRows) {
				element, err = a.CreateElement(ctx, uuid.Nil, input)
			}
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
				view, err = a.UpdateView(ctx, planned.GetViewId(), uuid.Nil, viewName, nil, planned.ViewLabel, nil)
				if errors.Is(err, sql.ErrNoRows) {
					ownerID := element.GetId()
					view, err = a.CreateView(ctx, uuid.Nil, &ownerID, viewName, planned.ViewLabel, false)
				}
			} else {
				ownerID := element.GetId()
				view, err = a.CreateView(ctx, uuid.Nil, &ownerID, viewName, planned.ViewLabel, false)
			}
			if err != nil {
				return nil, err
			}
			if planned.ViewDensityLevel != nil {
				if err := a.Store.SetViewDensityLevel(ctx, int64(view.GetId()), int(planned.GetViewDensityLevel())); err != nil {
					return nil, err
				}
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
			if placement.PositionX == nil && placement.PositionY == nil {
				if autoLayoutTargets[int64(viewID)] == nil {
					autoLayoutTargets[int64(viewID)] = map[int64]struct{}{}
				}
				autoLayoutTargets[int64(viewID)][int64(elementID)] = struct{}{}
			}
			if placement.VisibilityDelta != nil {
				if _, err := a.Store.SetVisibilityOverride(ctx, int64(viewID), "element", int64(elementID), int(placement.GetVisibilityDelta())); err != nil {
					return nil, err
				}
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
			if errors.Is(err, sql.ErrNoRows) {
				connector, err = a.createConnectorWithID(ctx, planned.GetId(), input)
			}
		} else {
			connector, err = a.CreateConnector(ctx, uuid.Nil, input)
		}
		if err != nil {
			return nil, err
		}
		if planned.VisibilityDelta != nil {
			if _, err := a.Store.SetVisibilityOverride(ctx, int64(viewID), "connector", int64(connector.GetId()), int(planned.GetVisibilityDelta())); err != nil {
				return nil, err
			}
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

	layoutPositions, err := a.layoutPlanPlacements(ctx, autoLayoutTargets)
	if err != nil {
		return nil, err
	}
	for _, placement := range resp.CreatedPlacements {
		if byElement, ok := layoutPositions[int64(placement.GetViewId())]; ok {
			if pos, ok := byElement[int64(placement.GetElementId())]; ok {
				placement.PositionX = pos.X
				placement.PositionY = pos.Y
			}
		}
	}

	return resp, nil
}

func (a *APIAdapter) layoutPlanPlacements(ctx context.Context, targets map[int64]map[int64]struct{}) (map[int64]map[int64]layout.Placement, error) {
	out := map[int64]map[int64]layout.Placement{}
	for viewID, elementIDs := range targets {
		if len(elementIDs) == 0 {
			continue
		}
		positions, err := a.layoutPlanView(ctx, viewID, elementIDs)
		if err != nil {
			return nil, err
		}
		out[viewID] = positions
	}
	return out, nil
}

func (a *APIAdapter) layoutPlanView(ctx context.Context, viewID int64, targets map[int64]struct{}) (map[int64]layout.Placement, error) {
	placements, err := a.planViewPlacementNodes(ctx, viewID)
	if err != nil {
		return nil, err
	}
	connectors, err := a.planViewLayoutConnectors(ctx, viewID)
	if err != nil {
		return nil, err
	}
	next := layout.LayoutPlacements(placements, targets, connectors, false)
	for elementID, pos := range next {
		if err := a.Store.legacy.UpdatePlacement(ctx, viewID, elementID, pos.X, pos.Y); err != nil {
			return nil, err
		}
	}
	return next, nil
}

func (a *APIAdapter) planViewPlacementNodes(ctx context.Context, viewID int64) ([]layout.Placement, error) {
	var rows []placementLayoutModel
	if err := a.Store.legacy.BunDB().NewSelect().
		Model(&rows).
		Column("element_id", "position_x", "position_y").
		Where("view_id = ?", viewID).
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]layout.Placement, 0, len(rows))
	for _, row := range rows {
		out = append(out, layout.Placement{ElementID: row.ElementID, X: row.PositionX, Y: row.PositionY})
	}
	return out, nil
}

func (a *APIAdapter) planViewLayoutConnectors(ctx context.Context, viewID int64) ([]layout.Connector, error) {
	var rows []connectorLayoutModel
	if err := a.Store.legacy.BunDB().NewSelect().
		Model(&rows).
		Column("source_element_id", "target_element_id").
		Where("view_id = ?", viewID).
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]layout.Connector, 0, len(rows))
	for _, row := range rows {
		out = append(out, layout.Connector{Source: row.SourceElementID, Target: row.TargetElementID})
	}
	return out, nil
}

// PruneMissingCLIResources removes resources previously owned by CLI metadata
// when they no longer appear in the current plan. It deliberately only acts on
// IDs from the prior metadata snapshot so watch/imported resources are left
// alone.
func (a *APIAdapter) PruneMissingCLIResources(ctx context.Context, workspaceID uuid.UUID, previous *workspace.Meta, req *diagv1.ApplyPlanRequest) error {
	if previous == nil {
		return nil
	}

	plannedElements := map[workspace.ResourceID]bool{}
	plannedViews := map[workspace.ResourceID]bool{}
	plannedConnectors := map[workspace.ResourceID]bool{}
	for _, element := range req.GetElements() {
		if element.GetId() != 0 {
			plannedElements[workspace.ResourceID(element.GetId())] = true
		}
		if element.GetHasView() && element.GetViewId() != 0 {
			plannedViews[workspace.ResourceID(element.GetViewId())] = true
		}
	}
	for _, connector := range req.GetConnectors() {
		if connector.GetId() != 0 {
			plannedConnectors[workspace.ResourceID(connector.GetId())] = true
		}
	}

	for _, metadata := range previous.Connectors {
		if metadata == nil || metadata.ID == 0 || plannedConnectors[metadata.ID] {
			continue
		}
		if err := a.DeleteConnector(ctx, int32(metadata.ID), workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	for _, metadata := range previous.Views {
		if metadata == nil || metadata.ID == 0 || plannedViews[metadata.ID] {
			continue
		}
		if err := a.DeleteView(ctx, int32(metadata.ID), workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	for _, metadata := range previous.Elements {
		if metadata == nil || metadata.ID == 0 || plannedElements[metadata.ID] {
			continue
		}
		if err := a.DeleteElement(ctx, int32(metadata.ID), workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (a *APIAdapter) ListVersions(ctx context.Context, workspaceID uuid.UUID, limit int) ([]*diagv1.WorkspaceVersionInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []workspaceVersionModel
	if err := a.Store.legacy.BunDB().NewSelect().
		Model(&rows).
		Order("id DESC").
		Limit(limit).
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*diagv1.WorkspaceVersionInfo, 0, len(rows))
	for _, row := range rows {
		version, err := workspaceVersionToProto(row, workspaceID)
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, nil
}

func (a *APIAdapter) GetLatestVersion(ctx context.Context, workspaceID uuid.UUID) (*diagv1.WorkspaceVersionInfo, error) {
	var row workspaceVersionModel
	err := a.Store.legacy.BunDB().NewSelect().
		Model(&row).
		Order("id DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, api.ErrUnimplemented
	}
	if err != nil {
		return nil, err
	}
	return workspaceVersionToProto(row, workspaceID)
}

func (a *APIAdapter) CreateVersion(ctx context.Context, workspaceID uuid.UUID, versionID, source string, parentID *int32, viewCount, elementCount, connectorCount int, description, workspaceHash *string) (*diagv1.WorkspaceVersionInfo, error) {
	var parent *int64
	if parentID != nil {
		value := int64(*parentID)
		parent = &value
	}
	row := &workspaceVersionModel{
		VersionID:       versionID,
		Source:          source,
		ParentVersionID: parent,
		ViewCount:       int64(viewCount),
		ElementCount:    int64(elementCount),
		ConnectorCount:  int64(connectorCount),
		Description:     description,
		WorkspaceHash:   workspaceHash,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	_, err := a.Store.legacy.BunDB().NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return nil, err
	}
	return workspaceVersionToProto(*row, workspaceID)
}

func (a *APIAdapter) GetVersioningEnabled(ctx context.Context, _ uuid.UUID) (bool, error) {
	var row workspaceVersionSettingsModel
	err := a.Store.legacy.BunDB().NewSelect().
		Model(&row).
		Where("id = 1").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return row.CLIVersioningEnabled != 0, err
}

func (a *APIAdapter) SetVersioningEnabled(ctx context.Context, _ uuid.UUID, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	_, err := a.Store.legacy.BunDB().NewInsert().
		Model(&workspaceVersionSettingsModel{ID: 1, CLIVersioningEnabled: value}).
		On("CONFLICT(id) DO UPDATE").
		Set("cli_versioning_enabled = excluded.cli_versioning_enabled").
		Exec(ctx)
	return err
}

func (a *APIAdapter) GetWorkspaceResourceCounts(ctx context.Context, _ uuid.UUID) (views, elements, connectors int, err error) {
	views, err = a.Store.legacy.BunDB().NewSelect().Model((*countModel)(nil)).Count(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	elements, err = a.Store.legacy.BunDB().NewSelect().Model((*elementCountModel)(nil)).Count(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	connectors, err = a.Store.legacy.BunDB().NewSelect().Model((*connectorCountModel)(nil)).Count(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	return views, elements, connectors, nil
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

func workspaceVersionToProto(row workspaceVersionModel, workspaceID uuid.UUID) (*diagv1.WorkspaceVersionInfo, error) {
	createdAt, err := time.Parse(time.RFC3339, row.CreatedAt)
	if err != nil {
		createdAt = time.Now().UTC()
	}
	info := &diagv1.WorkspaceVersionInfo{
		Id:             strconv.FormatInt(row.ID, 10),
		OrgId:          workspaceID.String(),
		VersionId:      row.VersionID,
		Source:         row.Source,
		ViewCount:      int32(row.ViewCount),
		ElementCount:   int32(row.ElementCount),
		ConnectorCount: int32(row.ConnectorCount),
		CreatedAt:      timestamppb.New(createdAt),
	}
	if row.ParentVersionID != nil {
		parent := strconv.FormatInt(*row.ParentVersionID, 10)
		info.ParentVersionId = &parent
	}
	if row.Description != nil {
		info.Description = row.Description
	}
	if row.WorkspaceHash != nil {
		info.WorkspaceHash = row.WorkspaceHash
	}
	return info, nil
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
		Tags:      cloneStrings(node.Tags),
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
	if node.Markdown != nil {
		p.Markdown = viewMarkdownToProto(node.Markdown)
	}
	return p
}

func elementToProto(element app.LibraryElement, workspaceID uuid.UUID) *diagv1.Element {
	p := &diagv1.Element{
		Id:              int32(element.ID),
		OrgId:           workspaceID.String(),
		Name:            element.Name,
		Kind:            element.Kind,
		Tags:            cloneStrings(element.Tags),
		CreatedAt:       ts(element.CreatedAt),
		UpdatedAt:       ts(element.UpdatedAt),
		HasView:         element.HasView,
		ViewLabel:       element.ViewLabel,
		BypassNoiseGate: element.BypassNoiseGate,
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
	if p.LogoUrl == nil {
		p.LogoUrl = derivedTechnologyLogoURL(element.TechnologyConnectors)
	}
	return p
}

func placedElementToProto(item app.PlacedElement) *diagv1.PlacedElement {
	p := &diagv1.PlacedElement{
		Id:              int32(item.ID),
		ViewId:          int32(item.ViewID),
		ElementId:       int32(item.ElementID),
		PositionX:       item.PositionX,
		PositionY:       item.PositionY,
		Name:            item.Name,
		Kind:            item.Kind,
		Tags:            cloneStrings(item.Tags),
		HasView:         item.HasView,
		ViewLabel:       item.ViewLabel,
		BypassNoiseGate: item.BypassNoiseGate,
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
	if p.LogoUrl == nil {
		p.LogoUrl = derivedTechnologyLogoURL(item.TechnologyConnectors)
	}
	return p
}

func derivedTechnologyLogoURL(links []app.TechnologyConnector) *string {
	var fallback string
	for _, link := range links {
		if link.Type != "catalog" || link.Slug == "" {
			continue
		}
		path, ok := tech.IconURLForSlug(link.Slug)
		if !ok {
			continue
		}
		if link.IsPrimaryIcon {
			return &path
		}
		if fallback == "" {
			fallback = path
		}
	}
	if fallback == "" {
		return nil
	}
	return &fallback
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
		Tags:            cloneStrings(connector.Tags),
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
	if links == nil {
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
	if values == nil {
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

func boolValue(value *bool) bool {
	return value != nil && *value
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
