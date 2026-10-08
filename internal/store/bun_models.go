package store

import "github.com/uptrace/bun"

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
