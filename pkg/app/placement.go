package app

import (
	"context"
	"errors"
	"github.com/uptrace/bun"

	"github.com/google/uuid"
)

type placementJoinRow struct {
	ID                   int64   `bun:"id"`
	ViewID               int64   `bun:"view_id"`
	ElementID            int64   `bun:"element_id"`
	PositionX            float64 `bun:"position_x"`
	PositionY            float64 `bun:"position_y"`
	Name                 string  `bun:"name"`
	Kind                 *string `bun:"kind"`
	Description          *string `bun:"description"`
	Technology           *string `bun:"technology"`
	URL                  *string `bun:"url"`
	LogoURL              *string `bun:"logo_url"`
	TechnologyConnectors string  `bun:"technology_connectors"`
	Tags                 string  `bun:"tags"`
	Repo                 *string `bun:"repo"`
	RepositoryID         *string `bun:"repository_id"`
	Branch               *string `bun:"branch"`
	FilePath             *string `bun:"file_path"`
	Language             *string `bun:"language"`
	BypassNoiseGate      bool    `bun:"bypass_noise_gate"`
}

func (s *Store) Placements(ctx context.Context, viewID int64) ([]PlacedElement, error) {
	var scanned []placementJoinRow
	query := s.bun.NewSelect().
		TableExpr("placements AS p").
		ColumnExpr("p.id, p.view_id, p.element_id, p.position_x, p.position_y").
		ColumnExpr("e.name, e.kind, e.description, e.technology, e.url, e.logo_url, e.technology_connectors, e.tags, e.repo, e.repository_id, e.branch, e.file_path, e.language, e.bypass_noise_gate").
		Join("JOIN elements AS e ON e.id = p.element_id").
		Where("p.view_id = ?", viewID).
		Order("p.id")
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		query = query.Where("e.org_id = ?", orgID)
	}
	if err := query.Scan(ctx, &scanned); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PlacedElement, 0, len(scanned))
	for _, row := range scanned {
		item := placedElementFromPlacementRow(row)
		if meta, ok := viewMeta[item.ElementID]; ok {
			item.HasView = meta.hasView
			item.ViewLabel = meta.label
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) AllPlacements(ctx context.Context) ([]PlacedElement, error) {
	var scanned []placementJoinRow
	query := s.bun.NewSelect().
		TableExpr("placements AS p").
		ColumnExpr("p.id, p.view_id, p.element_id, p.position_x, p.position_y").
		ColumnExpr("e.name, e.kind, e.description, e.technology, e.url, e.logo_url, e.technology_connectors, e.tags, e.repo, e.repository_id, e.branch, e.file_path, e.language, e.bypass_noise_gate").
		Join("JOIN elements AS e ON e.id = p.element_id").
		Order("p.view_id").
		Order("p.id")
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		query = query.Where("e.org_id = ?", orgID)
	}
	if err := query.Scan(ctx, &scanned); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PlacedElement, 0, len(scanned))
	for _, row := range scanned {
		item := placedElementFromPlacementRow(row)
		if meta, ok := viewMeta[item.ElementID]; ok {
			item.HasView = meta.hasView
			item.ViewLabel = meta.label
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ElementPlacements(ctx context.Context, viewID int64) ([]ElementPlacement, error) {
	var rows []elementPlacementModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Where("view_id = ?", viewID).
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]ElementPlacement, 0, len(rows))
	for _, row := range rows {
		out = append(out, elementPlacementFromModel(row))
	}
	return out, nil
}

func (s *Store) AddPlacement(ctx context.Context, viewID, elementID int64, x, y float64) (ElementPlacement, error) {
	now := nowString()
	placementExists, err := s.bun.NewSelect().
		Model((*elementPlacementModel)(nil)).
		Where("view_id = ?", viewID).
		Where("element_id = ?", elementID).
		Exists(ctx)
	if err != nil {
		return ElementPlacement{}, err
	}
	row := &elementPlacementModel{
		ViewID:    viewID,
		ElementID: elementID,
		PositionX: x,
		PositionY: y,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = s.bun.NewInsert().
		Model(row).
		On("CONFLICT(view_id, element_id) DO UPDATE").
		Set("position_x = excluded.position_x").
		Set("position_y = excluded.position_y").
		Set("updated_at = excluded.updated_at").
		Exec(ctx)
	if err != nil {
		return ElementPlacement{}, err
	}
	var got elementPlacementModel
	if err := s.bun.NewSelect().
		Model(&got).
		Where("view_id = ?", viewID).
		Where("element_id = ?", elementID).
		Scan(ctx); err != nil {
		return ElementPlacement{}, err
	}

	if !placementExists {
		// Auto-include related connectors when placing an element if the opposite
		// endpoint is already present in this view and the source connector exists
		// in another view.
		var related []connectorModel
		if err := s.bun.NewSelect().
			Model(&related).
			Column("source_element_id", "target_element_id", "label", "description", "relationship", "direction", "style", "url", "source_handle", "target_handle", "tags").
			Where("((source_element_id = ? AND target_element_id IN (SELECT element_id FROM placements WHERE view_id = ?)) OR (target_element_id = ? AND source_element_id IN (SELECT element_id FROM placements WHERE view_id = ?)))", elementID, viewID, elementID, viewID).
			Where("view_id != ?", viewID).
			Scan(ctx); err == nil {
			for _, c := range related {
				_, _ = s.CreateConnector(ctx, Connector{
					ViewID:          viewID,
					SourceElementID: c.SourceElementID,
					TargetElementID: c.TargetElementID,
					Label:           c.Label,
					Description:     c.Description,
					Relationship:    c.Relationship,
					Direction:       c.Direction,
					Style:           c.Style,
					URL:             c.URL,
					SourceHandle:    c.SourceHandle,
					TargetHandle:    c.TargetHandle,
					Tags:            parseStrings(c.Tags),
				})
			}
		}
	}

	return elementPlacementFromModel(got), nil
}

func (s *Store) UpdatePlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	_, err := s.bun.NewUpdate().
		Model((*elementPlacementModel)(nil)).
		Set("position_x = ?", x).
		Set("position_y = ?", y).
		Set("updated_at = ?", nowString()).
		Where("view_id = ?", viewID).
		Where("element_id = ?", elementID).
		Exec(ctx)
	return err
}

