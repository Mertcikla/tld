package app

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"sort"
	"strings"
)

type ViewTreeNode struct {
	ID             int64          `json:"id"`
	OwnerElementID *int64         `json:"owner_element_id"`
	Name           string         `json:"name"`
	Description    *string        `json:"description"`
	LevelLabel     *string        `json:"level_label"`
	Level          int            `json:"level"`
	Depth          int            `json:"depth"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	ParentViewID   *int64         `json:"parent_view_id"`
	Children       []ViewTreeNode `json:"children"`
}

type ViewSummary struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Label     *string `json:"label"`
	IsRoot    bool    `json:"is_root"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, owner_element_id, name, description, level_label, level, created_at, updated_at FROM views ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []viewRow
	for rows.Next() {
		var row viewRow
		if err := rows.Scan(&row.ID, &row.OwnerElementID, &row.Name, &row.Description, &row.LevelLabel, &row.Level, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) parentViewForOwner(ctx context.Context, ownerElementID int64, excludeViewID int64) (*int64, error) {
	row := s.db.QueryRowContext(ctx, `SELECT view_id FROM placements WHERE element_id = ? AND view_id != ? ORDER BY view_id LIMIT 1`, ownerElementID, excludeViewID)
	var viewID int64
	if err := row.Scan(&viewID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &viewID, nil
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

	placementRows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT p.element_id, p.view_id
		FROM placements p
		JOIN views v ON v.owner_element_id = p.element_id
		ORDER BY p.element_id, p.view_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = placementRows.Close() }()
	for placementRows.Next() {
		var elementID, parentID int64
		if err := placementRows.Scan(&elementID, &parentID); err != nil {
			return nil, err
		}
		for _, childID := range ownerViewIDs[elementID] {
			if parentID == childID || parentMap[childID] != nil {
				continue
			}
			pid := parentID
			parentMap[childID] = &pid
		}
	}
	return parentMap, placementRows.Err()
}

func (s *Store) childViewMeta(ctx context.Context, elementID int64) (bool, *string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT level_label FROM views WHERE owner_element_id = ? ORDER BY id LIMIT 1`, elementID)
	var label sql.NullString
	if err := row.Scan(&label); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil, nil
		}
		return false, nil, err
	}
	if label.Valid {
		return true, &label.String, nil
	}
	return true, nil, nil
}

type childViewMetaValue struct {
	hasView bool
	label   *string
}

func (s *Store) childViewMetaMap(ctx context.Context) (map[int64]childViewMetaValue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT owner_element_id, level_label FROM views WHERE owner_element_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]childViewMetaValue{}
	for rows.Next() {
		var elementID int64
		var label sql.NullString
		if err := rows.Scan(&elementID, &label); err != nil {
			return nil, err
		}
		if _, exists := out[elementID]; exists {
			continue
		}
		meta := childViewMetaValue{hasView: true}
		if label.Valid {
			labelCopy := label.String
			meta.label = &labelCopy
		}
		out[elementID] = meta
	}
	return out, rows.Err()
}

func viewNodeFromRow(row viewRow, parentID *int64, depth int) ViewTreeNode {
	var ownerElementID *int64
	if row.OwnerElementID.Valid {
		ownerElementID = new(row.OwnerElementID.Int64)
	}
	var description *string
	if row.Description.Valid {
		description = new(row.Description.String)
	}
	var levelLabel *string
	if row.LevelLabel.Valid {
		levelLabel = new(row.LevelLabel.String)
	}
	return ViewTreeNode{
		ID:             row.ID,
		OwnerElementID: ownerElementID,
		Name:           row.Name,
		Description:    description,
		LevelLabel:     levelLabel,
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
		node := viewNodeFromRow(row, parentMap[row.ID], depth)
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
			IsRoot:    node.ParentViewID == nil,
			CreatedAt: node.CreatedAt,
			UpdatedAt: node.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Store) ViewByID(ctx context.Context, id int64) (ViewTreeNode, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, owner_element_id, name, description, level_label, level, created_at, updated_at FROM views WHERE id = ?`, id)
	var view viewRow
	if err := row.Scan(&view.ID, &view.OwnerElementID, &view.Name, &view.Description, &view.LevelLabel, &view.Level, &view.CreatedAt, &view.UpdatedAt); err != nil {
		return ViewTreeNode{}, err
	}
	var parentID *int64
	var err error
	if view.OwnerElementID.Valid {
		parentID, err = s.parentViewForOwner(ctx, view.OwnerElementID.Int64, view.ID)
		if err != nil {
			return ViewTreeNode{}, err
		}
	}
	return viewNodeFromRow(view, parentID, 0), nil
}

func (s *Store) ChildViews(ctx context.Context, parentViewID int64) ([]ViewTreeNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT v.id, v.owner_element_id, v.name, v.description, v.level_label, v.level, v.created_at, v.updated_at
		FROM views v
		JOIN placements p ON p.element_id = v.owner_element_id
		WHERE p.view_id = ? AND v.id != ?
		ORDER BY v.id`, parentViewID, parentViewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ViewTreeNode{}
	for rows.Next() {
		var row viewRow
		if err := rows.Scan(&row.ID, &row.OwnerElementID, &row.Name, &row.Description, &row.LevelLabel, &row.Level, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		parentID := parentViewID
		out = append(out, viewNodeFromRow(row, &parentID, 0))
	}
	return out, rows.Err()
}

func (s *Store) RootViews(ctx context.Context) ([]ViewTreeNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.id, v.owner_element_id, v.name, v.description, v.level_label, v.level, v.created_at, v.updated_at
		FROM views v
		WHERE v.owner_element_id IS NULL
		   OR NOT EXISTS (
		     SELECT 1 FROM placements p
		     WHERE p.element_id = v.owner_element_id
		       AND p.view_id != v.id
		   )
		ORDER BY v.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ViewTreeNode{}
	for rows.Next() {
		var row viewRow
		if err := rows.Scan(&row.ID, &row.OwnerElementID, &row.Name, &row.Description, &row.LevelLabel, &row.Level, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, viewNodeFromRow(row, nil, 0))
	}
	return out, rows.Err()
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
	res, err := s.db.ExecContext(ctx, `INSERT INTO views(owner_element_id, name, description, level_label, level, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, ?, ?)`,
		ownerElementID, strings.TrimSpace(name), levelLabel, level, now, now)
	if err != nil {
		return ViewSummary{}, err
	}
	id, _ := res.LastInsertId()
	view, err := s.ViewByID(ctx, id)
	if err != nil {
		return ViewSummary{}, err
	}
	return ViewSummary{
		ID:        view.ID,
		Name:      view.Name,
		Label:     view.LevelLabel,
		IsRoot:    view.ParentViewID == nil,
		CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt,
	}, nil
}

func (s *Store) UpdateView(ctx context.Context, id int64, name *string, levelLabel *string) (ViewSummary, error) {
	current, err := s.ViewByID(ctx, id)
	if err != nil {
		return ViewSummary{}, err
	}
	nextName := current.Name
	if name != nil && strings.TrimSpace(*name) != "" {
		nextName = strings.TrimSpace(*name)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE views SET name = ?, level_label = ?, updated_at = ? WHERE id = ?`, nextName, levelLabel, nowString(), id)
	if err != nil {
		return ViewSummary{}, err
	}
	updated, err := s.ViewByID(ctx, id)
	if err != nil {
		return ViewSummary{}, err
	}
	return ViewSummary{
		ID:        updated.ID,
		Name:      updated.Name,
		Label:     updated.LevelLabel,
		IsRoot:    updated.ParentViewID == nil,
		CreatedAt: updated.CreatedAt,
		UpdatedAt: updated.UpdatedAt,
	}, nil
}

