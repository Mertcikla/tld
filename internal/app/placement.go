package app

import "context"

func (s *Store) Placements(ctx context.Context, viewID int64) ([]PlacedElement, error) {
	type placementRow struct {
		item    PlacedElement
		techRaw string
		tagRaw  string
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.view_id, p.element_id, p.position_x, p.position_y,
		       e.name, e.kind, e.description, e.technology, e.url, e.logo_url, e.technology_connectors, e.tags, e.repo, e.branch, e.file_path, e.language, e.created_at, e.updated_at
		FROM placements p
		JOIN elements e ON e.id = p.element_id
		WHERE p.view_id = ?
		ORDER BY p.id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scanned := make([]placementRow, 0)
	for rows.Next() {
		var row placementRow
		if err := rows.Scan(&row.item.ID, &row.item.ViewID, &row.item.ElementID, &row.item.PositionX, &row.item.PositionY,
			&row.item.Name, &row.item.Kind, &row.item.Description, &row.item.Technology, &row.item.URL, &row.item.LogoURL,
			&row.techRaw, &row.tagRaw, &row.item.Repo, &row.item.Branch, &row.item.FilePath, &row.item.Language, new(string), new(string)); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PlacedElement, 0, len(scanned))
	for _, row := range scanned {
		item := row.item
		item.TechnologyConnectors = parseTechnologyConnectors(row.techRaw)
		item.Tags = parseStrings(row.tagRaw)
		if meta, ok := viewMeta[item.ElementID]; ok {
			item.HasView = meta.hasView
			item.ViewLabel = meta.label
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) AllPlacements(ctx context.Context) ([]PlacedElement, error) {
	type placementRow struct {
		item    PlacedElement
		techRaw string
		tagRaw  string
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.view_id, p.element_id, p.position_x, p.position_y,
		       e.name, e.kind, e.description, e.technology, e.url, e.logo_url, e.technology_connectors, e.tags, e.repo, e.branch, e.file_path, e.language, e.created_at, e.updated_at
		FROM placements p
		JOIN elements e ON e.id = p.element_id
		ORDER BY p.view_id, p.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scanned := make([]placementRow, 0)
	for rows.Next() {
		var row placementRow
		if err := rows.Scan(&row.item.ID, &row.item.ViewID, &row.item.ElementID, &row.item.PositionX, &row.item.PositionY,
			&row.item.Name, &row.item.Kind, &row.item.Description, &row.item.Technology, &row.item.URL, &row.item.LogoURL,
			&row.techRaw, &row.tagRaw, &row.item.Repo, &row.item.Branch, &row.item.FilePath, &row.item.Language, new(string), new(string)); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	viewMeta, err := s.childViewMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PlacedElement, 0, len(scanned))
	for _, row := range scanned {
		item := row.item
		item.TechnologyConnectors = parseTechnologyConnectors(row.techRaw)
		item.Tags = parseStrings(row.tagRaw)
		if meta, ok := viewMeta[item.ElementID]; ok {
			item.HasView = meta.hasView
			item.ViewLabel = meta.label
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ElementPlacements(ctx context.Context, viewID int64) ([]ElementPlacement, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, view_id, element_id, position_x, position_y FROM placements WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]ElementPlacement, 0)
	for rows.Next() {
		var item ElementPlacement
		if err := rows.Scan(&item.ID, &item.ViewID, &item.ElementID, &item.PositionX, &item.PositionY); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AddPlacement(ctx context.Context, viewID, elementID int64, x, y float64) (ElementPlacement, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(view_id, element_id) DO UPDATE SET position_x = excluded.position_x, position_y = excluded.position_y, updated_at = excluded.updated_at`,
		viewID, elementID, x, y, now, now)
	if err != nil {
		return ElementPlacement{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, view_id, element_id, position_x, position_y FROM placements WHERE view_id = ? AND element_id = ?`, viewID, elementID)
	var item ElementPlacement
	if err := row.Scan(&item.ID, &item.ViewID, &item.ElementID, &item.PositionX, &item.PositionY); err != nil {
		return ElementPlacement{}, err
	}
	return item, nil
}

func (s *Store) UpdatePlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE placements SET position_x = ?, position_y = ?, updated_at = ? WHERE view_id = ? AND element_id = ?`, x, y, nowString(), viewID, elementID)
	return err
}

func (s *Store) DeletePlacement(ctx context.Context, viewID, elementID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM placements WHERE view_id = ? AND element_id = ?`, viewID, elementID)
	return err
}
