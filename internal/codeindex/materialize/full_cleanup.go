package materialize

import (
	"database/sql"
	"errors"
	"strings"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

// A full map owns the source hierarchy inside its generated views. Additional
// placements in authored views remain intact when generated parents change.
func (m *mapMaterializer) removeFullPlacements(elementID, keepView int64) error {
	placements, err := m.ws.ListElementPlacements(m.ctx, elementID)
	if err != nil {
		return err
	}
	if m.fullGeneratedViews == nil {
		m.fullGeneratedViews = map[int64]bool{}
		for key, mapping := range m.byKey {
			if mapping.Kind == cstore.MappingView && (key == mapKeyPrefix+"view|"+m.repositoryKey() || strings.HasPrefix(key, mapKeyPrefix+"fact|") || strings.HasPrefix(key, mapKeyPrefix+"directory|") || strings.HasPrefix(key, mapKeyPrefix+"external") || strings.HasPrefix(key, mapKeyPrefix+"metadata|")) {
				m.fullGeneratedViews[mapping.ResourceID] = true
			}
		}
	}

	for _, p := range placements {
		if p.ViewID != keepView && m.fullGeneratedViews[p.ViewID] {
			if err := m.ws.DeletePlacement(m.ctx, p.ViewID, elementID); err != nil {
				return err
			}
			if m.placed[p.ViewID] != nil {
				delete(m.placed[p.ViewID], elementID)
			}
		}
	}
	return nil
}

// Older full maps created a child view for every leaf. Remove only mapped,
// empty generated views with their default presentation. Owner elements,
// authored content, renamed views, layers and connectors remain intact.
func (m *mapMaterializer) removeEmptyFullView(key, name string, owner int64) error {
	mapping, ok := m.byKey[key]
	if !ok || mapping.Kind != cstore.MappingView {
		return nil
	}
	view, err := m.ws.ViewByID(m.ctx, mapping.ResourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if view.OwnerElementID == nil || *view.OwnerElementID != owner || view.Name != name || view.Description != nil || view.Markdown != nil || (view.LevelLabel != nil && *view.LevelLabel != "Map") {
		return nil
	}
	for _, tag := range view.Tags {
		if tag != StaleTag {
			return nil
		}
	}
	placements, err := m.ws.ElementPlacements(m.ctx, view.ID)
	if err != nil {
		return err
	}
	if len(placements) > 0 {
		return nil
	}
	connectors, err := m.ws.Connectors(m.ctx, view.ID)
	if err != nil {
		return err
	}
	if len(connectors) > 0 {
		return nil
	}
	layers, err := m.ws.Layers(m.ctx, view.ID)
	if err != nil {
		return err
	}
	if len(layers) > 0 {
		return nil
	}
	if err := m.ws.DeleteView(m.ctx, view.ID); err != nil {
		return err
	}
	m.result.Pruned++
	return nil
}

// Retire the previous symbol projection without deleting indexed data or
// authored library elements. Only mapped symbol connectors and placements in
// generated views are removed; empty untouched child views can then disappear.
func (m *mapMaterializer) retireFullSymbolMap() error {
	for key, mapping := range m.byKey {
		if mapping.Kind == cstore.MappingConnector && strings.HasPrefix(key, mapKeyPrefix+"symbol-edge|") {
			if err := m.ws.DeleteConnector(m.ctx, mapping.ResourceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
	}
	for key, mapping := range m.byKey {
		if mapping.Kind != cstore.MappingElement || m.kept[key] {
			continue
		}
		if strings.HasPrefix(key, mapKeyPrefix+"fact|") || strings.HasPrefix(key, mapKeyPrefix+"external") || strings.HasPrefix(key, mapKeyPrefix+"metadata|") {
			if err := m.removeFullPlacements(mapping.ResourceID, 0); err != nil {
				return err
			}
		}
	}
	// Include file views: files are leaves even if older maps gave them children.
	for key, mapping := range m.byKey {
		if mapping.Kind != cstore.MappingElement || (!strings.HasPrefix(key, mapKeyPrefix+"fact|") && !strings.HasPrefix(key, mapKeyPrefix+"external") && !strings.HasPrefix(key, mapKeyPrefix+"metadata|")) {
			continue
		}
		element, err := m.ws.ElementByID(m.ctx, mapping.ResourceID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		viewKey := key + "|view"
		name := element.Name
		// Old file views used full paths before the element's label was shortened.
		if mapping, ok := m.byKey[viewKey]; ok {
			view, err := m.ws.ViewByID(m.ctx, mapping.ResourceID)
			if err == nil && element.FilePath != nil && view.Name == *element.FilePath {
				name = view.Name
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if err := m.removeEmptyFullView(viewKey, name, element.ID); err != nil {
			return err
		}
	}
	return nil
}
