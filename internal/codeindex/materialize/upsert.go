package materialize

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// StaleTag marks a retained resource absent from the latest applied map. This
// includes grouping/configuration changes, not just deleted source files.
const StaleTag = "codeindex:stale"

// reconcileMapStatus is the conservative upsert reconciliation boundary. It
// never removes resources, placements or ownership. Future mapping policies
// must retain this safety guarantee for authored data and shared resources.
// Status changes happen after materialization; interrupted runs keep ownership
// and can be retried. Layers retain their original membership (tags there are
// selectors, not status labels).
func (m *mapMaterializer) reconcileMapStatus() error {
	for key, mapping := range m.byKey {
		if !strings.HasPrefix(key, mapKeyPrefix) {
			continue
		}
		// A manually deleted resource may have been recreated under the same key.
		// Its replacement already has no stale tag.
		err := m.setResourceStale(mapping, !m.kept[key])
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("update map status %s: %w", key, err)
		}
	}
	return nil
}

func (m *mapMaterializer) setResourceStale(mapping cstore.ResourceMapping, stale bool) error {
	switch mapping.Kind {
	case cstore.MappingElement:
		item, err := m.ws.ElementByID(m.ctx, mapping.ResourceID)
		if err != nil {
			return err
		}
		tags := staleTags(item.Tags, stale)
		if !slices.Equal(item.Tags, tags) {
			_, err = m.ws.UpdateElement(m.ctx, item.ID, core.LibraryElement{Tags: tags})
		}
		return err
	case cstore.MappingView:
		item, err := m.ws.ViewByID(m.ctx, mapping.ResourceID)
		if err != nil {
			return err
		}
		tags := staleTags(item.Tags, stale)
		if !slices.Equal(item.Tags, tags) {
			_, err = m.ws.UpdateView(m.ctx, item.ID, nil, nil, nil, tags)
		}
		return err
	case cstore.MappingConnector:
		item, err := m.ws.ConnectorByID(m.ctx, mapping.ResourceID)
		if err != nil {
			return err
		}
		tags := staleTags(item.Tags, stale)
		if !slices.Equal(item.Tags, tags) {
			_, err = m.ws.UpdateConnector(m.ctx, item.ID, core.Connector{Tags: tags})
		}
		return err
	}
	return nil
}

func staleTags(tags []string, stale bool) []string {
	// Non-nil empty slices explicitly clear the final status tag in store patches.
	out := make([]string, 0, len(tags)+1)
	for _, tag := range tags {
		if tag != StaleTag {
			out = append(out, tag)
		}
	}
	if stale {
		out = append(out, StaleTag)
	}
	return out
}
