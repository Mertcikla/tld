package app

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type ViewTreeNode struct {
	ID             int64                 `json:"id"`
	OwnerElementID *int64                `json:"owner_element_id"`
	Name           string                `json:"name"`
	Description    *string               `json:"description"`
	LevelLabel     *string               `json:"level_label"`
	Markdown       *ViewMarkdownDocument `json:"markdown,omitempty"`
	Tags           []string              `json:"tags"`
	Level          int                   `json:"level"`
	Depth          int                   `json:"depth"`
	CreatedAt      string                `json:"created_at"`
	UpdatedAt      string                `json:"updated_at"`
	ParentViewID   *int64                `json:"parent_view_id"`
	Children       []ViewTreeNode        `json:"children"`
}

type ViewSummary struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Label     *string  `json:"label"`
	Tags      []string `json:"tags"`
	IsRoot    bool     `json:"is_root"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type ViewConnector struct {
	ID           int64  `json:"id"`
	ElementID    *int64 `json:"element_id"`
	FromViewID   int64  `json:"from_view_id"`
	ToViewID     int64  `json:"to_view_id"`
	ToViewName   string `json:"to_view_name"`
	RelationType string `json:"relation_type"`
}

type IncomingViewConnector struct {
	ID           int64  `json:"id"`
	ElementID    int64  `json:"element_id"`
	ElementName  string `json:"element_name"`
	FromViewID   int64  `json:"from_view_id"`
	FromViewName string `json:"from_view_name"`
	ToViewID     int64  `json:"to_view_id"`
}

type ViewPlacement struct {
	ViewID   int64  `json:"view_id"`
	ViewName string `json:"view_name"`
}

type ViewLayer struct {
	ID        int64    `json:"id"`
	DiagramID int64    `json:"diagram_id"`
	Name      string   `json:"name"`
	Tags      []string `json:"tags"`
	Color     *string  `json:"color,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

func (s *Store) listViewRows(ctx context.Context) ([]viewRow, error) {
	var rows []viewModel
	if err := s.bun.NewSelect().Model(&rows).Order("id").Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]viewRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, viewRowFromModel(row))
	}
	return out, nil
}

func (s *Store) parentViewForOwner(ctx context.Context, ownerElementID int64, excludeViewID int64) (*int64, error) {
	var placement elementPlacementModel
	if err := s.bun.NewSelect().
		Model(&placement).
		Column("view_id").
		Where("element_id = ?", ownerElementID).
		Where("view_id != ?", excludeViewID).
		Order("view_id").
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &placement.ViewID, nil
}

func (s *Store) parentViewMap(ctx context.Context, rows []viewRow) (map[int64]*int64, error) {
	ownerViewIDs := make(map[int64][]int64, len(rows))
	parentMap := make(map[int64]*int64, len(rows))
	for _, row := range rows {
		parentMap[row.ID] = nil
		if row.OwnerElementID.Valid {
			ownerViewIDs[row.OwnerElementID.Int64] = append(ownerViewIDs[row.OwnerElementID.Int64], row.ID)
		}
	}
	if len(ownerViewIDs) == 0 {
		return parentMap, nil
	}

	var placementRows []struct {
		ElementID int64 `bun:"element_id"`
		ViewID    int64 `bun:"view_id"`
	}
	query := s.bun.NewSelect().
		TableExpr("placements AS p").
		Distinct().
		ColumnExpr("p.element_id, p.view_id").
		Join("JOIN views AS v ON v.owner_element_id = p.element_id").
		Order("p.element_id").
		Order("p.view_id")
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		query = query.Where("v.org_id = ?", orgID)
	}
	if err := query.Scan(ctx, &placementRows); err != nil {
		return nil, err
	}
	for _, row := range placementRows {
		for _, childID := range ownerViewIDs[row.ElementID] {
			if row.ViewID == childID || parentMap[childID] != nil {
				continue
			}
			pid := row.ViewID
			parentMap[childID] = &pid
		}
	}
	return parentMap, nil
}

func (s *Store) childViewMeta(ctx context.Context, elementID int64) (bool, *string, error) {
	var row viewModel
	if err := s.bun.NewSelect().
		Model(&row).
		Column("level_label").
		Where("owner_element_id = ?", elementID).
		Order("id").
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil, nil
		}
		return false, nil, err
	}
	if row.LevelLabel != nil {
		return true, row.LevelLabel, nil
	}
	return true, nil, nil
}

type childViewMetaValue struct {
	hasView bool
	label   *string
}

func (s *Store) childViewMetaMap(ctx context.Context) (map[int64]childViewMetaValue, error) {
	var rows []viewModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Column("owner_element_id", "level_label").
		Where("owner_element_id IS NOT NULL").
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := map[int64]childViewMetaValue{}
	for _, row := range rows {
		if row.OwnerElementID == nil {
			continue
		}
		elementID := *row.OwnerElementID
		if _, exists := out[elementID]; exists {
			continue
		}
		meta := childViewMetaValue{hasView: true}
		if row.LevelLabel != nil {
			labelCopy := *row.LevelLabel
			meta.label = &labelCopy
		}
		out[elementID] = meta
	}
	return out, nil
}

