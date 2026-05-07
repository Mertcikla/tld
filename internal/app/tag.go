package app

import (
	"context"
	"strings"

	"github.com/mertcikla/tld/internal/tagcolors"
)

type Tag struct {
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
}

func (s *Store) Layers(ctx context.Context, viewID int64) ([]ViewLayer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, view_id, name, tags, color, created_at, updated_at FROM view_layers WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]ViewLayer, 0)
	for rows.Next() {
		var rawTags string
		var item ViewLayer
		if err := rows.Scan(&item.ID, &item.DiagramID, &item.Name, &rawTags, &item.Color, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Tags = parseStrings(rawTags)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateLayer(ctx context.Context, viewID int64, name string, tags []string, color *string) (ViewLayer, error) {
	if err := s.ensureTagColors(ctx, tags); err != nil {
		return ViewLayer{}, err
	}

	if color == nil || strings.TrimSpace(*color) == "" {
		// User said pick unused, usually means relative to existing layers in the same view or global tags.
		// Frontend uses tagColors.
		tagsMap, err := s.Tags(ctx)
		if err != nil {
			return ViewLayer{}, err
		}
		var usedColors []string
		for _, t := range tagsMap {
			usedColors = append(usedColors, t.Color)
		}
		// Also consider existing layers colors
		layers, err := s.Layers(ctx, viewID)
		if err == nil {
			for _, l := range layers {
				if l.Color != nil {
					usedColors = append(usedColors, *l.Color)
				}
			}
		}
		c := s.pickUnusedColor(ctx, usedColors)
		color = &c
	}

	now := nowString()
	res, err := s.db.ExecContext(ctx, `INSERT INTO view_layers(view_id, name, tags, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		viewID, name, jsonString(tags, "[]"), color, now, now)
	if err != nil {
		return ViewLayer{}, err
	}
	id, _ := res.LastInsertId()
	return s.LayerByID(ctx, id)
}

func (s *Store) LayerByID(ctx context.Context, id int64) (ViewLayer, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, view_id, name, tags, color, created_at, updated_at FROM view_layers WHERE id = ?`, id)
	var rawTags string
	var item ViewLayer
	if err := row.Scan(&item.ID, &item.DiagramID, &item.Name, &rawTags, &item.Color, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ViewLayer{}, err
	}
	item.Tags = parseStrings(rawTags)
	return item, nil
}

func (s *Store) UpdateLayer(ctx context.Context, id int64, patch ViewLayer) (ViewLayer, error) {
	current, err := s.LayerByID(ctx, id)
	if err != nil {
		return ViewLayer{}, err
	}
	if patch.Name == "" {
		patch.Name = current.Name
	}
	if patch.Tags == nil {
		patch.Tags = current.Tags
	}
	if err := s.ensureTagColors(ctx, patch.Tags); err != nil {
		return ViewLayer{}, err
	}
	if patch.Color == nil {
		patch.Color = current.Color
	}
	_, err = s.db.ExecContext(ctx, `UPDATE view_layers SET name = ?, tags = ?, color = ?, updated_at = ? WHERE id = ?`, patch.Name, jsonString(patch.Tags, "[]"), patch.Color, nowString(), id)
	if err != nil {
		return ViewLayer{}, err
	}
	return s.LayerByID(ctx, id)
}

func (s *Store) DeleteLayer(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM view_layers WHERE id = ?`, id)
	return err
}

func (s *Store) Tags(ctx context.Context) (map[string]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, color, description FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]Tag{}
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.Color, &tag.Description); err != nil {
			return nil, err
		}
		out[tag.Name] = tag
	}
	return out, rows.Err()
}

func (s *Store) UpdateTag(ctx context.Context, name, color string, description *string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tags(name, color, description) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET color = excluded.color, description = excluded.description`,
		name, color, description)
	return err
}

func (s *Store) pickUnusedColor(ctx context.Context, usedColors []string) string {
	return tagcolors.PickUnusedColor(usedColors)
}

func (s *Store) ensureTagColors(ctx context.Context, tags []string) error {
	return tagcolors.Ensure(ctx, s.db, tags)
}
