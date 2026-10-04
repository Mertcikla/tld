package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
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
	RepositoryID         *string               `json:"repository_id,omitempty"`
	Branch               *string               `json:"branch,omitempty"`
	FilePath             *string               `json:"file_path,omitempty"`
	Language             *string               `json:"language,omitempty"`
	BypassNoiseGate      bool                  `json:"bypass_noise_gate"`
	BypassNoiseGateSet   bool                  `json:"-"`
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
	RepositoryID         *string               `json:"repository_id,omitempty"`
	Branch               *string               `json:"branch,omitempty"`
	FilePath             *string               `json:"file_path,omitempty"`
	Language             *string               `json:"language,omitempty"`
	BypassNoiseGate      bool                  `json:"bypass_noise_gate"`
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
	RepositoryID         *string               `json:"repository_id,omitempty"`
	Branch               *string               `json:"branch,omitempty"`
	Language             *string               `json:"language,omitempty"`
	FilePath             *string               `json:"file_path,omitempty"`
	BypassNoiseGate      bool                  `json:"bypass_noise_gate"`
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
	RepositoryID    *string               `json:"repository_id"`
	Branch          *string               `json:"branch"`
	Language        *string               `json:"language"`
	FilePath        *string               `json:"file_path"`
	BypassNoiseGate *bool                 `json:"bypass_noise_gate"`
	HasView         bool                  `json:"has_view"`
	ViewLabel       *string               `json:"view_label"`
}

func optionalBool(value bool, set bool) *bool {
	if !set {
		return nil
	}
	return &value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func (s *Store) Elements(ctx context.Context, limit, offset int, search string) ([]LibraryElement, int, error) {
	query := s.bun.NewSelect().Model((*elementModel)(nil))
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		query = query.Where("org_id = ?", orgID)
	}
	if strings.TrimSpace(search) != "" {
		pattern := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?)", pattern)
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var rows []elementModel
	selectQuery := s.bun.NewSelect().Model(&rows)
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		selectQuery = selectQuery.Where("org_id = ?", orgID)
	}
	if strings.TrimSpace(search) != "" {
		pattern := "%" + strings.TrimSpace(search) + "%"
		selectQuery = selectQuery.Where("LOWER(name) LIKE LOWER(?)", pattern)
	}
	selectQuery = selectQuery.OrderExpr("LOWER(name) ASC").Order("id")
	if limit > 0 {
		selectQuery = selectQuery.Limit(limit).Offset(offset)
	}
	if err := selectQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, 0, err
	}

	out := make([]LibraryElement, 0, len(rows))
	for _, row := range rows {
		elem := elementFromModel(row)
		if meta, ok := viewMeta[elem.ID]; ok {
			elem.HasView = meta.hasView
			elem.ViewLabel = meta.label
		}
		out = append(out, elem)
	}
	return out, total, nil
}

func (s *Store) ElementByID(ctx context.Context, id int64) (LibraryElement, error) {
	var row elementModel
	if err := s.bun.NewSelect().Model(&row).Where("id = ?", id).Scan(ctx); err != nil {
		return LibraryElement{}, err
	}
	elem := elementFromModel(row)
	hasView, label, err := s.childViewMeta(ctx, elem.ID)
	if err != nil {
		return LibraryElement{}, err
	}
	elem.HasView = hasView
	elem.ViewLabel = label
	return elem, nil
}

func (s *Store) CreateElement(ctx context.Context, input LibraryElement) (LibraryElement, error) {
	if err := s.ensureTagColors(ctx, input.Tags); err != nil {
		return LibraryElement{}, err
	}
	now := nowString()
	row := &elementModel{
		Name:                 strings.TrimSpace(input.Name),
		Kind:                 input.Kind,
		Description:          input.Description,
		Technology:           input.Technology,
		URL:                  input.URL,
		LogoURL:              input.LogoURL,
		TechnologyConnectors: jsonString(input.TechnologyConnectors, "[]"),
		Tags:                 jsonString(input.Tags, "[]"),
		Repo:                 input.Repo,
		RepositoryID:         input.RepositoryID,
		Branch:               input.Branch,
		FilePath:             input.FilePath,
		Language:             input.Language,
		BypassNoiseGate:      input.BypassNoiseGate,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	_, err := s.bun.NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return LibraryElement{}, err
	}
	return s.ElementByID(ctx, row.ID)
}

