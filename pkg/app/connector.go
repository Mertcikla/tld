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
	if patch.Tags != nil {
		if err := s.ensureTagColors(ctx, patch.Tags); err != nil {
			return Connector{}, err
		}
	}
	var row connectorModel
	q := s.bun.NewUpdate().
		Model(&row).
		Where("id = ?", id).
		Set("updated_at = ?", nowString()).
		Returning("*")
	if patch.Tags != nil {
		q = q.Set("tags = ?", jsonString(patch.Tags, "[]"))
	}
	if patch.ViewID != 0 {
		q = q.Set("view_id = ?", patch.ViewID)
	}
	if patch.SourceElementID != 0 {
		q = q.Set("source_element_id = ?", patch.SourceElementID)
	}
	if patch.TargetElementID != 0 {
		q = q.Set("target_element_id = ?", patch.TargetElementID)
	}
	if patch.Label != nil {
		q = q.Set("label = ?", patch.Label)
	}
	if patch.Description != nil {
		q = q.Set("description = ?", patch.Description)
	}
	if patch.Relationship != nil {
		q = q.Set("relationship = ?", patch.Relationship)
	}
	if patch.Direction != "" {
		q = q.Set("direction = ?", patch.Direction)
	}
	if patch.Style != "" {
		q = q.Set("style = ?", patch.Style)
	}
	if patch.URL != nil {
		q = q.Set("url = ?", patch.URL)
	}
	if patch.SourceHandle != nil {
		q = q.Set("source_handle = ?", patch.SourceHandle)
	}
	if patch.TargetHandle != nil {
		q = q.Set("target_handle = ?", patch.TargetHandle)
	}
	if err := q.Scan(ctx); err != nil {
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
