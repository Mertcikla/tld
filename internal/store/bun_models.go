package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/uptrace/bun"
)

type placementLayoutModel struct {
	bun.BaseModel `bun:"table:placements"`

	ID        int64   `bun:"id,pk,autoincrement"`
	ViewID    int64   `bun:"view_id"`
	ElementID int64   `bun:"element_id"`
	PositionX float64 `bun:"position_x"`
	PositionY float64 `bun:"position_y"`
}

type connectorLayoutModel struct {
	bun.BaseModel `bun:"table:connectors"`

	ID              int64 `bun:"id,pk,autoincrement"`
	ViewID          int64 `bun:"view_id"`
	SourceElementID int64 `bun:"source_element_id"`
	TargetElementID int64 `bun:"target_element_id"`
}

type connectorInsertModel struct {
	bun.BaseModel `bun:"table:connectors"`

	ID              int64   `bun:"id,pk"`
	ViewID          int64   `bun:"view_id"`
	SourceElementID int64   `bun:"source_element_id"`
	TargetElementID int64   `bun:"target_element_id"`
	Label           *string `bun:"label"`
	Description     *string `bun:"description"`
	Relationship    *string `bun:"relationship"`
	Direction       string  `bun:"direction"`
	Style           string  `bun:"style"`
	URL             *string `bun:"url"`
	SourceHandle    *string `bun:"source_handle"`
	TargetHandle    *string `bun:"target_handle"`
	Tags            string  `bun:"tags"`
	CreatedAt       string  `bun:"created_at"`
	UpdatedAt       string  `bun:"updated_at"`
}

type countModel struct {
	bun.BaseModel `bun:"table:views"`

	ID int64 `bun:"id,pk,autoincrement"`
}

type elementCountModel struct {
	bun.BaseModel `bun:"table:elements"`

	ID int64 `bun:"id,pk,autoincrement"`
}

type connectorCountModel struct {
	bun.BaseModel `bun:"table:connectors"`

	ID int64 `bun:"id,pk,autoincrement"`
}

type visibilityOverrideModel struct {
	bun.BaseModel `bun:"table:view_visibility_overrides"`

	ViewID       int64      `bun:"view_id,pk"`
	ResourceType string     `bun:"resource_type,pk"`
	ResourceID   int64      `bun:"resource_id,pk"`
	OrgID        *uuid.UUID `bun:"org_id,nullzero"`
	LevelDelta   int        `bun:"level_delta"`
	CreatedAt    string     `bun:"created_at"`
	UpdatedAt    string     `bun:"updated_at"`
}

func (m *visibilityOverrideModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	orgID := app.TenantOrgIDFromCtx(ctx)
	if orgID == uuid.Nil {
		return nil
	}
	if _, ok := query.(*bun.InsertQuery); ok && m != nil && m.OrgID == nil {
		m.OrgID = &orgID
	}
	return nil
}

func (m *visibilityOverrideModel) BeforeSelect(ctx context.Context, query *bun.SelectQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *visibilityOverrideModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	return applyTenantWhere(ctx, query)
}

func (m *visibilityOverrideModel) BeforeDelete(ctx context.Context, query *bun.DeleteQuery) error {
	return applyTenantWhere(ctx, query)
}

func applyTenantWhere(ctx context.Context, query any) error {
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

func visibilityOverrideFromModel(row visibilityOverrideModel) VisibilityOverride {
	return VisibilityOverride{
		ViewID:       row.ViewID,
		ResourceType: row.ResourceType,
		ResourceID:   row.ResourceID,
		LevelDelta:   row.LevelDelta,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
