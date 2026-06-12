package app

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type viewLayerModel struct {
	bun.BaseModel `bun:"table:view_layers"`

	ID        int64   `bun:"id,pk,autoincrement"`
	ViewID    int64   `bun:"view_id"`
	Name      string  `bun:"name"`
	Tags      string  `bun:"tags"`
	Color     *string `bun:"color"`
	CreatedAt string  `bun:"created_at"`
	UpdatedAt string  `bun:"updated_at"`
}

type tagModel struct {
	bun.BaseModel `bun:"table:tags"`

	OrgID       *uuid.UUID `bun:"org_id,nullzero"`
	Name        string     `bun:"name,pk"`
	Color       string     `bun:"color"`
	Description *string    `bun:"description"`
}

func (m *tagModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return applyTenantScope(ctx, query, func(orgID uuid.UUID) {
		if m != nil && m.OrgID == nil {
			m.OrgID = &orgID
		}
	})
}

func (m *tagModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *tagModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *tagModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyTenantWhere(ctx, query)
}

type elementPlacementModel struct {
	bun.BaseModel `bun:"table:placements"`

	ID        int64   `bun:"id,pk,autoincrement"`
	ViewID    int64   `bun:"view_id"`
	ElementID int64   `bun:"element_id"`
	PositionX float64 `bun:"position_x"`
	PositionY float64 `bun:"position_y"`
	CreatedAt string  `bun:"created_at"`
	UpdatedAt string  `bun:"updated_at"`
}

type connectorModel struct {
	bun.BaseModel `bun:"table:connectors"`

	ID              int64      `bun:"id,pk,autoincrement"`
	OrgID           *uuid.UUID `bun:"org_id,nullzero"`
	ViewID          int64      `bun:"view_id"`
	SourceElementID int64      `bun:"source_element_id"`
	TargetElementID int64      `bun:"target_element_id"`
	Label           *string    `bun:"label"`
	Description     *string    `bun:"description"`
	Relationship    *string    `bun:"relationship"`
	Direction       string     `bun:"direction"`
	Style           string     `bun:"style"`
	URL             *string    `bun:"url"`
	SourceHandle    *string    `bun:"source_handle"`
	TargetHandle    *string    `bun:"target_handle"`
	Tags            string     `bun:"tags"`
	CreatedAt       string     `bun:"created_at"`
	UpdatedAt       string     `bun:"updated_at"`
}

func (m *connectorModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return applyTenantScope(ctx, query, func(orgID uuid.UUID) {
		if m != nil && m.OrgID == nil {
			m.OrgID = &orgID
		}
	})
}

func (m *connectorModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *connectorModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *connectorModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyTenantWhere(ctx, query)
}

type elementModel struct {
	bun.BaseModel `bun:"table:elements"`

	ID                   int64      `bun:"id,pk,autoincrement"`
	OrgID                *uuid.UUID `bun:"org_id,nullzero"`
	Name                 string     `bun:"name"`
	Kind                 *string    `bun:"kind"`
	Description          *string    `bun:"description"`
	Technology           *string    `bun:"technology"`
	URL                  *string    `bun:"url"`
	LogoURL              *string    `bun:"logo_url"`
	TechnologyConnectors string     `bun:"technology_connectors"`
	Tags                 string     `bun:"tags"`
	Repo                 *string    `bun:"repo"`
	Branch               *string    `bun:"branch"`
	FilePath             *string    `bun:"file_path"`
	Language             *string    `bun:"language"`
	BypassNoiseGate      bool       `bun:"bypass_noise_gate"`
	CreatedAt            string     `bun:"created_at"`
	UpdatedAt            string     `bun:"updated_at"`
}

func (m *elementModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return applyTenantScope(ctx, query, func(orgID uuid.UUID) {
		if m != nil && m.OrgID == nil {
			m.OrgID = &orgID
		}
	})
}

func (m *elementModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *elementModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *elementModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyTenantWhere(ctx, query)
}

type viewModel struct {
	bun.BaseModel `bun:"table:views"`

	ID             int64      `bun:"id,pk,autoincrement"`
	OrgID          *uuid.UUID `bun:"org_id,nullzero"`
	OwnerElementID *int64     `bun:"owner_element_id"`
	Name           string     `bun:"name"`
	Description    *string    `bun:"description"`
	LevelLabel     *string    `bun:"level_label"`
	Tags           string     `bun:"tags"`
	Level          int        `bun:"level"`
	DensityLevel   int        `bun:"density_level"`
	CreatedAt      string     `bun:"created_at"`
	UpdatedAt      string     `bun:"updated_at"`
}

