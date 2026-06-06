// Package api provides the public APIStore adapter that wraps *app.Store
// to satisfy the api.Store interface. Wrappers can embed this adapter
// and override specific methods for multi-tenant or SaaS-specific behavior.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// APIStore wraps *app.Store to implement api.Store for both SQLite and PostgreSQL.
// SaaS wrappers can embed this struct and override methods that need multi-tenant
// awareness (tags, view layers, markdown, versioning).
type APIStore struct {
	Store *app.Store
	sqlDB *sql.DB
	bunDB *bun.DB
}

// NewAPIStore creates a public api.Store implementation backed by *app.Store.
func NewAPIStore(store *app.Store) *APIStore {
	return &APIStore{
		Store: store,
		sqlDB: store.DB(),
		bunDB: store.BunDB(),
	}
}

// BunDB exposes the underlying Bun database for operations that need raw queries.
func (a *APIStore) BunDB() *bun.DB { return a.bunDB }

// ─── Views ───────────────────────────────────────────────────────────────────

func (a *APIStore) ListViews(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.View, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	flat := flattenViewTreeNodes(nodes)
	out := make([]*diagv1.View, 0, len(flat))
	for _, node := range flat {
		out = append(out, viewNodeToProto(node, workspaceID))
	}
	return out, nil
}

