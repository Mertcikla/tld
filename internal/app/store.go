package app

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
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

type ExploreViewData struct {
	Placements []PlacedElement `json:"placements"`
	Connectors []Connector     `json:"connectors"`
}

type ExploreData struct {
	Tree        []ViewTreeNode             `json:"tree"`
	Views       map[string]ExploreViewData `json:"views"`
	Navigations []ViewConnector            `json:"navigations"`
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
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
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

type scanner interface {
	Scan(dest ...any) error
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
	elements, _, err := s.Elements(ctx, 0, 0, "")
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