func (m *viewModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return applyTenantScope(ctx, query, func(orgID uuid.UUID) {
		if m != nil && m.OrgID == nil {
			m.OrgID = &orgID
		}
	})
}

func (m *viewModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *viewModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *viewModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyTenantWhere(ctx, query)
}

type viewMarkdownModel struct {
	bun.BaseModel `bun:"table:view_markdown_documents"`

	ViewID     int64      `bun:"view_id,pk"`
	OrgID      *uuid.UUID `bun:"org_id,nullzero"`
	Path       string     `bun:"path"`
	IsManaged  bool       `bun:"is_managed"`
	SourceKind string     `bun:"source_kind"`
	CreatedAt  string     `bun:"created_at"`
	UpdatedAt  string     `bun:"updated_at"`
}

func (m *viewMarkdownModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return applyTenantScope(ctx, query, func(orgID uuid.UUID) {
		if m != nil && m.OrgID == nil {
			m.OrgID = &orgID
		}
	})
}

func (m *viewMarkdownModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *viewMarkdownModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *viewMarkdownModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyTenantWhere(ctx, query)
}

type visibilityOverrideModel struct {
	bun.BaseModel `bun:"table:view_visibility_overrides"`

	ViewID       int64  `bun:"view_id,pk"`
	ResourceType string `bun:"resource_type,pk"`
	ResourceID   int64  `bun:"resource_id,pk"`
	LevelDelta   int    `bun:"level_delta"`
	CreatedAt    string `bun:"created_at"`
	UpdatedAt    string `bun:"updated_at"`
}

func applyTenantScope(ctx context.Context, query bun.Query, setOrgID func(uuid.UUID)) error {
	orgID := TenantOrgIDFromCtx(ctx)
	if orgID == uuid.Nil {
		return nil
	}
	switch query.(type) {
	case *bun.InsertQuery:
		setOrgID(orgID)
	}
	return nil
}

func applyTenantWhere(ctx context.Context, query any) error {
	orgID := TenantOrgIDFromCtx(ctx)
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

func viewLayerFromModel(row viewLayerModel) ViewLayer {
	return ViewLayer{
		ID:        row.ID,
		DiagramID: row.ViewID,
		Name:      row.Name,
		Tags:      parseStrings(row.Tags),
		Color:     row.Color,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func tagFromModel(row tagModel) Tag {
	return Tag{Name: row.Name, Color: row.Color, Description: row.Description}
}

func elementPlacementFromModel(row elementPlacementModel) ElementPlacement {
	return ElementPlacement{
		ID:        row.ID,
		ViewID:    row.ViewID,
		ElementID: row.ElementID,
		PositionX: row.PositionX,
		PositionY: row.PositionY,
	}
}

func elementFromModel(row elementModel) LibraryElement {
	return LibraryElement{
		ID:                   row.ID,
		Name:                 row.Name,
		Kind:                 row.Kind,
		Description:          row.Description,
		Technology:           row.Technology,
		URL:                  row.URL,
		LogoURL:              row.LogoURL,
		TechnologyConnectors: parseTechnologyConnectors(row.TechnologyConnectors),
		Tags:                 parseStrings(row.Tags),
		Repo:                 row.Repo,
		Branch:               row.Branch,
		FilePath:             row.FilePath,
		Language:             row.Language,
		BypassNoiseGate:      row.BypassNoiseGate,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func viewRowFromModel(row viewModel) viewRow {
	var owner sql.NullInt64
	if row.OwnerElementID != nil {
		owner.Valid = true
		owner.Int64 = *row.OwnerElementID
	}
	return viewRow{
		ID:             row.ID,
		OwnerElementID: owner,
		Name:           row.Name,
		Description:    nullStringFromPtr(row.Description),
		LevelLabel:     nullStringFromPtr(row.LevelLabel),
		Tags:           row.Tags,
		Level:          row.Level,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func nullStringFromPtr(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func connectorFromModel(row connectorModel) Connector {
	return Connector{
		ID:              row.ID,
		ViewID:          row.ViewID,
		SourceElementID: row.SourceElementID,
		TargetElementID: row.TargetElementID,
		Label:           row.Label,
		Description:     row.Description,
		Relationship:    row.Relationship,
		Direction:       row.Direction,
		Style:           row.Style,
		URL:             row.URL,
		SourceHandle:    row.SourceHandle,
		TargetHandle:    row.TargetHandle,
		Tags:            parseStrings(row.Tags),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