func (a *APIStore) GetViews(ctx context.Context, workspaceID uuid.UUID, parentViewID *int32, isRoot *bool, search string, limit, offset int) ([]*diagv1.View, int, error) {
	var flat []app.ViewTreeNode
	switch {
	case parentViewID != nil:
		nodes, err := a.Store.ChildViews(ctx, int64(*parentViewID))
		if err != nil {
			return nil, 0, err
		}
		flat = nodes
	case isRoot != nil && *isRoot:
		nodes, err := a.Store.RootViews(ctx)
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
			if (node.ParentViewID == nil) != *isRoot {
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
	for _, node := range filtered {
		out = append(out, viewNodeToProto(node, workspaceID))
	}
	return out, total, nil
}

func (a *APIStore) GetView(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.View, error) {
	view, err := a.Store.ViewByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return viewNodeToProto(view, workspaceID), nil
}

func (a *APIStore) CreateView(ctx context.Context, workspaceID uuid.UUID, ownerElementID *int32, name string, label *string, isRoot bool) (*diagv1.View, error) {
	var ownerID *int64
	if ownerElementID != nil {
		v := int64(*ownerElementID)
		ownerID = &v
	}
	view, err := a.Store.CreateView(ctx, name, label, ownerID)
	if err != nil {
		return nil, err
	}
	return a.GetView(ctx, int32(view.ID), workspaceID)
}

func (a *APIStore) UpdateView(ctx context.Context, id int32, workspaceID uuid.UUID, name string, description *string, label *string, tags []string) (*diagv1.View, error) {
	nameCopy := name
	_, err := a.Store.UpdateView(ctx, int64(id), &nameCopy, description, label, tags)
	if err != nil {
		return nil, err
	}
	return a.GetView(ctx, id, workspaceID)
}

func (a *APIStore) DeleteView(ctx context.Context, id int32, workspaceID uuid.UUID) error {
	return a.Store.DeleteView(ctx, int64(id))
}

// ─── Elements ────────────────────────────────────────────────────────────────

func (a *APIStore) ListElements(ctx context.Context, workspaceID uuid.UUID, limit, offset int32, search string) ([]*diagv1.Element, int, error) {
	elements, total, err := a.Store.Elements(ctx, int(limit), int(offset), search)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*diagv1.Element, 0, len(elements))
	for _, element := range elements {
		out = append(out, elementToProto(element, workspaceID))
	}
	return out, total, nil
}

func (a *APIStore) GetElement(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.Element, error) {
	element, err := a.Store.ElementByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return elementToProto(element, workspaceID), nil
}

func (a *APIStore) CreateElement(ctx context.Context, workspaceID uuid.UUID, input ElementInput) (*diagv1.Element, error) {
	techLinks := technologyLinksToConnectors(input.TechLinks)
	if techLinks == nil {
		techLinks = []app.TechnologyConnector{}
	}
	element, err := a.Store.CreateElement(ctx, app.LibraryElement{
		Name:                 input.Name,
		Description:          input.Description,
		Kind:                 input.Kind,
		Technology:           input.Technology,
		URL:                  input.URL,
		LogoURL:              input.LogoURL,
		TechnologyConnectors: techLinks,
		Tags:                 input.Tags,
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
	return elementToProto(element, workspaceID), nil
}

func (a *APIStore) UpdateElement(ctx context.Context, id int32, workspaceID uuid.UUID, input ElementInput) (*diagv1.Element, error) {
	element, err := a.Store.UpdateElement(ctx, int64(id), app.LibraryElement{
		Name:                 input.Name,
		Description:          input.Description,
		Kind:                 input.Kind,
		Technology:           input.Technology,
		URL:                  input.URL,
		LogoURL:              input.LogoURL,
		TechnologyConnectors: technologyLinksToConnectors(input.TechLinks),
		Tags:                 input.Tags,
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
	return elementToProto(element, workspaceID), nil
}

func (a *APIStore) DeleteElement(ctx context.Context, id int32, workspaceID uuid.UUID) error {
	return a.Store.DeleteElement(ctx, int64(id))
}

// ─── Placements ──────────────────────────────────────────────────────────────

func (a *APIStore) ListPlacements(ctx context.Context, viewID int32) ([]*diagv1.PlacedElement, error) {
	elems, err := a.Store.Placements(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.PlacedElement, 0, len(elems))
	for _, pe := range elems {
		out = append(out, placedElementToProto(pe))
	}
	return out, nil
}

func (a *APIStore) ListAllPlacements(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.PlacedElement, error) {
	elems, err := a.Store.AllPlacements(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.PlacedElement, 0, len(elems))
	for _, pe := range elems {
		out = append(out, placedElementToProto(pe))
	}
	return out, nil
}

func (a *APIStore) ListElementPlacements(ctx context.Context, elementID int32, workspaceID uuid.UUID) ([]*diagv1.ViewPlacementInfo, error) {
	placements, err := a.Store.ListElementPlacements(ctx, int64(elementID))
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

func (a *APIStore) AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error) {
	_, err := a.Store.AddPlacement(ctx, int64(viewID), int64(elementID), x, y)
	if err != nil {
		return nil, err
	}
	placements, err := a.Store.Placements(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	for i := range placements {
		if placements[i].ElementID == int64(elementID) {
			return placedElementToProto(placements[i]), nil
		}
	}
	return nil, fmt.Errorf("placement %d/%d not found after insert", viewID, elementID)
}

func (a *APIStore) UpdatePlacementPosition(ctx context.Context, viewID, elementID int32, x, y float64) error {
	return a.Store.UpdatePlacement(ctx, int64(viewID), int64(elementID), x, y)
}

func (a *APIStore) RemovePlacement(ctx context.Context, viewID, elementID int32) error {
	return a.Store.DeletePlacement(ctx, int64(viewID), int64(elementID))
}

// ─── Connectors ──────────────────────────────────────────────────────────────

func (a *APIStore) ListConnectors(ctx context.Context, viewID int32, workspaceID uuid.UUID) ([]*diagv1.Connector, error) {
	connectors, err := a.Store.Connectors(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.Connector, 0, len(connectors))
	for _, c := range connectors {
		out = append(out, connectorToProto(c))
	}
	return out, nil
}

func (a *APIStore) ListAllConnectors(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.Connector, error) {
	connectors, err := a.Store.AllConnectors(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.Connector, 0, len(connectors))
	for _, c := range connectors {
		out = append(out, connectorToProto(c))
	}
	return out, nil
}

func (a *APIStore) GetConnector(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.Connector, error) {
	connector, err := a.Store.ConnectorByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIStore) CreateConnector(ctx context.Context, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
	connector, err := a.Store.CreateConnector(ctx, app.Connector{
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
		Tags:            input.Tags,
	})
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIStore) UpdateConnector(ctx context.Context, id int32, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
	connector, err := a.Store.UpdateConnector(ctx, int64(id), app.Connector{
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
		Tags:            input.Tags,
	})
	if err != nil {
		return nil, err
	}
	return connectorToProto(connector), nil
}

func (a *APIStore) DeleteConnector(ctx context.Context, id int32, workspaceID uuid.UUID) error {
	return a.Store.DeleteConnector(ctx, int64(id))
}

// ─── Navigations ─────────────────────────────────────────────────────────────

func (a *APIStore) ListElementNavigations(ctx context.Context, workspaceID uuid.UUID, elementID int32) ([]*diagv1.ElementNavigationInfo, error) {
	navs, err := a.Store.ListElementNavigations(ctx, int64(elementID), nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.ElementNavigationInfo, 0, len(navs))
	for _, nav := range navs {
		out = append(out, navigationToProto(nav))
	}
	return out, nil
}

func (a *APIStore) ListIncomingElementNavigations(ctx context.Context, viewID int32) ([]*diagv1.IncomingElementNavigationInfo, error) {
	navs, err := a.Store.ListIncomingNavigations(ctx, int64(viewID))
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

// ─── View Layers ─────────────────────────────────────────────────────────────

func (a *APIStore) ListViewLayers(ctx context.Context, viewID int32) ([]*diagv1.ViewLayer, error) {
	layers, err := a.Store.Layers(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	out := make([]*diagv1.ViewLayer, 0, len(layers))
	for _, layer := range layers {
		out = append(out, layerToProto(layer))
	}
	return out, nil
}

func (a *APIStore) ListAllViewLayers(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.ViewLayer, error) {
	nodes, err := a.Store.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	layers, err := a.Store.AllLayers(ctx)
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

func (a *APIStore) GetViewLayer(ctx context.Context, id int32) (*diagv1.ViewLayer, error) {
	layer, err := a.Store.LayerByID(ctx, int64(id))
	if err != nil {
		return nil, err
	}
	return layerToProto(layer), nil
}

func (a *APIStore) CreateViewLayer(ctx context.Context, viewID int32, name string, tags []string, color string) (*diagv1.ViewLayer, error) {
	layer, err := a.Store.CreateLayer(ctx, int64(viewID), name, tags, &color)
	if err != nil {
		return nil, err
	}
	return layerToProto(layer), nil
}

func (a *APIStore) UpdateViewLayer(ctx context.Context, id int32, name *string, tags []string, color *string) (*diagv1.ViewLayer, error) {
	var layerName string
	if name != nil {
		layerName = *name
	}
	layer, err := a.Store.UpdateLayer(ctx, int64(id), app.ViewLayer{
		Name:  layerName,
		Tags:  tags,
		Color: color,
	})
	if err != nil {
		return nil, err
	}
	return layerToProto(layer), nil
}

func (a *APIStore) DeleteViewLayer(ctx context.Context, id int32) error {
	return a.Store.DeleteLayer(ctx, int64(id))
}

// ─── Tags ────────────────────────────────────────────────────────────────────

func (a *APIStore) Tags(ctx context.Context, workspaceID uuid.UUID) (map[string]*diagv1.Tag, error) {
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

func (a *APIStore) UpdateTag(ctx context.Context, workspaceID uuid.UUID, name, color string, description *string) error {
	return a.Store.UpdateTag(ctx, name, color, description)
}

func (a *APIStore) DeleteTag(ctx context.Context, workspaceID uuid.UUID, name string) error {
	return a.Store.DeleteTag(ctx, name)
}

func apiCollaborationWorkspaceKey(workspaceID uuid.UUID) string {
	if workspaceID == uuid.Nil {
		return "local"
	}
	return workspaceID.String()
}

func apiNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func apiParseStoredTime(value string) *timestamppb.Timestamp {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return timestamppb.New(time.Now().UTC())
	}
	return timestamppb.New(parsed)
}

func (a *APIStore) ListViewThreads(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32) ([]*diagv1.ThreadInfo, error) {
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
	err := a.bunDB.NewRaw(
		`SELECT t.id, t.workspace_id, t.view_id, t.element_id, t.connector_id, t.created_by,
		        COALESCE(NULLIF(t.created_by_username, ''), t.created_by) AS created_by_username,
		        t.status, t.created_at, t.resolved_at
		 FROM view_threads t
		 WHERE `+where+`
		 ORDER BY t.created_at ASC`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, targetID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	threads := make([]*diagv1.ThreadInfo, 0, len(rows))
	for _, row := range rows {
		thread := apiThreadRowToProto(row.ID, row.WorkspaceID, row.ViewID, row.ElementID, row.ConnectorID, row.CreatedBy, row.CreatedByUsername, row.Status, row.CreatedAt, row.ResolvedAt)
		comments, err := a.apiListViewComments(ctx, workspaceID, viewID, row.ID)
		if err != nil {
			return nil, err
		}
		thread.Comments = comments
		threads = append(threads, thread)
	}
	return threads, nil
}

func (a *APIStore) CreateViewThread(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32, createdBy, createdByUsername string) (*diagv1.ThreadInfo, error) {
	var row struct {
		ID int32 `bun:"id"`
	}
	err := a.bunDB.NewRaw(
		`INSERT INTO view_threads (workspace_id, view_id, element_id, connector_id, created_by, created_by_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'open', ?)
		 RETURNING id`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, elementID, connectorID, createdBy, createdByUsername, apiNowRFC3339(),
	).Scan(ctx, &row)
	if err != nil {
		return nil, err
	}
	return a.GetViewThread(ctx, workspaceID, viewID, row.ID)
}

func (a *APIStore) GetViewThread(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) (*diagv1.ThreadInfo, error) {
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
	err := a.bunDB.NewRaw(
		`SELECT t.id, t.workspace_id, t.view_id, t.element_id, t.connector_id, t.created_by,
		        COALESCE(NULLIF(t.created_by_username, ''), t.created_by) AS created_by_username,
		        t.status, t.created_at, t.resolved_at
		 FROM view_threads t
		 WHERE t.workspace_id = ? AND t.view_id = ? AND t.id = ?`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, threadID,
	).Scan(ctx, &row)
	if err != nil {
		return nil, err
	}
	thread := apiThreadRowToProto(row.ID, row.WorkspaceID, row.ViewID, row.ElementID, row.ConnectorID, row.CreatedBy, row.CreatedByUsername, row.Status, row.CreatedAt, row.ResolvedAt)
	comments, err := a.apiListViewComments(ctx, workspaceID, viewID, threadID)
	if err != nil {
		return nil, err
	}
	thread.Comments = comments
	return thread, nil
}

func (a *APIStore) SetViewThreadResolved(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, resolved bool) error {
	status := "open"
	var resolvedAt *string
	if resolved {
		status = "resolved"
		now := apiNowRFC3339()
		resolvedAt = &now
	}
	_, err := a.bunDB.NewRaw(
		`UPDATE view_threads
		 SET status = ?, resolved_at = ?
		 WHERE workspace_id = ? AND view_id = ? AND id = ?`,
		status, resolvedAt, apiCollaborationWorkspaceKey(workspaceID), viewID, threadID,
	).Exec(ctx)
	return err
}

func (a *APIStore) CreateViewComment(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, authorID, authorUsername, body string) (*diagv1.CommentInfo, error) {
	now := apiNowRFC3339()
	var row struct {
		ID int32 `bun:"id"`
	}
	err := a.bunDB.NewRaw(
		`INSERT INTO view_comments (workspace_id, view_id, thread_id, author_id, author_username, body, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, threadID, authorID, authorUsername, body, now, now,
	).Scan(ctx, &row)
	if err != nil {
		return nil, err
	}
	comments, err := a.apiListViewComments(ctx, workspaceID, viewID, threadID)
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

func (a *APIStore) apiListViewComments(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) ([]*diagv1.CommentInfo, error) {
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
	err := a.bunDB.NewRaw(
		`SELECT c.id, c.workspace_id, c.view_id, c.thread_id, c.author_id,
		        COALESCE(NULLIF(c.author_username, ''), c.author_id) AS author_username,
		        c.body, c.created_at, c.updated_at
		 FROM view_comments c
		 WHERE c.workspace_id = ? AND c.view_id = ? AND c.thread_id = ?
		 ORDER BY c.created_at ASC`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, threadID,
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
			CreatedAt:      apiParseStoredTime(row.CreatedAt),
			UpdatedAt:      apiParseStoredTime(row.UpdatedAt),
		})
	}
	return out, nil
}

func apiThreadRowToProto(id int32, workspaceID string, viewID int32, elementID, connectorID *int32, createdBy, createdByUsername, status, createdAt string, resolvedAt *string) *diagv1.ThreadInfo {
	thread := &diagv1.ThreadInfo{
		Id:                id,
		OrgId:             workspaceID,
		ViewId:            viewID,
		ElementId:         elementID,
		ConnectorId:       connectorID,
		CreatedBy:         createdBy,
		CreatedByUsername: createdByUsername,
		Status:            status,
		CreatedAt:         apiParseStoredTime(createdAt),
		Comments:          []*diagv1.CommentInfo{},
	}
	if resolvedAt != nil {
		thread.ResolvedAt = apiParseStoredTime(*resolvedAt)
	}
	return thread
}

func (a *APIStore) ListViewElementReactions(ctx context.Context, workspaceID uuid.UUID, viewID int32, userID string) ([]*diagv1.NodeReactionSummary, error) {
	var rows []struct {
		ElementID   int32  `bun:"element_id"`
		Emoji       string `bun:"emoji"`
		Count       int32  `bun:"reaction_count"`
		ReactedByMe int32  `bun:"reacted_by_me"`
	}
	err := a.bunDB.NewRaw(
		`SELECT element_id, emoji, COUNT(*) AS reaction_count,
		        MAX(CASE WHEN user_id = ? THEN 1 ELSE 0 END) AS reacted_by_me
		 FROM element_reactions
		 WHERE workspace_id = ? AND view_id = ?
		 GROUP BY element_id, emoji
		 ORDER BY element_id ASC, emoji ASC`,
		userID, apiCollaborationWorkspaceKey(workspaceID), viewID,
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

func (a *APIStore) ToggleElementReaction(ctx context.Context, workspaceID uuid.UUID, viewID, elementID int32, userID, emoji string) (bool, error) {
	result, err := a.bunDB.NewRaw(
		`DELETE FROM element_reactions
		 WHERE workspace_id = ? AND view_id = ? AND element_id = ? AND user_id = ? AND emoji = ?`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, elementID, userID, emoji,
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return false, nil
	}
	_, err = a.bunDB.NewRaw(
		`INSERT INTO element_reactions (workspace_id, view_id, element_id, user_id, emoji, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, elementID, userID, emoji, apiNowRFC3339(),
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *APIStore) ListDrawings(ctx context.Context, workspaceID uuid.UUID, viewID int32) ([]RealtimeDrawingInput, error) {
	var rows []struct {
		PathID   string  `bun:"path_id"`
		UserID   string  `bun:"user_id"`
		Points   string  `bun:"points"`
		Color    string  `bun:"color"`
		Width    float64 `bun:"width"`
		Text     string  `bun:"text"`
		FontSize float64 `bun:"font_size"`
	}
	err := a.bunDB.NewRaw(
		`SELECT path_id, user_id, points, color, width, text, font_size
		 FROM drawings
		 WHERE workspace_id = ? AND view_id = ?
		 ORDER BY created_at ASC`,
		apiCollaborationWorkspaceKey(workspaceID), viewID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]RealtimeDrawingInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, RealtimeDrawingInput{
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

func (a *APIStore) UpsertDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, input RealtimeDrawingInput) error {
	now := apiNowRFC3339()
	_, err := a.bunDB.NewRaw(
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
		apiCollaborationWorkspaceKey(workspaceID), viewID, input.UserID, input.PathID, string(input.Points), input.Color, input.Width, input.Text, input.FontSize, now, now,
	).Exec(ctx)
	return err
}

func (a *APIStore) DeleteDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, pathID string) error {
	_, err := a.bunDB.NewRaw(
		`DELETE FROM drawings WHERE workspace_id = ? AND view_id = ? AND path_id = ?`,
		apiCollaborationWorkspaceKey(workspaceID), viewID, pathID,
	).Exec(ctx)
	return err
}

// ─── View Markdown ───────────────────────────────────────────────────────────
// Default implementation uses app.Store's view markdown methods.
// Wrappers with different markdown storage (e.g., DB-based with content column)
// should override these methods.

func (a *APIStore) GetViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID) (*diagv1.ViewMarkdownDocument, string, error) {
	doc, err := a.Store.ViewMarkdownByViewID(ctx, int64(viewID))
	if err != nil {
		return nil, "", err
	}
	return &diagv1.ViewMarkdownDocument{
		Path:      doc.Path,
		IsManaged: doc.IsManaged,
		UpdatedAt: ts(doc.UpdatedAt),
	}, "", nil
}

func (a *APIStore) CreateViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, fileName *string, initialContent *string) (*diagv1.View, error) {
	path := defaultViewMarkdownPath(viewID, fileName)
	if err := a.Store.UpsertViewMarkdown(ctx, int64(viewID), path, true, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return a.GetView(ctx, viewID, workspaceID)
}

func (a *APIStore) LinkViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, path string) (*diagv1.View, error) {
	if err := a.Store.UpsertViewMarkdown(ctx, int64(viewID), path, false, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return a.GetView(ctx, viewID, workspaceID)
}

func (a *APIStore) SaveViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, content string) (*diagv1.ViewMarkdownDocument, error) {
	return nil, ErrUnimplemented
}

func (a *APIStore) UnlinkViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, deleteManagedFile bool) (*diagv1.View, error) {
	if err := a.Store.DeleteViewMarkdown(ctx, int64(viewID)); err != nil {
		return nil, err
	}
	return a.GetView(ctx, viewID, workspaceID)
}

// ─── Projected View Content ──────────────────────────────────────────────────

func (a *APIStore) GetProjectedViewContent(ctx context.Context, viewID int32, workspaceID uuid.UUID, densityOverride *int32) (*diagv1.ViewContent, error) {
	var override *int
	if densityOverride != nil {
		val := int(*densityOverride)
		override = &val
	}
	content, err := a.Store.GetProjectedViewContent(ctx, int64(viewID), override)
	if err != nil {
		return nil, err
	}
	out := &diagv1.ViewContent{
		Placements: make([]*diagv1.PlacedElement, 0, len(content.Placements)),
		Connectors: make([]*diagv1.Connector, 0, len(content.Connectors)),
	}
	for _, pe := range content.Placements {
		out.Placements = append(out.Placements, placedElementToProto(pe))
	}
	for _, c := range content.Connectors {
		out.Connectors = append(out.Connectors, connectorToProto(c))
	}
	return out, nil
}

// ─── ApplyPlan ───────────────────────────────────────────────────────────────
// Subset of ApplyPlan. Wrappers should override for full implementation.

func (a *APIStore) ApplyPlan(ctx context.Context, workspaceID uuid.UUID, req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
	return nil, ErrUnimplemented
}

// ─── Versioning ──────────────────────────────────────────────────────────────

var _ = []any{
	// compile-time check
	(*APIStore)(nil),
}

type workspaceVersionModel struct {
	bun.BaseModel `bun:"table:workspace_versions"`

	ID              int64      `bun:"id,pk,autoincrement"`
	OrgID           *uuid.UUID `bun:"org_id,nullzero"`
	VersionID       string     `bun:"version_id"`
	Source          string     `bun:"source"`
	ParentVersionID *int64     `bun:"parent_version_id"`
	ViewCount       int64      `bun:"view_count"`
	ElementCount    int64      `bun:"element_count"`
	ConnectorCount  int64      `bun:"connector_count"`
	Description     *string    `bun:"description"`
	WorkspaceHash   *string    `bun:"workspace_hash"`
	CreatedAt       string     `bun:"created_at"`
}

func (m *workspaceVersionModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	orgID := app.TenantOrgIDFromCtx(ctx)
	if orgID == uuid.Nil {
		return nil
	}
	if _, ok := query.(*bun.InsertQuery); ok && m != nil && m.OrgID == nil {
		m.OrgID = &orgID
	}
	return nil
}

func (m *workspaceVersionModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyAPIStoreTenantWhere(ctx, query)
}

func (m *workspaceVersionModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyAPIStoreTenantWhere(ctx, query)
}

func (m *workspaceVersionModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyAPIStoreTenantWhere(ctx, query)
}

type workspaceVersionSettingsModel struct {
	bun.BaseModel `bun:"table:workspace_version_settings"`

	ID                   int `bun:"id,pk"`
	CLIVersioningEnabled int `bun:"cli_versioning_enabled"`
}

type apiStoreViewCountModel struct {
	bun.BaseModel `bun:"table:views"`

	ID int64 `bun:"id,pk"`
}

func (m *apiStoreViewCountModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyAPIStoreTenantWhere(ctx, query)
}

type apiStoreElementCountModel struct {
	bun.BaseModel `bun:"table:elements"`

	ID int64 `bun:"id,pk"`
}

func (m *apiStoreElementCountModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyAPIStoreTenantWhere(ctx, query)
}

type apiStoreConnectorCountModel struct {
	bun.BaseModel `bun:"table:connectors"`

	ID int64 `bun:"id,pk"`
}

func (m *apiStoreConnectorCountModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyAPIStoreTenantWhere(ctx, query)
}

func (a *APIStore) ListVersions(ctx context.Context, workspaceID uuid.UUID, limit int) ([]*diagv1.WorkspaceVersionInfo, error) {
	ctx = app.WithTenantOrgID(ctx, workspaceID)
	if limit <= 0 {
		limit = 50
	}
	var rows []workspaceVersionModel
	if err := a.bunDB.NewSelect().Model(&rows).Order("id DESC").Limit(limit).Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*diagv1.WorkspaceVersionInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspaceVersionToProto(row, workspaceID))
	}
	return out, nil
}

func (a *APIStore) GetLatestVersion(ctx context.Context, workspaceID uuid.UUID) (*diagv1.WorkspaceVersionInfo, error) {
	ctx = app.WithTenantOrgID(ctx, workspaceID)
	var row workspaceVersionModel
	if err := a.bunDB.NewSelect().Model(&row).Order("id DESC").Limit(1).Scan(ctx); err != nil {
		return nil, ErrUnimplemented
	}
	return workspaceVersionToProto(row, workspaceID), nil
}

func (a *APIStore) CreateVersion(ctx context.Context, workspaceID uuid.UUID, versionID, source string, parentID *int32, viewCount, elementCount, connectorCount int, description, workspaceHash *string) (*diagv1.WorkspaceVersionInfo, error) {
	ctx = app.WithTenantOrgID(ctx, workspaceID)
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
	if _, err := a.bunDB.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return workspaceVersionToProto(*row, workspaceID), nil
}

func (a *APIStore) GetVersioningEnabled(ctx context.Context, workspaceID uuid.UUID) (bool, error) {
	var row workspaceVersionSettingsModel
	if err := a.bunDB.NewSelect().Model(&row).Column("cli_versioning_enabled").Where("id = 1").Scan(ctx); err != nil {
		return true, nil
	}
	return row.CLIVersioningEnabled == 1, nil
}

func (a *APIStore) SetVersioningEnabled(ctx context.Context, workspaceID uuid.UUID, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	_, err := a.bunDB.NewInsert().Model(&workspaceVersionSettingsModel{ID: 1, CLIVersioningEnabled: value}).
		On("CONFLICT(id) DO UPDATE").
		Set("cli_versioning_enabled = excluded.cli_versioning_enabled").
		Exec(ctx)
	return err
}

func (a *APIStore) GetWorkspaceResourceCounts(ctx context.Context, workspaceID uuid.UUID) (views, elements, connectors int, err error) {
	ctx = app.WithTenantOrgID(ctx, workspaceID)
	viewQuery := a.bunDB.NewSelect().Model((*apiStoreViewCountModel)(nil))
	if workspaceID != uuid.Nil {
		viewQuery = viewQuery.Where("org_id = ?", workspaceID)
	}
	views, err = viewQuery.Count(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	elementQuery := a.bunDB.NewSelect().Model((*apiStoreElementCountModel)(nil))
	if workspaceID != uuid.Nil {
		elementQuery = elementQuery.Where("org_id = ?", workspaceID)
	}
	elements, err = elementQuery.Count(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	connectorQuery := a.bunDB.NewSelect().Model((*apiStoreConnectorCountModel)(nil))
	if workspaceID != uuid.Nil {
		connectorQuery = connectorQuery.Where("org_id = ?", workspaceID)
	}
	connectors, err = connectorQuery.Count(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	return views, elements, connectors, nil
}

func applyAPIStoreTenantWhere(ctx context.Context, query any) error {
	orgID := app.TenantOrgIDFromCtx(ctx)
	if orgID == uuid.Nil {
		return nil
	}
	switch q := query.(type) {
	case *bun.SelectQuery:
		q.Where("org_id = ?", orgID)
	case *bun.UpdateQuery:
		q.Where("org_id = ?", orgID)
	case *bun.DeleteQuery:
		q.Where("org_id = ?", orgID)
	}
	return nil
}

// ─── Conversion helpers ──────────────────────────────────────────────────────

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
		Tags:      node.Tags,
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
		p.Markdown = &diagv1.ViewMarkdownDocument{
			Path:      node.Markdown.Path,
			IsManaged: node.Markdown.IsManaged,
			UpdatedAt: ts(node.Markdown.UpdatedAt),
		}
	}
	return p
}

func elementToProto(element app.LibraryElement, workspaceID uuid.UUID) *diagv1.Element {
	p := &diagv1.Element{
		Id:              int32(element.ID),
		OrgId:           workspaceID.String(),
		Name:            element.Name,
		Kind:            element.Kind,
		Tags:            element.Tags,
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
		Tags:            item.Tags,
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
		Tags:            connector.Tags,
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
		Tags:      layer.Tags,
		Color:     color,
		CreatedAt: ts(layer.CreatedAt),
		UpdatedAt: ts(layer.UpdatedAt),
	}
}

func technologyLinksToConnectors(links []*diagv1.TechnologyLink) []app.TechnologyConnector {
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

func workspaceVersionToProto(row workspaceVersionModel, workspaceID uuid.UUID) *diagv1.WorkspaceVersionInfo {
	createdAt, _ := time.Parse(time.RFC3339, row.CreatedAt)
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
	return info
}

func defaultViewMarkdownPath(viewID int32, fileName *string) string {
	name := "notes"
	if fileName != nil {
		name = *fileName
	}
	return fmt.Sprintf("view-markdown/view-%d-%s.md", viewID, name)
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

func boolValue(value *bool) bool {
	return value != nil && *value
}

func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := range len(s) - len(substr) + 1 {
		if equalFoldASCII(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
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
