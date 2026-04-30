package app

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"sort"
	"strings"
	"time"

	sqlitevec "github.com/viant/sqlite-vec/vec"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func (s *Store) DB() *sql.DB {
	return s.db
}

type TechnologyConnector struct {
	Type          string `json:"type"`
	Slug          string `json:"slug,omitempty"`
	Label         string `json:"label"`
	IsPrimaryIcon bool   `json:"is_primary_icon,omitempty"`
}

type LibraryElement struct {
	ID                   int64                 `json:"id"`
	Name                 string                `json:"name"`
	Kind                 *string               `json:"kind"`
	Description          *string               `json:"description"`
	Technology           *string               `json:"technology"`
	URL                  *string               `json:"url"`
	LogoURL              *string               `json:"logo_url"`
	TechnologyConnectors []TechnologyConnector `json:"technology_connectors"`
	Tags                 []string              `json:"tags"`
	Repo                 *string               `json:"repo,omitempty"`
	Branch               *string               `json:"branch,omitempty"`
	FilePath             *string               `json:"file_path,omitempty"`
	Language             *string               `json:"language,omitempty"`
	CreatedAt            string                `json:"created_at"`
	UpdatedAt            string                `json:"updated_at"`
	HasView              bool                  `json:"has_view"`
	ViewLabel            *string               `json:"view_label"`
}

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

type PlacedElement struct {
	ID                   int64                 `json:"id"`
	ViewID               int64                 `json:"view_id"`
	ElementID            int64                 `json:"element_id"`
	PositionX            float64               `json:"position_x"`
	PositionY            float64               `json:"position_y"`
	Name                 string                `json:"name"`
	Description          *string               `json:"description"`
	Kind                 *string               `json:"kind"`
	Technology           *string               `json:"technology"`
	URL                  *string               `json:"url"`
	LogoURL              *string               `json:"logo_url"`
	TechnologyConnectors []TechnologyConnector `json:"technology_connectors"`
	Tags                 []string              `json:"tags"`
	Repo                 *string               `json:"repo,omitempty"`
	Branch               *string               `json:"branch,omitempty"`
	FilePath             *string               `json:"file_path,omitempty"`
	Language             *string               `json:"language,omitempty"`
	HasView              bool                  `json:"has_view"`
	ViewLabel            *string               `json:"view_label"`
}

type ElementPlacement struct {
	ID        int64   `json:"id"`
	ViewID    int64   `json:"view_id"`
	ElementID int64   `json:"element_id"`
	PositionX float64 `json:"position_x"`
	PositionY float64 `json:"position_y"`
}

type Connector struct {
	ID              int64   `json:"id"`
	ViewID          int64   `json:"view_id"`
	SourceElementID int64   `json:"source_element_id"`
	TargetElementID int64   `json:"target_element_id"`
	Label           *string `json:"label"`
	Description     *string `json:"description"`
	Relationship    *string `json:"relationship"`
	Direction       string  `json:"direction"`
	Style           string  `json:"style"`
	URL             *string `json:"url"`
	SourceHandle    *string `json:"source_handle"`
	TargetHandle    *string `json:"target_handle"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
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

type Tag struct {
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
}

type ExploreViewData struct {
	Placements []PlacedElement `json:"placements"`
	Connectors []Connector     `json:"connectors"`
}

type ExploreData struct {
	Tree        []ViewTreeNode             `json:"tree"`
	Views       map[string]ExploreViewData `json:"views"`
	Navigations []ViewConnector            `json:"navigations"`
}

type DependencyElement struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Description          *string               `json:"description,omitempty"`
	Type                 *string               `json:"type,omitempty"`
	Technology           *string               `json:"technology,omitempty"`
	URL                  *string               `json:"url,omitempty"`
	LogoURL              *string               `json:"logo_url,omitempty"`
	TechnologyConnectors []TechnologyConnector `json:"technology_connectors"`
	Tags                 []string              `json:"tags"`
	Repo                 *string               `json:"repo,omitempty"`
	Branch               *string               `json:"branch,omitempty"`
	Language             *string               `json:"language,omitempty"`
	FilePath             *string               `json:"file_path,omitempty"`
	CreatedAt            string                `json:"created_at"`
	UpdatedAt            string                `json:"updated_at"`
}

type DependencyConnector struct {
	ID               string  `json:"id"`
	ViewID           string  `json:"view_id"`
	SourceElementID  string  `json:"source_element_id"`
	TargetElementID  string  `json:"target_element_id"`
	Label            *string `json:"label,omitempty"`
	Description      *string `json:"description,omitempty"`
	RelationshipType *string `json:"relationship_type,omitempty"`
	Direction        string  `json:"direction"`
	ConnectorType    string  `json:"connector_type"`
	URL              *string `json:"url,omitempty"`
	SourceHandle     *string `json:"source_handle,omitempty"`
	TargetHandle     *string `json:"target_handle,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type PlanElement struct {
	Ref             string                `json:"ref"`
	Name            string                `json:"name"`
	Kind            *string               `json:"kind"`
	Description     *string               `json:"description"`
	Technology      *string               `json:"technology"`
	URL             *string               `json:"url"`
	LogoURL         *string               `json:"logo_url"`
	TechnologyLinks []TechnologyConnector `json:"technology_links"`
	Tags            []string              `json:"tags"`
	Repo            *string               `json:"repo"`
	Branch          *string               `json:"branch"`
	Language        *string               `json:"language"`
	FilePath        *string               `json:"file_path"`
	HasView         bool                  `json:"has_view"`
	ViewLabel       *string               `json:"view_label"`
}

