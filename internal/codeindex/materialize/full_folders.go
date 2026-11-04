package materialize

import (
	"sort"
	"strings"

	cgraph "github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
)

type fullFolder struct {
	elementID  int64
	parentView int64
	parent     string
	depth      int
}

// Roll file dependencies up both branches. Each step connects containing
// folders, then lifts the deeper side (or both sides at equal depths). Stop at
// the shared branch: internal coupling must never become a folder self-edge or
// an edge from a folder to one of its own descendants.
func (m *mapMaterializer) materializeFullFolderConnectors(edges []MapEdge, byFile map[string]string, folders map[string]fullFolder) error {
	pairs := map[[2]string]bool{}
	for _, edge := range edges {
		from, fromOK := byFile[edge.FromFactID]
		to, toOK := byFile[edge.ToFactID]
		if !fromOK || !toOK {
			continue
		}
		for from != to {
			source, target := folders[from], folders[to]
			if source.elementID == 0 || target.elementID == 0 || strings.HasPrefix(from, to+"/") || strings.HasPrefix(to, from+"/") {
				break
			}
			pairs[[2]string{from, to}] = true
			if source.depth >= target.depth {
				from = source.parent
			}
			if target.depth >= source.depth {
				to = target.parent
			}
		}
	}
	ordered := make([][2]string, 0, len(pairs))
	for pair := range pairs {
		ordered = append(ordered, pair)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i][0] != ordered[j][0] {
			return ordered[i][0] < ordered[j][0]
		}
		return ordered[i][1] < ordered[j][1]
	})
	for _, pair := range ordered {
		source, target := folders[pair[0]], folders[pair[1]]
		key := mapKeyPrefix + "folder-edge|" + m.repositoryKey() + "|" + cgraph.ID(pair[0], pair[1])
		if err := m.upsertConnector(key, core.Connector{ViewID: source.parentView, SourceElementID: source.elementID, TargetElementID: target.elementID, Direction: "forward", Style: "bezier"}); err != nil {
			return err
		}
		if source.parentView == target.parentView {
			m.layoutEdges[source.parentView] = append(m.layoutEdges[source.parentView], layout.Connector{Source: source.elementID, Target: target.elementID})
		}
	}
	return nil
}
