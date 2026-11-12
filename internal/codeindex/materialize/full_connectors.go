package materialize

import (
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"

	cgraph "github.com/mertcikla/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

func isFullDependencyKind(kind string) bool { return kind == "calls" || kind == "references" }

func fullDependencyKind(kind string) string {
	if isFullDependencyKind(kind) {
		return ""
	}
	return kind
}

// Reuse one previous connector so authored routes and descriptions survive the
// merge. Calls take precedence, independent of edge iteration order.
func (m *mapMaterializer) reuseFullDependency(key, from, to string) error {
	if mapping, ok := m.byKey[key]; ok {
		if _, err := m.ws.ConnectorByID(m.ctx, mapping.ResourceID); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	for _, kind := range []string{"calls", "references"} {
		oldKey := mapKeyPrefix + "file-edge|" + m.repositoryKey() + "|" + cgraph.ID(from, to, kind)
		mapping, ok := m.byKey[oldKey]
		if !ok || mapping.Kind != cstore.MappingConnector {
			continue
		}
		if _, err := m.ws.ConnectorByID(m.ctx, mapping.ResourceID); errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return err
		}
		mapping.LogicalKey = key
		m.byKey[key] = mapping
		return nil
	}
	return nil
}

// Old logical keys are retained as ownership aliases. A key aliasing a reused
// connector must stay current so stale reconciliation cannot mark it stale.
func (m *mapMaterializer) retireFullDependencyDuplicates() error {
	current := map[int64]bool{}
	for _, mapping := range m.mappingBuffers {
		if mapping.Kind == cstore.MappingConnector && m.kept[mapping.LogicalKey] {
			current[mapping.ResourceID] = true
		}
	}
	for key, mapping := range m.byKey {
		if mapping.Kind != cstore.MappingConnector || !strings.HasPrefix(key, mapKeyPrefix+"file-edge|") {
			continue
		}
		if current[mapping.ResourceID] {
			m.kept[key] = true
			continue
		}
		connector, err := m.ws.ConnectorByID(m.ctx, mapping.ResourceID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if (connector.Label != nil && isFullDependencyKind(*connector.Label)) || (connector.Relationship != nil && isFullDependencyKind(*connector.Relationship)) {
			if err := m.ws.DeleteConnector(m.ctx, connector.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// One wire can represent several source/target symbols. Sort and deduplicate
// each endpoint's declaration lines while retaining the file-pair identity.
func fullAnchoredEdges(input []MapEdge) []MapEdge {
	grouped := map[[3]string]*MapEdge{}
	out := []MapEdge{}
	for _, edge := range input {
		kind := fullDependencyKind(edge.Kind)
		key := [3]string{edge.FromFactID, edge.ToFactID, kind}
		item := grouped[key]
		if item == nil {
			item = &MapEdge{FromFactID: edge.FromFactID, ToFactID: edge.ToFactID, Kind: kind}
			grouped[key] = item
		}
		item.SourceLines = append(item.SourceLines, edge.SourceLines...)
		item.TargetLines = append(item.TargetLines, edge.TargetLines...)
	}
	for _, edge := range grouped {
		slices.Sort(edge.SourceLines)
		edge.SourceLines = slices.Compact(edge.SourceLines)
		slices.Sort(edge.TargetLines)
		edge.TargetLines = slices.Compact(edge.TargetLines)
		out = append(out, *edge)
	}
	return out
}

func fullLineHandle(side string, lines []uint32) string {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		if line > 0 {
			values = append(values, strconv.FormatUint(uint64(line), 10))
		}
	}
	if len(values) == 0 {
		return ""
	}
	return side + "-line-" + strings.Join(values, ",")
}

func canRefreshFullHandle(handle *string) bool {
	return handle == nil || (!strings.Contains(*handle, "-symbol-") && !strings.Contains(*handle, "-member-"))
}

func preserveFullLineHandle(saved *string, side string) string {
	if saved == nil {
		return side
	}
	if i := strings.Index(*saved, "-line-"); i >= 0 {
		chosen := strings.SplitN(side, "-", 2)[0]
		// Code previews expose semantic ports on their left and right edges only.
		if chosen != "left" && chosen != "right" {
			chosen = strings.SplitN(*saved, "-", 2)[0]
		}
		return chosen + (*saved)[i:]
	}
	return side
}

func refreshFullLineHandle(side string, saved *string, lines []uint32) string {
	if saved != nil {
		previous := strings.SplitN(*saved, "-", 2)[0]
		if previous == "left" || previous == "right" {
			side = previous
		}
	}
	return fullLineHandle(side, lines)
}