type PlanConnector struct {
	Ref              string  `json:"ref"`
	ViewRef          string  `json:"view_ref"`
	SourceElementRef string  `json:"source_element_ref"`
	TargetElementRef string  `json:"target_element_ref"`
	Label            *string `json:"label"`
	Description      *string `json:"description"`
	Relationship     *string `json:"relationship"`
	Direction        *string `json:"direction"`
	Style            *string `json:"style"`
	URL              *string `json:"url"`
	SourceHandle     *string `json:"source_handle"`
	TargetHandle     *string `json:"target_handle"`
}

func OpenStore(dbPath string, migrations embed.FS) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err := configureSQLiteDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := sqlitevec.Register(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("register sqlite-vec: %w", err)
	}
	if err := applyMigrations(db, migrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.ensureBootstrapData(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func configureSQLiteDB(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []string{
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = NORMAL;`,
		`PRAGMA foreign_keys = ON;`,
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("configure sqlite %s: %w", pragma, err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ensureBootstrapData(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM views`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO views(owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (NULL, ?, ?, ?, 1, ?, ?)`,
		"Workspace",
		"Local offline workspace",
		"Root",
		now,
		now,
	)
	return err
}

func applyMigrations(db *sql.DB, migrations embed.FS) error {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		sqlBytes, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func normalizeDirection(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "forward"
	}
	return *value
}

func normalizeStyle(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "solid"
	}
	return *value
}

func jsonString(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

func parseTechnologyConnectors(raw string) []TechnologyConnector {
	if raw == "" || raw == "null" {
		return []TechnologyConnector{}
	}
	var out []TechnologyConnector
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []TechnologyConnector{}
	}
	if out == nil {
		return []TechnologyConnector{}
	}
	return out
}

func parseStrings(raw string) []string {
	if raw == "" || raw == "null" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

type viewRow struct {
	ID             int64
	OwnerElementID sql.NullInt64
	Name           string
	Description    sql.NullString
	LevelLabel     sql.NullString
	Level          int
	CreatedAt      string
	UpdatedAt      string
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
	tree, err := s.ViewTree(ctx)
	if err != nil {
		return ViewTreeNode{}, err
	}
	for _, node := range flattenTree(tree) {
		if node.ID == id {
			return node, nil
		}
	}
	return ViewTreeNode{}, sql.ErrNoRows
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

type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) Elements(ctx context.Context, limit, offset int, search string) ([]LibraryElement, error) {
	type elementRow struct {
		ID          int64
		Name        string
		Kind        sql.NullString
		Description sql.NullString
		Technology  sql.NullString
		URL         sql.NullString
		LogoURL     sql.NullString
		TechRaw     string
		TagRaw      string
		Repo        sql.NullString
		Branch      sql.NullString
		FilePath    sql.NullString
		Language    sql.NullString
		CreatedAt   string
		UpdatedAt   string
	}

	query := `SELECT id, name, kind, description, technology, url, logo_url, technology_connectors, tags, repo, branch, file_path, language, created_at, updated_at FROM elements`
	args := []any{}
	if strings.TrimSpace(search) != "" {
		query += ` WHERE LOWER(name) LIKE LOWER(?) OR LOWER(COALESCE(description, '')) LIKE LOWER(?)`
		pattern := "%" + strings.TrimSpace(search) + "%"
		args = append(args, pattern, pattern)
	}
	query += ` ORDER BY updated_at DESC`
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scanned := make([]elementRow, 0)
	for rows.Next() {
		var row elementRow
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.Kind,
			&row.Description,
			&row.Technology,
			&row.URL,
			&row.LogoURL,
			&row.TechRaw,
			&row.TagRaw,
			&row.Repo,
			&row.Branch,
			&row.FilePath,
			&row.Language,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]LibraryElement, 0, len(scanned))
	for _, row := range scanned {
		elem := LibraryElement{
			ID:                   row.ID,
			Name:                 row.Name,
			TechnologyConnectors: parseTechnologyConnectors(row.TechRaw),
			Tags:                 parseStrings(row.TagRaw),
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
		}
		if row.Kind.Valid {
			elem.Kind = &row.Kind.String
		}
		if row.Description.Valid {
			elem.Description = &row.Description.String
		}
		if row.Technology.Valid {
			elem.Technology = &row.Technology.String
		}
		if row.URL.Valid {
			elem.URL = &row.URL.String
		}
		if row.LogoURL.Valid {
			elem.LogoURL = &row.LogoURL.String
		}
		if row.Repo.Valid {
			elem.Repo = &row.Repo.String
		}
		if row.Branch.Valid {
			elem.Branch = &row.Branch.String
		}
		if row.FilePath.Valid {
			elem.FilePath = &row.FilePath.String
		}
		if row.Language.Valid {
			elem.Language = &row.Language.String
		}
		if meta, ok := viewMeta[elem.ID]; ok {
			elem.HasView = meta.hasView
			elem.ViewLabel = meta.label
		}
		out = append(out, elem)
	}
	return out, nil
}

func (s *Store) ElementByID(ctx context.Context, id int64) (LibraryElement, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, kind, description, technology, url, logo_url, technology_connectors, tags, repo, branch, file_path, language, created_at, updated_at FROM elements WHERE id = ?`, id)
	return scanElement(row, true, s, ctx)
}

func (s *Store) CreateElement(ctx context.Context, input LibraryElement) (LibraryElement, error) {
	if err := s.ensureTagColors(ctx, input.Tags); err != nil {
		return LibraryElement{}, err
	}
	now := nowString()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO elements(name, kind, description, technology, url, logo_url, technology_connectors, tags, repo, branch, file_path, language, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(input.Name),
		input.Kind,
		input.Description,
		input.Technology,
		input.URL,
		input.LogoURL,
		jsonString(input.TechnologyConnectors, "[]"),
		jsonString(input.Tags, "[]"),
		input.Repo,
		input.Branch,
		input.FilePath,
		input.Language,
		now,
		now,
	)
	if err != nil {
		return LibraryElement{}, err
	}
	id, _ := res.LastInsertId()
	return s.ElementByID(ctx, id)
}

func (s *Store) UpdateElement(ctx context.Context, id int64, input LibraryElement) (LibraryElement, error) {
	if input.Tags != nil {
		if err := s.ensureTagColors(ctx, input.Tags); err != nil {
			return LibraryElement{}, err
		}
	}
	current, err := s.ElementByID(ctx, id)
	if err != nil {
		return LibraryElement{}, err
	}
	if input.Name == "" {
		input.Name = current.Name
	}
	if input.Kind == nil {
		input.Kind = current.Kind
	}
	if input.Description == nil {
		input.Description = current.Description
	}
	if input.Technology == nil {
		input.Technology = current.Technology
	}
	if input.URL == nil {
		input.URL = current.URL
	}
	if input.LogoURL == nil {
		input.LogoURL = current.LogoURL
	}
	if input.Repo == nil {
		input.Repo = current.Repo
	}
	if input.Branch == nil {
		input.Branch = current.Branch
	}
	if input.FilePath == nil {
		input.FilePath = current.FilePath
	}
	if input.Language == nil {
		input.Language = current.Language
	}
	if input.TechnologyConnectors == nil {
		input.TechnologyConnectors = current.TechnologyConnectors
	}
	if input.Tags == nil {
		input.Tags = current.Tags
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE elements SET name = ?, kind = ?, description = ?, technology = ?, url = ?, logo_url = ?, technology_connectors = ?, tags = ?, repo = ?, branch = ?, file_path = ?, language = ?, updated_at = ?
		WHERE id = ?`,
		input.Name, input.Kind, input.Description, input.Technology, input.URL, input.LogoURL,
		jsonString(input.TechnologyConnectors, "[]"), jsonString(input.Tags, "[]"),
		input.Repo, input.Branch, input.FilePath, input.Language, nowString(), id,
	)
	if err != nil {
		return LibraryElement{}, err
	}
	return s.ElementByID(ctx, id)
}

func (s *Store) DeleteElement(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM elements WHERE id = ?`, id)
	return err
}

func (s *Store) ListElementPlacements(ctx context.Context, elementID int64) ([]ViewPlacement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.view_id, v.name
		FROM placements p
		JOIN views v ON v.id = p.view_id
		WHERE p.element_id = ?
		ORDER BY p.view_id`, elementID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]ViewPlacement, 0)
	for rows.Next() {
		var placement ViewPlacement
		if err := rows.Scan(&placement.ViewID, &placement.ViewName); err != nil {
			return nil, err
		}
		out = append(out, placement)
	}
	return out, rows.Err()
}

func (s *Store) Placements(ctx context.Context, viewID int64) ([]PlacedElement, error) {
	type placementRow struct {
		item    PlacedElement
		techRaw string
		tagRaw  string
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.view_id, p.element_id, p.position_x, p.position_y,
		       e.name, e.kind, e.description, e.technology, e.url, e.logo_url, e.technology_connectors, e.tags, e.repo, e.branch, e.file_path, e.language, e.created_at, e.updated_at
		FROM placements p
		JOIN elements e ON e.id = p.element_id
		WHERE p.view_id = ?
		ORDER BY p.id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scanned := make([]placementRow, 0)
	for rows.Next() {
		var row placementRow
		if err := rows.Scan(&row.item.ID, &row.item.ViewID, &row.item.ElementID, &row.item.PositionX, &row.item.PositionY,
			&row.item.Name, &row.item.Kind, &row.item.Description, &row.item.Technology, &row.item.URL, &row.item.LogoURL,
			&row.techRaw, &row.tagRaw, &row.item.Repo, &row.item.Branch, &row.item.FilePath, &row.item.Language, new(string), new(string)); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PlacedElement, 0, len(scanned))
	for _, row := range scanned {
		item := row.item
		item.TechnologyConnectors = parseTechnologyConnectors(row.techRaw)
		item.Tags = parseStrings(row.tagRaw)
		if meta, ok := viewMeta[item.ElementID]; ok {
			item.HasView = meta.hasView
			item.ViewLabel = meta.label
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) AllPlacements(ctx context.Context) ([]PlacedElement, error) {
	type placementRow struct {
		item    PlacedElement
		techRaw string
		tagRaw  string
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.view_id, p.element_id, p.position_x, p.position_y,
		       e.name, e.kind, e.description, e.technology, e.url, e.logo_url, e.technology_connectors, e.tags, e.repo, e.branch, e.file_path, e.language, e.created_at, e.updated_at
		FROM placements p
		JOIN elements e ON e.id = p.element_id
		ORDER BY p.view_id, p.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scanned := make([]placementRow, 0)
	for rows.Next() {
		var row placementRow
		if err := rows.Scan(&row.item.ID, &row.item.ViewID, &row.item.ElementID, &row.item.PositionX, &row.item.PositionY,
			&row.item.Name, &row.item.Kind, &row.item.Description, &row.item.Technology, &row.item.URL, &row.item.LogoURL,
			&row.techRaw, &row.tagRaw, &row.item.Repo, &row.item.Branch, &row.item.FilePath, &row.item.Language, new(string), new(string)); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PlacedElement, 0, len(scanned))
	for _, row := range scanned {
		item := row.item
		item.TechnologyConnectors = parseTechnologyConnectors(row.techRaw)
		item.Tags = parseStrings(row.tagRaw)
		if meta, ok := viewMeta[item.ElementID]; ok {
			item.HasView = meta.hasView
			item.ViewLabel = meta.label
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ElementPlacements(ctx context.Context, viewID int64) ([]ElementPlacement, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, view_id, element_id, position_x, position_y FROM placements WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]ElementPlacement, 0)
	for rows.Next() {
		var item ElementPlacement
		if err := rows.Scan(&item.ID, &item.ViewID, &item.ElementID, &item.PositionX, &item.PositionY); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AddPlacement(ctx context.Context, viewID, elementID int64, x, y float64) (ElementPlacement, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(view_id, element_id) DO UPDATE SET position_x = excluded.position_x, position_y = excluded.position_y, updated_at = excluded.updated_at`,
		viewID, elementID, x, y, now, now)
	if err != nil {
		return ElementPlacement{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, view_id, element_id, position_x, position_y FROM placements WHERE view_id = ? AND element_id = ?`, viewID, elementID)
	var item ElementPlacement
	if err := row.Scan(&item.ID, &item.ViewID, &item.ElementID, &item.PositionX, &item.PositionY); err != nil {
		return ElementPlacement{}, err
	}
	return item, nil
}

func (s *Store) UpdatePlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE placements SET position_x = ?, position_y = ?, updated_at = ? WHERE view_id = ? AND element_id = ?`, x, y, nowString(), viewID, elementID)
	return err
}

func (s *Store) DeletePlacement(ctx context.Context, viewID, elementID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM placements WHERE view_id = ? AND element_id = ?`, viewID, elementID)
	return err
}

func (s *Store) Connectors(ctx context.Context, viewID int64) ([]Connector, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, view_id, source_element_id, target_element_id, label, description, relationship, direction, style, url, source_handle, target_handle, created_at, updated_at
		FROM connectors WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]Connector, 0)
	for rows.Next() {
		var item Connector
		if err := rows.Scan(&item.ID, &item.ViewID, &item.SourceElementID, &item.TargetElementID, &item.Label, &item.Description, &item.Relationship, &item.Direction, &item.Style, &item.URL, &item.SourceHandle, &item.TargetHandle, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AllConnectors(ctx context.Context) ([]Connector, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, view_id, source_element_id, target_element_id, label, description, relationship, direction, style, url, source_handle, target_handle, created_at, updated_at
		FROM connectors ORDER BY view_id, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]Connector, 0)
	for rows.Next() {
		var item Connector
		if err := rows.Scan(&item.ID, &item.ViewID, &item.SourceElementID, &item.TargetElementID, &item.Label, &item.Description, &item.Relationship, &item.Direction, &item.Style, &item.URL, &item.SourceHandle, &item.TargetHandle, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateConnector(ctx context.Context, input Connector) (Connector, error) {
	now := nowString()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO connectors(view_id, source_element_id, target_element_id, label, description, relationship, direction, style, url, source_handle, target_handle, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ViewID, input.SourceElementID, input.TargetElementID, input.Label, input.Description, input.Relationship,
		normalizeDirection(new(input.Direction)), input.Style, input.URL, input.SourceHandle, input.TargetHandle, now, now)
	if err != nil {
		return Connector{}, err
	}
	id, _ := res.LastInsertId()
	return s.ConnectorByID(ctx, id)
}

func (s *Store) ConnectorByID(ctx context.Context, id int64) (Connector, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, view_id, source_element_id, target_element_id, label, description, relationship, direction, style, url, source_handle, target_handle, created_at, updated_at FROM connectors WHERE id = ?`, id)
	var item Connector
	if err := row.Scan(&item.ID, &item.ViewID, &item.SourceElementID, &item.TargetElementID, &item.Label, &item.Description, &item.Relationship, &item.Direction, &item.Style, &item.URL, &item.SourceHandle, &item.TargetHandle, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Connector{}, err
	}
	return item, nil
}

func (s *Store) UpdateConnector(ctx context.Context, id int64, patch Connector) (Connector, error) {
	current, err := s.ConnectorByID(ctx, id)
	if err != nil {
		return Connector{}, err
	}
	if patch.SourceElementID == 0 {
		patch.SourceElementID = current.SourceElementID
	}
	if patch.TargetElementID == 0 {
		patch.TargetElementID = current.TargetElementID
	}
	if patch.ViewID == 0 {
		patch.ViewID = current.ViewID
	}
	if patch.Direction == "" {
		patch.Direction = current.Direction
	}
	if patch.Style == "" {
		patch.Style = current.Style
	}
	if patch.Label == nil {
		patch.Label = current.Label
	}
	if patch.Description == nil {
		patch.Description = current.Description
	}
	if patch.Relationship == nil {
		patch.Relationship = current.Relationship
	}
	if patch.URL == nil {
		patch.URL = current.URL
	}
	if patch.SourceHandle == nil {
		patch.SourceHandle = current.SourceHandle
	}
	if patch.TargetHandle == nil {
		patch.TargetHandle = current.TargetHandle
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE connectors SET source_element_id = ?, target_element_id = ?, label = ?, description = ?, relationship = ?, direction = ?, style = ?, url = ?, source_handle = ?, target_handle = ?, updated_at = ?
		WHERE id = ?`,
		patch.SourceElementID, patch.TargetElementID, patch.Label, patch.Description, patch.Relationship, patch.Direction, patch.Style, patch.URL, patch.SourceHandle, patch.TargetHandle, nowString(), id)
	if err != nil {
		return Connector{}, err
	}
	return s.ConnectorByID(ctx, id)
}

func (s *Store) DeleteConnector(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM connectors WHERE id = ?`, id)
	return err
}

func (s *Store) Layers(ctx context.Context, viewID int64) ([]ViewLayer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, view_id, name, tags, color, created_at, updated_at FROM view_layers WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]ViewLayer, 0)
	for rows.Next() {
		var rawTags string
		var item ViewLayer
		if err := rows.Scan(&item.ID, &item.DiagramID, &item.Name, &rawTags, &item.Color, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Tags = parseStrings(rawTags)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateLayer(ctx context.Context, viewID int64, name string, tags []string, color *string) (ViewLayer, error) {
	if err := s.ensureTagColors(ctx, tags); err != nil {
		return ViewLayer{}, err
	}

	if color == nil || strings.TrimSpace(*color) == "" {
		// User said pick unused, usually means relative to existing layers in the same view or global tags.
		// Frontend uses tagColors.
		tagsMap, err := s.Tags(ctx)
		if err != nil {
			return ViewLayer{}, err
		}
		var usedColors []string
		for _, t := range tagsMap {
			usedColors = append(usedColors, t.Color)
		}
		// Also consider existing layers colors
		layers, err := s.Layers(ctx, viewID)
		if err == nil {
			for _, l := range layers {
				if l.Color != nil {
					usedColors = append(usedColors, *l.Color)
				}
			}
		}
		c := s.pickUnusedColor(ctx, usedColors)
		color = &c
	}

	now := nowString()
	res, err := s.db.ExecContext(ctx, `INSERT INTO view_layers(view_id, name, tags, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		viewID, name, jsonString(tags, "[]"), color, now, now)
	if err != nil {
		return ViewLayer{}, err
	}
	id, _ := res.LastInsertId()
	return s.LayerByID(ctx, id)
}

func (s *Store) LayerByID(ctx context.Context, id int64) (ViewLayer, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, view_id, name, tags, color, created_at, updated_at FROM view_layers WHERE id = ?`, id)
	var rawTags string
	var item ViewLayer
	if err := row.Scan(&item.ID, &item.DiagramID, &item.Name, &rawTags, &item.Color, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ViewLayer{}, err
	}
	item.Tags = parseStrings(rawTags)
	return item, nil
}

func (s *Store) UpdateLayer(ctx context.Context, id int64, patch ViewLayer) (ViewLayer, error) {
	current, err := s.LayerByID(ctx, id)
	if err != nil {
		return ViewLayer{}, err
	}
	if patch.Name == "" {
		patch.Name = current.Name
	}
	if patch.Tags == nil {
		patch.Tags = current.Tags
	}
	if patch.Color == nil {
		patch.Color = current.Color
	}
	_, err = s.db.ExecContext(ctx, `UPDATE view_layers SET name = ?, tags = ?, color = ?, updated_at = ? WHERE id = ?`, patch.Name, jsonString(patch.Tags, "[]"), patch.Color, nowString(), id)
	if err != nil {
		return ViewLayer{}, err
	}
	return s.LayerByID(ctx, id)
}

func (s *Store) DeleteLayer(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM view_layers WHERE id = ?`, id)
	return err
}

func (s *Store) Tags(ctx context.Context) (map[string]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, color, description FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]Tag{}
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.Color, &tag.Description); err != nil {
			return nil, err
		}
		out[tag.Name] = tag
	}
	return out, rows.Err()
}

func (s *Store) UpdateTag(ctx context.Context, name, color string, description *string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tags(name, color, description) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET color = excluded.color, description = excluded.description`,
		name, color, description)
	return err
}

func (s *Store) ListElementNavigations(ctx context.Context, elementID int64, fromViewID, toViewID *int64) ([]ViewConnector, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name FROM views WHERE owner_element_id = ? ORDER BY id LIMIT 1`, elementID)
	var childViewID int64
	var childViewName string
	if err := row.Scan(&childViewID, &childViewName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []ViewConnector{}, nil
		}
		return nil, err
	}
	parentID, err := s.parentViewForOwner(ctx, elementID, childViewID)
	if err != nil {
		return nil, err
	}
	out := make([]ViewConnector, 0, 1)
	if fromViewID != nil && *fromViewID > 0 {
		if parentID != nil && *parentID == *fromViewID {
			out = append(out, ViewConnector{ID: 0, ElementID: &elementID, FromViewID: *fromViewID, ToViewID: childViewID, ToViewName: childViewName, RelationType: "child"})
		}
		return out, nil
	}
	if toViewID != nil && *toViewID > 0 && parentID != nil && *toViewID == childViewID {
		out = append(out, ViewConnector{ID: 0, ElementID: &elementID, FromViewID: *parentID, ToViewID: childViewID, ToViewName: childViewName, RelationType: "child"})
	}
	return out, nil
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

func (s *Store) Explore(ctx context.Context) (ExploreData, error) {
	tree, err := s.ViewTree(ctx)
	if err != nil {
		return ExploreData{}, err
	}
	flat := flattenTree(tree)
	views := map[string]ExploreViewData{}
	navs := make([]ViewConnector, 0)
	for _, view := range flat {
		placements, err := s.Placements(ctx, view.ID)
		if err != nil {
			return ExploreData{}, err
		}
		connectors, err := s.Connectors(ctx, view.ID)
		if err != nil {
			return ExploreData{}, err
		}
		views[fmt.Sprint(view.ID)] = ExploreViewData{
			Placements: placements,
			Connectors: connectors,
		}
		for _, placement := range placements {
			if placement.HasView {
				child, err := s.ListElementNavigations(ctx, placement.ElementID, &view.ID, nil)
				if err != nil {
					return ExploreData{}, err
				}
				navs = append(navs, child...)
			}
		}
	}
	return ExploreData{Tree: tree, Views: views, Navigations: navs}, nil
}

func (s *Store) Dependencies(ctx context.Context) (map[string]any, error) {
	elements, err := s.Elements(ctx, 0, 0, "")
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, view_id, source_element_id, target_element_id, label, description, relationship, direction, style, url, source_handle, target_handle, created_at, updated_at FROM connectors ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	connectors := []DependencyConnector{}
	for rows.Next() {
		var c Connector
		if err := rows.Scan(&c.ID, &c.ViewID, &c.SourceElementID, &c.TargetElementID, &c.Label, &c.Description, &c.Relationship, &c.Direction, &c.Style, &c.URL, &c.SourceHandle, &c.TargetHandle, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		connectors = append(connectors, DependencyConnector{
			ID:               fmt.Sprint(c.ID),
			ViewID:           fmt.Sprint(c.ViewID),
			SourceElementID:  fmt.Sprint(c.SourceElementID),
			TargetElementID:  fmt.Sprint(c.TargetElementID),
			Label:            c.Label,
			Description:      c.Description,
			RelationshipType: c.Relationship,
			Direction:        c.Direction,
			ConnectorType:    c.Style,
			URL:              c.URL,
			SourceHandle:     c.SourceHandle,
			TargetHandle:     c.TargetHandle,
			CreatedAt:        c.CreatedAt,
			UpdatedAt:        c.UpdatedAt,
		})
	}
	deps := []DependencyElement{}
	for _, element := range elements {
		deps = append(deps, DependencyElement{
			ID:                   fmt.Sprint(element.ID),
			Name:                 element.Name,
			Description:          element.Description,
			Type:                 element.Kind,
			Technology:           element.Technology,
			URL:                  element.URL,
			LogoURL:              element.LogoURL,
			TechnologyConnectors: element.TechnologyConnectors,
			Tags:                 element.Tags,
			Repo:                 element.Repo,
			Branch:               element.Branch,
			Language:             element.Language,
			FilePath:             element.FilePath,
			CreatedAt:            element.CreatedAt,
			UpdatedAt:            element.UpdatedAt,
		})
	}
	return map[string]any{"elements": deps, "connectors": connectors}, nil
}

func (s *Store) ImportPlan(ctx context.Context, elements []PlanElement, connectors []PlanConnector) (int64, error) {
	viewName := "Imported Diagram"
	if len(elements) > 0 && strings.TrimSpace(elements[0].Name) != "" {
		viewName = strings.TrimSpace(elements[0].Name)
	}
	view, err := s.CreateView(ctx, viewName, new("Imported"), nil)
	if err != nil {
		return 0, err
	}
	refToID := map[string]int64{}
	for index, element := range elements {
		created, err := s.CreateElement(ctx, LibraryElement{
			Name:                 element.Name,
			Kind:                 element.Kind,
			Description:          element.Description,
			Technology:           element.Technology,
			URL:                  element.URL,
			LogoURL:              element.LogoURL,
			TechnologyConnectors: element.TechnologyLinks,
			Tags:                 element.Tags,
			Repo:                 element.Repo,
			Branch:               element.Branch,
			FilePath:             element.FilePath,
			Language:             element.Language,
		})
		if err != nil {
			return 0, err
		}
		refToID[element.Ref] = created.ID
		col := index % 4
		row := index / 4
		if _, err := s.AddPlacement(ctx, view.ID, created.ID, float64(120+col*240), float64(120+row*180)); err != nil {
			return 0, err
		}
	}
	for _, connector := range connectors {
		sourceID := refToID[connector.SourceElementRef]
		targetID := refToID[connector.TargetElementRef]
		if sourceID == 0 || targetID == 0 {
			continue
		}
		if _, err := s.CreateConnector(ctx, Connector{
			ViewID:          view.ID,
			SourceElementID: sourceID,
			TargetElementID: targetID,
			Label:           connector.Label,
			Description:     connector.Description,
			Relationship:    connector.Relationship,
			Direction:       normalizeDirection(connector.Direction),
			Style:           normalizeStyle(connector.Style),
			URL:             connector.URL,
			SourceHandle:    connector.SourceHandle,
			TargetHandle:    connector.TargetHandle,
		}); err != nil {
			return 0, err
		}
	}
	return view.ID, nil
}

func (s *Store) ThumbnailSVG(ctx context.Context, viewID int64) (string, error) {
	placements, err := s.Placements(ctx, viewID)
	if err != nil {
		return "", err
	}
	connectors, err := s.Connectors(ctx, viewID)
	if err != nil {
		return "", err
	}
	const width = 320.0
	const height = 180.0
	var minX, minY, maxX, maxY float64
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range placements {
		minX = math.Min(minX, p.PositionX)
		minY = math.Min(minY, p.PositionY)
		maxX = math.Max(maxX, p.PositionX+140)
		maxY = math.Max(maxY, p.PositionY+80)
	}
	if len(placements) == 0 {
		minX, minY, maxX, maxY = 0, 0, width, height
	}
	scaleX := width / math.Max(1, maxX-minX)
	scaleY := height / math.Max(1, maxY-minY)
	scale := math.Min(scaleX, scaleY) * 0.9
	offsetX := (width - (maxX-minX)*scale) / 2
	offsetY := (height - (maxY-minY)*scale) / 2
	point := func(x, y float64) (float64, float64) {
		return offsetX + (x-minX)*scale, offsetY + (y-minY)*scale
	}
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180" viewBox="0 0 320 180">`)
	b.WriteString(`<rect width="320" height="180" rx="12" fill="#0f172a"/>`)
	for _, c := range connectors {
		var src, dst *PlacedElement
		for i := range placements {
			if placements[i].ElementID == c.SourceElementID {
				src = &placements[i]
			}
			if placements[i].ElementID == c.TargetElementID {
				dst = &placements[i]
			}
		}
		if src == nil || dst == nil {
			continue
		}
		x1, y1 := point(src.PositionX+70, src.PositionY+40)
		x2, y2 := point(dst.PositionX+70, dst.PositionY+40)
		fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="#475569" stroke-width="2"/>`, x1, y1, x2, y2)
	}
	for _, p := range placements {
		x, y := point(p.PositionX, p.PositionY)
		w := 140.0 * scale
		h := 80.0 * scale
		fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="10" fill="#1e293b" stroke="#64748b"/>`, x, y, w, h)
		fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" font-family="sans-serif" font-size="10" fill="#e2e8f0">`, x+8, y+18)
		b.WriteString(htmlEscape(trimTo(p.Name, 24)))
		b.WriteString(`</text>`)
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

func trimTo(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(value)
}

var SWATCH_COLORS = []string{
	"#F56565", "#ED8936", "#ECC94B", "#48BB78", "#38B2AC",
	"#4299E1", "#667EEA", "#9F7AEA", "#ED64A6", "#A0AEC0",
}

func (s *Store) pickUnusedColor(ctx context.Context, usedColors []string) string {
	used := make(map[string]bool)
	for _, c := range usedColors {
		used[strings.ToUpper(c)] = true
	}

	var pool []string
	for _, c := range SWATCH_COLORS {
		if !used[strings.ToUpper(c)] {
			pool = append(pool, c)
		}
	}

	source := pool
	if len(source) == 0 {
		source = SWATCH_COLORS
	}

	return source[time.Now().UnixNano()%int64(len(source))]
}

func (s *Store) ensureTagColors(ctx context.Context, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	existing, err := s.Tags(ctx)
	if err != nil {
		return err
	}

	var usedColors []string
	for _, t := range existing {
		usedColors = append(usedColors, t.Color)
	}

	for _, name := range tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := existing[name]; !ok {
			color := s.pickUnusedColor(ctx, usedColors)
			if err := s.UpdateTag(ctx, name, color, nil); err != nil {
				return err
			}
			usedColors = append(usedColors, color)
			// Refresh existing to avoid re-adding same tag if it appears twice in the list
			existing[name] = Tag{Name: name, Color: color}
		}
	}
	return nil
}