func (s *Store) UpdateElement(ctx context.Context, id int64, input LibraryElement) (LibraryElement, error) {
	if input.Tags != nil {
		if err := s.ensureTagColors(ctx, input.Tags); err != nil {
			return LibraryElement{}, err
		}
	}
	var technologyConnectors any
	if input.TechnologyConnectors != nil {
		technologyConnectors = jsonString(input.TechnologyConnectors, "[]")
	}
	var tags any
	if input.Tags != nil {
		tags = jsonString(input.Tags, "[]")
	}
	res, err := s.bun.NewUpdate().
		Model((*elementModel)(nil)).
		Set("name = COALESCE(NULLIF(?, ''), name)", strings.TrimSpace(input.Name)).
		Set("kind = COALESCE(?, kind)", input.Kind).
		Set("description = COALESCE(?, description)", input.Description).
		Set("technology = COALESCE(?, technology)", input.Technology).
		Set("url = COALESCE(?, url)", input.URL).
		Set("logo_url = COALESCE(?, logo_url)", input.LogoURL).
		Set("technology_connectors = COALESCE(?, technology_connectors)", technologyConnectors).
		Set("tags = COALESCE(?, tags)", tags).
		Set("repo = COALESCE(?, repo)", input.Repo).
		Set("repository_id = COALESCE(?, repository_id)", input.RepositoryID).
		Set("branch = COALESCE(?, branch)", input.Branch).
		Set("file_path = COALESCE(?, file_path)", input.FilePath).
		Set("language = COALESCE(?, language)", input.Language).
		Set("bypass_noise_gate = COALESCE(?, bypass_noise_gate)", optionalBool(input.BypassNoiseGate, input.BypassNoiseGateSet)).
		Set("updated_at = ?", nowString()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return LibraryElement{}, err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return LibraryElement{}, sql.ErrNoRows
	}
	return s.ElementByID(ctx, id)
}

func (s *Store) DeleteElement(ctx context.Context, id int64) error {
	_, err := s.bun.NewDelete().Model((*elementModel)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) ListElementPlacements(ctx context.Context, elementID int64) ([]ViewPlacement, error) {
	var rows []ViewPlacement
	query := s.bun.NewSelect().
		TableExpr("placements AS p").
		ColumnExpr("p.view_id").
		ColumnExpr("v.name AS view_name").
		Join("JOIN views AS v ON v.id = p.view_id").
		Where("p.element_id = ?", elementID).
		Order("p.view_id")
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		query = query.Where("v.org_id = ?", orgID)
	}
	if err := query.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) ListElementNavigations(ctx context.Context, elementID int64, fromViewID, toViewID *int64) ([]ViewConnector, error) {
	var child viewModel
	if err := s.bun.NewSelect().
		Model(&child).
		Column("id", "name").
		Where("owner_element_id = ?", elementID).
		Order("id").
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []ViewConnector{}, nil
		}
		return nil, err
	}
	parentID, err := s.parentViewForOwner(ctx, elementID, child.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ViewConnector, 0, 1)
	if fromViewID != nil && *fromViewID > 0 {
		if parentID != nil && *parentID == *fromViewID {
			out = append(out, ViewConnector{ID: 0, ElementID: &elementID, FromViewID: *fromViewID, ToViewID: child.ID, ToViewName: child.Name, RelationType: "child"})
		}
		return out, nil
	}
	if toViewID != nil && *toViewID > 0 && parentID != nil && *toViewID == child.ID {
		out = append(out, ViewConnector{ID: 0, ElementID: &elementID, FromViewID: *parentID, ToViewID: child.ID, ToViewName: child.Name, RelationType: "child"})
	}
	return out, nil
}
