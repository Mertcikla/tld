package materialize

import (
	"fmt"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// upsertLayer creates or refreshes a generated view layer. On update the user's
// layer name and color are preserved; only membership tags are refreshed.
func (m *mapMaterializer) upsertLayer(logicalKey string, viewID int64, name string, tags []string) error {
	m.kept[logicalKey] = true
	if mapping, ok := m.byKey[logicalKey]; ok && mapping.Kind == cstore.MappingLayer {
		if layer, err := m.ws.LayerByID(m.ctx, mapping.ResourceID); err == nil {
			if layer.DiagramID == viewID {
				if _, err := m.ws.UpdateLayer(m.ctx, mapping.ResourceID, core.ViewLayer{Tags: tags}); err == nil {
					m.recordMapping(logicalKey, cstore.MappingLayer, mapping.ResourceID)
					return nil
				}
			} else {
				// The layer's view changed (for example after re-nesting); drop
				// the old row and recreate it in the new view.
				_ = m.ws.DeleteLayer(m.ctx, mapping.ResourceID)
				_ = m.idx.DeleteMapping(m.ctx, logicalKey)
			}
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
