package materialize

import (
	"database/sql"
	"errors"
	"fmt"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

// upsertLayer creates missing layers and preserves existing authored layers.
func (m *mapMaterializer) upsertLayer(logicalKey string, viewID int64, name string, tags []string) error {
	m.kept[logicalKey] = true
	if mapping, ok := m.byKey[logicalKey]; ok && mapping.Kind == cstore.MappingLayer {
		if _, err := m.ws.LayerByID(m.ctx, mapping.ResourceID); err == nil {
			// Membership and ownership may have been edited; retain both.
			m.recordMapping(logicalKey, cstore.MappingLayer, mapping.ResourceID)
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	created, err := m.ws.CreateLayer(m.ctx, viewID, name, tags, nil)
	if err != nil {
		return fmt.Errorf("create map layer %q: %w", logicalKey, err)
	}
	m.mappingBuffers = append(m.mappingBuffers, cstore.ResourceMapping{
		LogicalKey:   logicalKey,
		Kind:         cstore.MappingLayer,
		ResourceID:   created.ID,
		RepositoryID: m.input.RepositoryID,
		SnapshotID:   m.input.SnapshotID,
	})
	return nil
}

func groupLayerKey(repositoryID, key string) string {
	return mapKeyPrefix + "grouplayer|" + repositoryID + "|" + key
}
