package app

import "context"

func (s *Store) Connectors(ctx context.Context, viewID int64) ([]Connector, error) {
	var rows []connectorModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Where("view_id = ?", viewID).
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]Connector, 0, len(rows))
	for _, row := range rows {
		out = append(out, connectorFromModel(row))
	}
	return out, nil
}

func (s *Store) AllConnectors(ctx context.Context) ([]Connector, error) {
	var rows []connectorModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Order("view_id").
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]Connector, 0, len(rows))
	for _, row := range rows {
		out = append(out, connectorFromModel(row))
	}
	return out, nil
}

func (s *Store) CreateConnector(ctx context.Context, input Connector) (Connector, error) {
	if err := s.ensureTagColors(ctx, input.Tags); err != nil {
		return Connector{}, err
	}
	now := nowString()
	row := &connectorModel{
		ViewID:          input.ViewID,
		SourceElementID: input.SourceElementID,
		TargetElementID: input.TargetElementID,
		Label:           input.Label,
		Description:     input.Description,
		Relationship:    input.Relationship,
		Direction:       normalizeDirection(&input.Direction),
		Style:           input.Style,
		URL:             input.URL,
		SourceHandle:    input.SourceHandle,
		TargetHandle:    input.TargetHandle,
		Tags:            jsonString(input.Tags, "[]"),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := s.bun.NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return Connector{}, err
	}
	return s.ConnectorByID(ctx, row.ID)
}

func (s *Store) ConnectorByID(ctx context.Context, id int64) (Connector, error) {
	var row connectorModel
	if err := s.bun.NewSelect().
		Model(&row).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return Connector{}, err
	}
	return connectorFromModel(row), nil
}

func (s *Store) UpdateConnector(ctx context.Context, id int64, patch Connector) (Connector, error) {
	var row connectorModel
	if err := s.bun.NewSelect().Model(&row).Where("id = ?", id).Scan(ctx); err != nil {
		return Connector{}, err
	}
	if patch.Tags != nil {
		if err := s.ensureTagColors(ctx, patch.Tags); err != nil {
			return Connector{}, err
		}
		row.Tags = jsonString(patch.Tags, "[]")
	}
	if patch.ViewID != 0 {
		row.ViewID = patch.ViewID
	}
	if patch.SourceElementID != 0 {
		row.SourceElementID = patch.SourceElementID
	}
	if patch.TargetElementID != 0 {
		row.TargetElementID = patch.TargetElementID
	}
	if patch.Label != nil {
		row.Label = patch.Label
	}
	if patch.Description != nil {
		row.Description = patch.Description
	}
	if patch.Relationship != nil {
		row.Relationship = patch.Relationship
	}
	if patch.Direction != "" {
		row.Direction = patch.Direction
	}
	if patch.Style != "" {
		row.Style = patch.Style
	}
	if patch.URL != nil {
		row.URL = patch.URL
	}
	if patch.SourceHandle != nil {
		row.SourceHandle = patch.SourceHandle
	}
	if patch.TargetHandle != nil {
		row.TargetHandle = patch.TargetHandle
	}
	row.UpdatedAt = nowString()
	if err := s.bun.NewUpdate().
		Model(&row).
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return Connector{}, err
	}
	return connectorFromModel(row), nil
}

func (s *Store) DeleteConnector(ctx context.Context, id int64) error {
	_, err := s.bun.NewDelete().
		Model((*connectorModel)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
