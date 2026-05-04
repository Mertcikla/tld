package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

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

func (s *Store) Elements(ctx context.Context, limit, offset int, search string) ([]LibraryElement, int, error) {
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

	where := ""
	args := []any{}
	if strings.TrimSpace(search) != "" {
		where = ` WHERE LOWER(name) LIKE LOWER(?) OR LOWER(COALESCE(description, '')) LIKE LOWER(?)`
		pattern := "%" + strings.TrimSpace(search) + "%"
		args = append(args, pattern, pattern)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM elements`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, name, kind, description, technology, url, logo_url, technology_connectors, tags, repo, branch, file_path, language, created_at, updated_at FROM elements` + where
	query += ` ORDER BY updated_at DESC`
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, 0, err
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
	return out, total, nil
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
