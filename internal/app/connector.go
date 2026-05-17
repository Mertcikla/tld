package app

import "context"

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
	row := s.db.QueryRowContext(ctx, `
		UPDATE connectors SET
			view_id = CASE WHEN ? = 0 THEN view_id ELSE ? END,
			source_element_id = CASE WHEN ? = 0 THEN source_element_id ELSE ? END,
			target_element_id = CASE WHEN ? = 0 THEN target_element_id ELSE ? END,
			label = COALESCE(?, label),
			description = COALESCE(?, description),
			relationship = COALESCE(?, relationship),
			direction = COALESCE(NULLIF(?, ''), direction),
			style = COALESCE(NULLIF(?, ''), style),
			url = COALESCE(?, url),
			source_handle = COALESCE(?, source_handle),
			target_handle = COALESCE(?, target_handle),
			updated_at = ?
		WHERE id = ?
		RETURNING id, view_id, source_element_id, target_element_id, label, description, relationship, direction, style, url, source_handle, target_handle, created_at, updated_at`,
		patch.ViewID, patch.ViewID,
		patch.SourceElementID, patch.SourceElementID,
		patch.TargetElementID, patch.TargetElementID,
		patch.Label, patch.Description, patch.Relationship, patch.Direction, patch.Style, patch.URL, patch.SourceHandle, patch.TargetHandle,
		nowString(), id)
	var item Connector
	if err := row.Scan(&item.ID, &item.ViewID, &item.SourceElementID, &item.TargetElementID, &item.Label, &item.Description, &item.Relationship, &item.Direction, &item.Style, &item.URL, &item.SourceHandle, &item.TargetHandle, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Connector{}, err
	}
	return item, nil
}

func (s *Store) DeleteConnector(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM connectors WHERE id = ?`, id)
	return err
}
