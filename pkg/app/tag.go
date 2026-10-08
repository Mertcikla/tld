package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mertcikla/tld/v2/internal/tagcolors"
	"github.com/uptrace/bun"
)

type Tag struct {
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
}

func (s *Store) Layers(ctx context.Context, viewID int64) ([]ViewLayer, error) {
	var rows []viewLayerModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Where("view_id = ?", viewID).
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]ViewLayer, 0, len(rows))
	for _, row := range rows {
		out = append(out, viewLayerFromModel(row))
	}
	return out, nil
}

func (s *Store) AllLayers(ctx context.Context) ([]ViewLayer, error) {
	var rows []viewLayerModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Order("view_id").
		Order("id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]ViewLayer, 0, len(rows))
	for _, row := range rows {
		out = append(out, viewLayerFromModel(row))
	}
	return out, nil
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
	row := &viewLayerModel{
		ViewID:    viewID,
		Name:      name,
		Tags:      jsonString(tags, "[]"),
		Color:     color,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.bun.NewInsert().Model(row).Exec(ctx)
	if err != nil {
		return ViewLayer{}, err
	}
	return s.LayerByID(ctx, row.ID)
}

func (s *Store) LayerByID(ctx context.Context, id int64) (ViewLayer, error) {
	var row viewLayerModel
	if err := s.bun.NewSelect().
		Model(&row).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ViewLayer{}, err
	}
	return viewLayerFromModel(row), nil
}

func (s *Store) UpdateLayer(ctx context.Context, id int64, patch ViewLayer) (ViewLayer, error) {
	if patch.Tags != nil {
		if err := s.ensureTagColors(ctx, patch.Tags); err != nil {
			return ViewLayer{}, err
		}
	}
	var row viewLayerModel
	q := s.bun.NewUpdate().
		Model(&row).
		Where("id = ?", id).
		Set("updated_at = ?", nowString()).
		Returning("*")
	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.Tags != nil {
		q = q.Set("tags = ?", jsonString(patch.Tags, "[]"))
	}
	if patch.Color != nil {
		q = q.Set("color = ?", patch.Color)
	}
	if err := q.Scan(ctx); err != nil {
		return ViewLayer{}, err
	}
	return viewLayerFromModel(row), nil
}

func (s *Store) DeleteLayer(ctx context.Context, id int64) error {
	_, err := s.bun.NewDelete().
		Model((*viewLayerModel)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (s *Store) Tags(ctx context.Context) (map[string]Tag, error) {
	var rows []tagModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Order("name").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make(map[string]Tag, len(rows))
	for _, row := range rows {
		out[row.Name] = tagFromModel(row)
	}
	return out, nil
}

func (s *Store) UpdateTag(ctx context.Context, name, color string, description *string) error {
	row := &tagModel{Name: name, Color: color, Description: description}
	_, err := s.bun.NewInsert().
		Model(row).
		On("CONFLICT(org_id, name) DO UPDATE").
		Set("color = excluded.color").
		Set("description = excluded.description").
		Exec(ctx)
	return err
}

// DeleteTag removes a tag from the tags table and strips it from all resources that store tag arrays.
func (s *Store) DeleteTag(ctx context.Context, name string) error {
	return s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := stripElementTag(ctx, tx, name); err != nil {
			return err
		}
		if err := stripViewTag(ctx, tx, name); err != nil {
			return err
		}
		if err := stripConnectorTag(ctx, tx, name); err != nil {
			return err
		}
		if err := stripLayerTag(ctx, tx, name); err != nil {
			return err
		}
		if _, err := tx.NewDelete().
			Model((*tagModel)(nil)).
			Where("name = ?", name).
			Exec(ctx); err != nil {
			return err
		}
		return nil
	})
}

func stripElementTag(ctx context.Context, tx bun.Tx, name string) error {
	var rows []elementModel
	if err := tx.NewSelect().Model(&rows).Column("id", "tags").Scan(ctx); err != nil {
		return err
	}
	for _, row := range rows {
		next, changed := stripStoredTag(row.Tags, name)
		if !changed {
			continue
		}
		if _, err := tx.NewUpdate().
			Model((*elementModel)(nil)).
			Set("tags = ?", next).
			Where("id = ?", row.ID).
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func stripViewTag(ctx context.Context, tx bun.Tx, name string) error {
	var rows []viewModel
	if err := tx.NewSelect().Model(&rows).Column("id", "tags").Scan(ctx); err != nil {
		return err
	}
	for _, row := range rows {
		next, changed := stripStoredTag(row.Tags, name)
		if !changed {
			continue
		}
		if _, err := tx.NewUpdate().
			Model((*viewModel)(nil)).
			Set("tags = ?", next).
			Where("id = ?", row.ID).
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func stripConnectorTag(ctx context.Context, tx bun.Tx, name string) error {
	var rows []connectorModel
	if err := tx.NewSelect().Model(&rows).Column("id", "tags").Scan(ctx); err != nil {
		return err
	}
	for _, row := range rows {
		next, changed := stripStoredTag(row.Tags, name)
		if !changed {
			continue
		}
		if _, err := tx.NewUpdate().
			Model((*connectorModel)(nil)).
			Set("tags = ?", next).
			Where("id = ?", row.ID).
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func stripLayerTag(ctx context.Context, tx bun.Tx, name string) error {
	var rows []viewLayerModel
	if err := tx.NewSelect().Model(&rows).Column("id", "tags").Scan(ctx); err != nil {
		return err
	}
	for _, row := range rows {
		next, changed := stripStoredTag(row.Tags, name)
		if !changed {
			continue
		}
		if _, err := tx.NewUpdate().
			Model((*viewLayerModel)(nil)).
			Set("tags = ?", next).
			Where("id = ?", row.ID).
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func stripStoredTag(raw, name string) (string, bool) {
	tags, valid := parseStoredTagList(raw)
	filtered := tags[:0]
	removed := false
	for _, tag := range tags {
		if tag == name {
			removed = true
			continue
		}
		filtered = append(filtered, tag)
	}
	if !valid || removed {
		return jsonString(filtered, "[]"), true
	}
	return raw, false
}

func parseStoredTagList(raw string) ([]string, bool) {
	if raw == "" || raw == "null" {
		return []string{}, raw == "null"
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}, err == nil
	}
	return out, true
}

func (s *Store) pickUnusedColor(ctx context.Context, usedColors []string) string {
	return tagcolors.PickUnusedColor(usedColors)
}

func (s *Store) ensureTagColors(ctx context.Context, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	existingTags, err := s.Tags(ctx)
	if err != nil {
		return err
	}
	existing := map[string]struct{}{}
	var usedColors []string
	for name, tag := range existingTags {
		existing[name] = struct{}{}
		usedColors = append(usedColors, tag.Color)
	}
	for _, name := range tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := existing[name]; ok {
			continue
		}
		color := tagcolors.PickUnusedColor(usedColors)
		if err := s.insertTagIfMissing(ctx, name, color); err != nil {
			return err
		}
		usedColors = append(usedColors, color)
		existing[name] = struct{}{}
	}
	return nil
}

func (s *Store) insertTagIfMissing(ctx context.Context, name, color string) error {
	_, err := s.bun.NewInsert().
		Model(&tagModel{Name: name, Color: color}).
		On("CONFLICT (org_id, name) DO NOTHING").
		Exec(ctx)
	return err
}
