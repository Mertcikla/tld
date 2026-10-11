package cmdutil

import (
	"strings"

	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// AutoPlace computes a non-overlapping, connector-aware canvas position for
// newRef inside parentRef's view. Existing placements act as fixed anchors and
// obstacles; connectors already in the view (plus any extra, such as a
// just-created connector that is not yet reflected in ws) steer the new node
// toward its neighbors. It reports ok=false when the workspace has nothing to
// lay the element out against, so the caller can keep its own default.
func AutoPlace(ws *workspace.Workspace, parentRef, newRef string, extra []workspace.Connector) (x, y float64, ok bool) {
	if ws == nil || newRef == "" {
		return 0, 0, false
	}
	parent := NormalizeParentRef(parentRef)

	keys := map[string]int64{}
	nextKey := int64(1)
	keyFor := func(ref string) int64 {
		if key, seen := keys[ref]; seen {
			return key
		}
		key := nextKey
		nextKey++
		keys[ref] = key
		return key
	}
	knownKey := func(ref string) (int64, bool) {
		if ref == newRef {
			return keyFor(newRef), true
		}
		if ws.Elements[ref] == nil {
			return 0, false
		}
		return keyFor(ref), true
	}

	newKey := keyFor(newRef)

	placements := make([]layout.Placement, 0, len(ws.Elements))
	for ref, element := range ws.Elements {
		if element == nil || ref == newRef {
			continue
		}
		for _, p := range element.Placements {
			if NormalizeParentRef(p.ParentRef) != parent {
				continue
			}
			placements = append(placements, layout.Placement{ElementID: keyFor(ref), X: p.PositionX, Y: p.PositionY})
			break
		}
	}

	connectors := make([]layout.Connector, 0, len(ws.Connectors)+len(extra))
	appendConnector := func(c *workspace.Connector) {
		if c == nil || NormalizeParentRef(c.View) != parent {
			return
		}
		source, sourceOK := knownKey(c.Source)
		target, targetOK := knownKey(c.Target)
		if !sourceOK || !targetOK {
			return
		}
		connectors = append(connectors, layout.Connector{Source: source, Target: target})
	}
	for _, c := range ws.Connectors {
		appendConnector(c)
	}
	for i := range extra {
		appendConnector(&extra[i])
	}

	targets := map[int64]struct{}{newKey: {}}
	result := layout.LayoutPlacements(placements, targets, connectors, false)
	position, found := result[newKey]
	if !found {
		return 0, 0, false
	}
	return position.X, position.Y, true
}

// FindPlacement returns the existing placement of ref in parentRef's view, or
// nil when the element has no placement there.
func FindPlacement(ws *workspace.Workspace, ref, parentRef string) *workspace.ViewPlacement {
	if ws == nil {
		return nil
	}
	element := ws.Elements[ref]
	if element == nil {
		return nil
	}
	parent := NormalizeParentRef(parentRef)
	for i := range element.Placements {
		if NormalizeParentRef(element.Placements[i].ParentRef) == parent {
			return &element.Placements[i]
		}
	}
	return nil
}

// NormalizeParentRef maps an empty parent to the synthetic root view.
func NormalizeParentRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return workspace.RootRef
	}
	return ref
}