func (s *Store) DeletePlacement(ctx context.Context, viewID, elementID int64) error {
	_, err := s.bun.NewDelete().
		Model((*elementPlacementModel)(nil)).
		Where("view_id = ?", viewID).
		Where("element_id = ?", elementID).
		Exec(ctx)
	return err
}

func placedElementFromPlacementRow(row placementJoinRow) PlacedElement {
	return PlacedElement{
		ID:                   row.ID,
		ViewID:               row.ViewID,
		ElementID:            row.ElementID,
		PositionX:            row.PositionX,
		PositionY:            row.PositionY,
		Name:                 row.Name,
		Kind:                 row.Kind,
		Description:          row.Description,
		Technology:           row.Technology,
		URL:                  row.URL,
		LogoURL:              row.LogoURL,
		TechnologyConnectors: parseTechnologyConnectors(row.TechnologyConnectors),
		Tags:                 parseStrings(row.Tags),
		Repo:                 row.Repo,
		RepositoryID:         row.RepositoryID,
		Branch:               row.Branch,
		FilePath:             row.FilePath,
		Language:             row.Language,
		BypassNoiseGate:      row.BypassNoiseGate,
	}
}

// AddPlacements atomically upserts a bounded batch and copies each related
// connector once when at least one of its endpoints is newly placed.
func (s *Store) AddPlacements(ctx context.Context, viewID int64, inputs []ElementPlacement) error {
	if len(inputs) == 0 {
		return nil
	}
	if len(inputs) > 100 {
		return errors.New("placement batch exceeds 100")
	}
	return s.RunInTransaction(ctx, func(ctx context.Context, tx *Store) error {
		var ids []int64
		var rows []elementPlacementModel
		positions := map[int64]int{}
		now := nowString()
		for _, input := range inputs {
			if i, ok := positions[input.ElementID]; ok {
				rows[i].PositionX = input.PositionX
				rows[i].PositionY = input.PositionY
				continue
			}
			positions[input.ElementID] = len(rows)
			ids = append(ids, input.ElementID)
			rows = append(rows, elementPlacementModel{ViewID: viewID, ElementID: input.ElementID, PositionX: input.PositionX, PositionY: input.PositionY, CreatedAt: now, UpdatedAt: now})
		}
		var existing []elementPlacementModel
		if err := tx.bun.NewSelect().Model(&existing).Column("element_id").Where("view_id = ?", viewID).Where("element_id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
			return err
		}
		present := map[int64]bool{}
		for _, row := range existing {
			present[row.ElementID] = true
		}
		var added []int64
		for _, id := range ids {
			if !present[id] {
				added = append(added, id)
			}
		}
		if _, err := tx.bun.NewInsert().Model(&rows).On("CONFLICT(view_id, element_id) DO UPDATE").Set("position_x = excluded.position_x").Set("position_y = excluded.position_y").Set("updated_at = excluded.updated_at").Exec(ctx); err != nil {
			return err
		}
		if len(added) == 0 {
			return nil
		}
		var related []connectorModel
		if err := tx.bun.NewSelect().Model(&related).
			Where("view_id != ?", viewID).
			Where("source_element_id IN (SELECT element_id FROM placements WHERE view_id = ?)", viewID).
			Where("target_element_id IN (SELECT element_id FROM placements WHERE view_id = ?)", viewID).
			Where("(source_element_id IN (?) OR target_element_id IN (?))", bun.List(added), bun.List(added)).Order("id").Scan(ctx); err != nil {
			return err
		}
		for _, row := range related {
			input := connectorFromModel(row)
			input.ViewID = viewID
			if _, err := tx.CreateConnector(ctx, input); err != nil {
				return err
			}
		}
		return nil
	})
}