func viewNodeFromRow(row viewRow, parentID *int64, depth int, markdown *ViewMarkdownDocument) ViewTreeNode {
	var ownerElementID *int64
	if row.OwnerElementID.Valid {
		value := row.OwnerElementID.Int64
		ownerElementID = &value
	}
	var description *string
	if row.Description.Valid {
		value := row.Description.String
		description = &value
	}
	var levelLabel *string
	if row.LevelLabel.Valid {
		value := row.LevelLabel.String
		levelLabel = &value
	}
	return ViewTreeNode{
		ID:             row.ID,
		OwnerElementID: ownerElementID,
		Name:           row.Name,
		Description:    description,
		LevelLabel:     levelLabel,
		Markdown:       markdown,
		Tags:           parseStrings(row.Tags),
		Level:          row.Level,
		Depth:          depth,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		ParentViewID:   parentID,
		Children:       []ViewTreeNode{},
	}
}

func (s *Store) ViewTree(ctx context.Context) ([]ViewTreeNode, error) {
	rows, err := s.listViewRows(ctx)
	if err != nil {
		return nil, err
	}
	markdownByViewID, err := s.viewMarkdownMap(ctx)
	if err != nil {
		return nil, err
	}
	parentMap, err := s.parentViewMap(ctx, rows)
	if err != nil {
		return nil, err
	}
	rowByID := make(map[int64]viewRow, len(rows))
	byParent := map[int64][]viewRow{}
	var roots []viewRow
	for _, row := range rows {
		rowByID[row.ID] = row
		if parentID := parentMap[row.ID]; parentID != nil {
			byParent[*parentID] = append(byParent[*parentID], row)
			continue
		}
		roots = append(roots, row)
	}
	visited := make(map[int64]bool, len(rows))
	var build func(row viewRow, depth int, stack map[int64]bool) ViewTreeNode
	build = func(row viewRow, depth int, stack map[int64]bool) ViewTreeNode {
		node := viewNodeFromRow(row, parentMap[row.ID], depth, markdownByViewID[row.ID])
		visited[row.ID] = true
		if stack[row.ID] {
			return node
		}
		nextStack := make(map[int64]bool, len(stack)+1)
		maps.Copy(nextStack, stack)
		nextStack[row.ID] = true
		children := byParent[row.ID]
		sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
		for _, child := range children {
			if nextStack[child.ID] {
				continue
			}
			node.Children = append(node.Children, build(child, depth+1, nextStack))
		}
		return node
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	out := make([]ViewTreeNode, 0, len(roots))
	for _, root := range roots {
		out = append(out, build(root, 0, map[int64]bool{}))
	}
	if len(visited) < len(rows) {
		remaining := make([]viewRow, 0, len(rows)-len(visited))
		for _, row := range rows {
			if visited[row.ID] {
				continue
			}
			remaining = append(remaining, rowByID[row.ID])
		}
		sort.Slice(remaining, func(i, j int) bool { return remaining[i].ID < remaining[j].ID })
		for _, row := range remaining {
			if visited[row.ID] {
				continue
			}
			node := build(row, 0, map[int64]bool{})
			node.ParentViewID = nil
			out = append(out, node)
		}
	}
	return out, nil
}

func flattenTree(nodes []ViewTreeNode) []ViewTreeNode {
	var out []ViewTreeNode
	var walk func(items []ViewTreeNode)
	walk = func(items []ViewTreeNode) {
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

func (s *Store) Views(ctx context.Context) ([]ViewSummary, error) {
	tree, err := s.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	flat := flattenTree(tree)
	out := make([]ViewSummary, 0, len(flat))
	for _, node := range flat {
		out = append(out, ViewSummary{
			ID:        node.ID,
			Name:      node.Name,
			Label:     node.LevelLabel,
			Tags:      node.Tags,
			IsRoot:    node.ParentViewID == nil,
			CreatedAt: node.CreatedAt,
			UpdatedAt: node.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Store) ViewByID(ctx context.Context, id int64) (ViewTreeNode, error) {
	var model viewModel
	if err := s.bun.NewSelect().Model(&model).Where("id = ?", id).Scan(ctx); err != nil {
		return ViewTreeNode{}, err
	}
	view := viewRowFromModel(model)
	var parentID *int64
	var err error
	if view.OwnerElementID.Valid {
		parentID, err = s.parentViewForOwner(ctx, view.OwnerElementID.Int64, view.ID)
		if err != nil {
			return ViewTreeNode{}, err
		}
	}
	markdown, err := s.ViewMarkdownByViewID(ctx, view.ID)
	if err != nil {
		return ViewTreeNode{}, err
	}
	return viewNodeFromRow(view, parentID, 0, markdown), nil
}

func (s *Store) ChildViews(ctx context.Context, parentViewID int64) ([]ViewTreeNode, error) {
	markdownByViewID, err := s.viewMarkdownMap(ctx)
	if err != nil {
		return nil, err
	}
	var rows []viewModel
	query := s.bun.NewSelect().
		Model(&rows).
		ModelTableExpr("views AS v").
		Distinct().
		ColumnExpr("v.id, v.owner_element_id, v.name, v.description, v.level_label, v.tags, v.level, v.created_at, v.updated_at").
		Join("JOIN placements AS p ON p.element_id = v.owner_element_id").
		Where("p.view_id = ?", parentViewID).
		Where("v.id != ?", parentViewID).
		Order("v.id")
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	out := []ViewTreeNode{}
	for _, model := range rows {
		row := viewRowFromModel(model)
		parentID := parentViewID
		out = append(out, viewNodeFromRow(row, &parentID, 0, markdownByViewID[row.ID]))
	}
	return out, nil
}

func (s *Store) RootViews(ctx context.Context) ([]ViewTreeNode, error) {
	markdownByViewID, err := s.viewMarkdownMap(ctx)
	if err != nil {
		return nil, err
	}
	var rows []viewModel
	query := s.bun.NewSelect().
		Model(&rows).
		ModelTableExpr("views AS v").
		ColumnExpr("v.id, v.owner_element_id, v.name, v.description, v.level_label, v.tags, v.level, v.created_at, v.updated_at").
		Where("v.owner_element_id IS NULL OR NOT EXISTS (SELECT 1 FROM placements p WHERE p.element_id = v.owner_element_id AND p.view_id != v.id)").
		Order("v.id")
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	out := []ViewTreeNode{}
	for _, model := range rows {
		out = append(out, viewNodeFromRow(viewRowFromModel(model), nil, 0, markdownByViewID[model.ID]))
	}
	return out, nil
}

func (s *Store) CreateView(ctx context.Context, name string, levelLabel *string, ownerElementID *int64) (ViewSummary, error) {
	now := nowString()
	level := 1
	if ownerElementID != nil {
		parentID, err := s.parentViewForOwner(ctx, *ownerElementID, 0)
		if err == nil && parentID != nil {
			parent, err := s.ViewByID(ctx, *parentID)
			if err == nil {
				level = parent.Level + 1
			}
		}
	}
	row := &viewModel{OwnerElementID: ownerElementID, Name: strings.TrimSpace(name), LevelLabel: levelLabel, Level: level, CreatedAt: now, UpdatedAt: now}
	_, err := s.bun.NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return ViewSummary{}, err
	}
	view, err := s.ViewByID(ctx, row.ID)
	if err != nil {
		return ViewSummary{}, err
	}
	return ViewSummary{
		ID:        view.ID,
		Name:      view.Name,
		Label:     view.LevelLabel,
		Tags:      view.Tags,
		IsRoot:    view.ParentViewID == nil,
		CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt,
	}, nil
}

func (s *Store) UpdateView(ctx context.Context, id int64, name *string, description *string, levelLabel *string, tags []string) (ViewSummary, error) {
	if tags != nil {
		if err := s.ensureTagColors(ctx, tags); err != nil {
			return ViewSummary{}, err
		}
	}
	q := s.bun.NewUpdate().
		Model((*viewModel)(nil)).
		Set("level_label = ?", levelLabel).
		Set("updated_at = ?", nowString()).
		Where("id = ?", id)
	if name != nil && strings.TrimSpace(*name) != "" {
		q = q.Set("name = ?", strings.TrimSpace(*name))
	}
	if description != nil {
		q = q.Set("description = ?", description)
	}
	if tags != nil {
		q = q.Set("tags = ?", jsonString(tags, "[]"))
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return ViewSummary{}, err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ViewSummary{}, sql.ErrNoRows
	}
	updated, err := s.ViewByID(ctx, id)
	if err != nil {
		return ViewSummary{}, err
	}
	return ViewSummary{
		ID:        updated.ID,
		Name:      updated.Name,
		Label:     updated.LevelLabel,
		Tags:      updated.Tags,
		IsRoot:    updated.ParentViewID == nil,
		CreatedAt: updated.CreatedAt,
		UpdatedAt: updated.UpdatedAt,
	}, nil
}

func (s *Store) SetViewLevel(ctx context.Context, id int64, level int) error {
	_, err := s.bun.NewUpdate().
		Model((*viewModel)(nil)).
		Set("level = ?", level).
		Set("updated_at = ?", nowString()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *Store) DeleteView(ctx context.Context, id int64) error {
	_, err := s.bun.NewDelete().Model((*viewModel)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) ListIncomingNavigations(ctx context.Context, viewID int64) ([]IncomingViewConnector, error) {
	view, err := s.ViewByID(ctx, viewID)
	if err != nil {
		return nil, err
	}
	if view.OwnerElementID == nil || view.ParentViewID == nil {
		return []IncomingViewConnector{}, nil
	}
	element, err := s.ElementByID(ctx, *view.OwnerElementID)
	if err != nil {
		return nil, err
	}
	parent, err := s.ViewByID(ctx, *view.ParentViewID)
	if err != nil {
		return nil, err
	}
	return []IncomingViewConnector{{
		ID:           0,
		ElementID:    *view.OwnerElementID,
		ElementName:  element.Name,
		FromViewID:   parent.ID,
		FromViewName: parent.Name,
		ToViewID:     view.ID,
	}}, nil
}