func (s *Store) SetViewLevel(ctx context.Context, id int64, level int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE views SET level = ?, updated_at = ? WHERE id = ?`, level, nowString(), id)
	return err
}

func (s *Store) DeleteView(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM views WHERE id = ?`, id)
	return err
}

func scanElement(row scanner, includeViewMeta bool, store *Store, ctx context.Context) (LibraryElement, error) {
	var (
		elem        LibraryElement
		techRaw     string
		tagRaw      string
		kind        sql.NullString
		description sql.NullString
		technology  sql.NullString
		url         sql.NullString
		logoURL     sql.NullString
		repo        sql.NullString
		branch      sql.NullString
		filePath    sql.NullString
		language    sql.NullString
	)
	if err := row.Scan(&elem.ID, &elem.Name, &kind, &description, &technology, &url, &logoURL, &techRaw, &tagRaw, &repo, &branch, &filePath, &language, &elem.CreatedAt, &elem.UpdatedAt); err != nil {
		return LibraryElement{}, err
	}
	if kind.Valid {
		elem.Kind = &kind.String
	}
	if description.Valid {
		elem.Description = &description.String
	}
	if technology.Valid {
		elem.Technology = &technology.String
	}
	if url.Valid {
		elem.URL = &url.String
	}
	if logoURL.Valid {
		elem.LogoURL = &logoURL.String
	}
	if repo.Valid {
		elem.Repo = &repo.String
	}
	if branch.Valid {
		elem.Branch = &branch.String
	}
	if filePath.Valid {
		elem.FilePath = &filePath.String
	}
	if language.Valid {
		elem.Language = &language.String
	}
	elem.TechnologyConnectors = parseTechnologyConnectors(techRaw)
	elem.Tags = parseStrings(tagRaw)
	if includeViewMeta {
		hasView, label, err := store.childViewMeta(ctx, elem.ID)
		if err != nil {
			return LibraryElement{}, err
		}
		elem.HasView = hasView
		elem.ViewLabel = label
	}
	return elem, nil
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
