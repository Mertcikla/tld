package mermaid

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

// SceneExportOptions controls portable scene output. Display selection is
// performed by impact.SelectScene before export.
type SceneExportOptions struct {
	// IncludeMetadata prepends a %% tld-scene comment naming the repository
	// and comparison key so the block can be round-tripped.
	IncludeMetadata bool
}

// ExportImpactScene renders a repository change scene — the exact payload the
// canvas loads — as a Mermaid flowchart, one subgraph per retained view. Node
// ids, labels, connector arrows, and change link styles follow the same rules
// as the canvas overlay. Isolated container owners are represented by their
// nested child subgraph rather than a redundant standalone node.
// Membership is selected once by the impact pipeline. Rendering uses only the
// frozen scene, without consulting the live workspace or source facts.
func ExportImpactScene(scene *codeindexv1.ImpactScene, opts SceneExportOptions) string {
	if scene == nil {
		return "flowchart LR\n"
	}
	view := sceneViewer{scene: scene}
	view.index()

	lines := []string{"flowchart LR"}
	if opts.IncludeMetadata {
		parts := []string{"%% tld-scene"}
		appendEntry := func(key, value string) {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				parts = append(parts, key+"="+EscapeMetadataValue(trimmed))
			}
		}
		appendEntry("repo", scene.GetComparison().GetRepositoryId())
		appendEntry("key", scene.GetComparison().GetKey())
		lines = append(lines, strings.Join(parts, " "))
		if summary := scene.Summary; summary != nil {
			lines = append(lines, fmt.Sprintf("%%%% impact files=%d added=%d removed=%d complete=%t basis=%s binary=%d affected=%d neighbours=%d authored=%d containers=%d radius=%d", summary.Files, summary.KnownLinesAdded, summary.KnownLinesRemoved, summary.LineCountsComplete, summary.LineCountsBasis, summary.BinaryFiles, summary.AffectedPlacements, summary.DependencyNeighbours, summary.AuthoredContext, summary.Containers, scene.GetComparison().GetEffectiveRadius()))
		}
	}

	seen := map[int32]bool{}
	for _, root := range view.scene.GetTree() {
		if root == nil {
			continue
		}
		lines = append(lines, view.renderTree(root, 0, seen)...)
	}
	// Defensive: retained views absent from the tree (e.g. drill-down targets
	// resolved only through navigations) render at the top level.
	for _, id := range view.retainedOrder {
		if view.retained[id] && !seen[id] {
			seen[id] = true
			lines = append(lines, view.renderNested(id, 0, nil, seen)...)
		}
	}
	if len(view.linkStyles) > 0 {
		lines = append(lines, "")
		lines = append(lines, view.linkStyles...)
	}
	return strings.Join(lines, "\n") + "\n"
}

// sceneViewer renders the frozen membership and relationships without selection.
type sceneViewer struct {
	scene         *codeindexv1.ImpactScene
	overlayOf     map[int32]*codeindexv1.ImpactSceneOverlay
	linkStyles    []string
	linkIndex     int
	retained      map[int32]bool
	retainedOrder []int32
	connected     map[string]bool
}

func (v *sceneViewer) index() {
	v.overlayOf = map[int32]*codeindexv1.ImpactSceneOverlay{}
	v.connected = map[string]bool{}
	for key, content := range v.scene.GetViews() {
		viewID, _ := strconv.ParseInt(key, 10, 32)
		local := map[int32]bool{}
		for _, placement := range content.GetPlacements() {
			local[placement.GetElement().GetElementId()] = true
			if placement.GetOverlay() != nil {
				v.overlayOf[placement.GetElement().GetElementId()] = placement.GetOverlay()
			}
		}
		for _, connector := range content.GetConnectors() {
			if connector == nil {
				continue
			}
			v.connected[sceneNodeRef(int32(viewID), connector.SourceElementId)] = true
			targetViewID := int32(viewID)
			if !local[connector.TargetElementId] {
				if ref := content.ConnectorTargets[strconv.Itoa(int(connector.Id))]; ref != nil {
					targetViewID = ref.ViewId
				}
			}
			v.connected[sceneNodeRef(targetViewID, connector.TargetElementId)] = true
		}
	}
	v.retained = map[int32]bool{}
	var walk func([]*diagv1.View)
	walk = func(views []*diagv1.View) {
		for _, view := range views {
			if view == nil || v.retained[view.Id] {
				continue
			}
			v.retained[view.Id] = true
			v.retainedOrder = append(v.retainedOrder, view.Id)
			walk(view.Children)
		}
	}
	walk(v.scene.Tree)
	for key := range v.scene.Views {
		id, _ := strconv.ParseInt(key, 10, 32)
		if !v.retained[int32(id)] {
			v.retained[int32(id)] = true
			v.retainedOrder = append(v.retainedOrder, int32(id))
		}
	}
	sort.Slice(v.retainedOrder, func(i, j int) bool { return v.retainedOrder[i] < v.retainedOrder[j] })
}

// sceneNodeRef names a placement node. Negative (transient) ids render as
// neg<N> so refs stay valid identifiers on both renderers.
func sceneNodeRef(viewID, elementID int32) string {
	return "el_" + sceneNum(viewID) + "_" + sceneNum(elementID)
}

func sceneNum(id int32) string {
	if id < 0 {
		return "neg" + strconv.FormatInt(int64(-id), 10)
	}
	return strconv.FormatInt(int64(id), 10)
}

func sceneViewRef(viewID int32) string {
	return "view_" + sceneNum(viewID)
}

// sceneChangeWord mirrors the canvas overlay vocabulary.
func sceneChangeWord(change codeindexv1.ChangeKind) string {
	switch change {
	case codeindexv1.ChangeKind_CHANGE_KIND_ADDED:
		return "added"
	case codeindexv1.ChangeKind_CHANGE_KIND_REMOVED:
		return "removed"
	case codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED:
		return "modified"
	default:
		return "unchanged"
	}
}

// sceneProvenanceGlyph renders the recorded origin of a placement.
func (v *sceneViewer) sceneProvenanceGlyph(viewID int32, element *diagv1.PlacedElement) string {
	origin := codeindexv1.ImpactOrigin_IMPACT_ORIGIN_UNSPECIFIED
	for _, placement := range v.scene.Views[strconv.Itoa(int(viewID))].Placements {
		if placement.Element.ElementId == element.ElementId {
			origin = placement.Origin
			break
		}
	}
	switch origin {
	case codeindexv1.ImpactOrigin_IMPACT_ORIGIN_RAW:
		return "◇ "
	case codeindexv1.ImpactOrigin_IMPACT_ORIGIN_MAPPED:
		return "▦ "
	default:
		return ""
	}
}

func (v *sceneViewer) sceneNodeLabel(viewID int32, element *diagv1.PlacedElement) string {
	label := strings.TrimSpace(element.GetName())
	if label == "" {
		label = fmt.Sprintf("element %d", element.GetElementId())
	}
	label = v.sceneProvenanceGlyph(viewID, element) + label
	if overlay, ok := v.overlayOf[element.GetElementId()]; ok {
		if word := sceneChangeWord(overlay.GetChange()); word != "unchanged" {
			label += "<br/>" + word
			if overlay.LinesAdded != nil && overlay.LinesRemoved != nil {
				label += fmt.Sprintf(" +%d \u2212%d", overlay.GetLinesAdded(), overlay.GetLinesRemoved())
			}
		} else {
			label += "<br/>(context)"
		}
	}
	return label
}

// renderTree emits one retained tree node with its retained children nested
// inside, mirroring the source view hierarchy instead of flattening every
// view into sibling subgraphs. A non-retained node is transparent: its
// retained descendants lift to the current level. seen guards hand-built
// payloads against cycles.
func (v *sceneViewer) renderTree(node *diagv1.View, depth int, seen map[int32]bool) []string {
	if node == nil || seen[node.GetId()] {
		return nil
	}
	seen[node.GetId()] = true
	var kids []*diagv1.View
	for _, child := range node.GetChildren() {
		if child == nil || seen[child.GetId()] || !v.retained[child.GetId()] {
			continue
		}
		kids = append(kids, child)
	}
	if !v.retained[node.GetId()] {
		var lines []string
		for _, kid := range kids {
			lines = append(lines, v.renderTree(kid, depth, seen)...)
		}
		return lines
	}
	return v.renderNested(node.GetId(), depth, kids, seen)
}

// renderNested emits the selected placements, nested children, and recorded
// relationships of one view, with change arrows and link styles.
func (v *sceneViewer) renderNested(viewID int32, depth int, kids []*diagv1.View, seen map[int32]bool) []string {
	content := v.scene.GetViews()[strconv.FormatInt(int64(viewID), 10)]
	if content == nil {
		var lines []string
		for _, kid := range kids {
			lines = append(lines, v.renderTree(kid, depth, seen)...)
		}
		return lines
	}
	name := v.viewName(viewID)
	head := strings.Repeat("  ", depth)
	pad := strings.Repeat("  ", depth+1)
	lines := []string{"", fmt.Sprintf(`%ssubgraph %s["%s"]`, head, sceneViewRef(viewID), escapeMermaidLabel(name))}
	rendered := map[int32]bool{}
	for _, placement := range content.GetPlacements() {
		if placement == nil || placement.GetElement() == nil {
			continue
		}
		if v.redundantOwner(viewID, placement, kids, seen) {
			continue
		}
		elementID := placement.GetElement().GetElementId()
		rendered[elementID] = true
		lines = append(lines, fmt.Sprintf(`%s%s["%s"]`, pad, sceneNodeRef(viewID, elementID),
			escapeMermaidLabel(v.sceneNodeLabel(viewID, placement.GetElement()))))
	}
	for _, kid := range kids {
		lines = append(lines, v.renderTree(kid, depth+1, seen)...)
	}
	for _, connector := range content.GetConnectors() {
		if connector == nil {
			continue
		}
		source, target := connector.GetSourceElementId(), connector.GetTargetElementId()
		if !rendered[source] {
			continue
		}
		if !rendered[target] {
			if ref := content.ConnectorTargets[strconv.Itoa(int(connector.Id))]; ref != nil {
				line := v.sceneConnectorLine(viewID, source, ref.ViewId, target, connector.GetLabel(), connector.GetTags(), connector.GetDirection())
				lines = append(lines, pad+line)
				v.linkIndex++
			}
			continue
		}
		lines = append(lines, pad+v.sceneConnectorLine(viewID, source, viewID, target, connector.GetLabel(), connector.GetTags(), connector.GetDirection()))
		// Mermaid numbers linkStyle statements over every rendered link line
		// in order, so the counter advances for each emitted connector, not
		// just the changed ones styleLink records.
		v.linkIndex++
	}
	lines = append(lines, head+"end")
	return lines
}

// redundantOwner only folds structural containers into a child subgraph that
// will actually render here. Overlays and connector endpoints need their node.
// This is a Mermaid representation choice; the frozen scene stays unchanged.
func (v *sceneViewer) redundantOwner(viewID int32, placement *codeindexv1.ScenePlacement, kids []*diagv1.View, seen map[int32]bool) bool {
	elementID := placement.GetElement().GetElementId()
	if placement.GetRole() != codeindexv1.ImpactRole_IMPACT_ROLE_CONTAINER ||
		v.overlayOf[elementID] != nil || v.connected[sceneNodeRef(viewID, elementID)] {
		return false
	}
	for _, kid := range kids {
		if !seen[kid.Id] && kid.OwnerElementId != nil && kid.GetOwnerElementId() == elementID &&
			v.scene.Views[strconv.Itoa(int(kid.Id))] != nil {
			return true
		}
	}
	return false
}

func (v *sceneViewer) viewName(viewID int32) string {
	name := v.viewNameIn(v.scene.GetTree(), viewID)
	if name == "" {
		name = fmt.Sprintf("view %d", viewID)
	}
	return name
}

// sceneConnectorLine mirrors the canvas edge language: user labels verbatim,
// thick arrows for added edges, crossed arrows for removed ones, and link
// styles carrying the change palette (modified keeps its arrow, color does
// the talking).
func (v *sceneViewer) sceneConnectorLine(sourceView, source, targetView, target int32, label string, tags []string, direction string) string {
	from, to := sceneNodeRef(sourceView, source), sceneNodeRef(targetView, target)
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "backward" {
		from, to = to, from
	}
	both, none := direction == "both" || direction == "bidirectional", direction == "none"
	change := edgeChangeFromTags(tags)
	text := strings.TrimSpace(label)
	arrow := "-->"
	if both {
		arrow = "<-->"
	} else if none {
		arrow = "---"
	}
	switch change {
	case "added":
		arrow = "==>"
		if both {
			arrow = "<==>"
		} else if none {
			arrow = "==="
		}
		v.styleLink("#48bb78", "")
	case "removed":
		v.styleLink("#fc8181", "5 5")
		arrow = "--x"
		if both {
			arrow = "x--x"
		} else if none {
			arrow = "---"
		}
		if !both && !none {
			if text != "" {
				return fmt.Sprintf(`%s--x|"%s"|%s`, from, escapeMermaidLabel(text), to)
			}
			return fmt.Sprintf(`%s--x%s`, from, to)
		}
	case "modified":
		v.styleLink("#ecc94b", "")
	}
	if text != "" {
		if arrow == "-->" {
			return fmt.Sprintf(`%s -- "%s" --> %s`, from, escapeMermaidLabel(text), to)
		}
		return fmt.Sprintf(`%s %s|"%s"| %s`, from, arrow, escapeMermaidLabel(text), to)
	}
	return fmt.Sprintf("%s %s %s", from, arrow, to)
}

// styleLink records a linkStyle statement for the connector line just
// emitted. The index is the caller's rendered-link counter: it advances for
// every emitted connector (see the render loop), so a changed edge following
// unchanged ones still addresses its own line. No trailing semicolon: the
// mermaid grammar rejects it (and its own error message mangles the line
// beyond recognition).
func (v *sceneViewer) styleLink(color, dash string) {
	line := fmt.Sprintf("linkStyle %d stroke:%s", v.linkIndex, color)
	if dash != "" {
		line += ",stroke-dasharray:" + dash
	}
	v.linkStyles = append(v.linkStyles, line)
}

func (v *sceneViewer) viewNameIn(tree []*diagv1.View, viewID int32) string {
	for _, view := range tree {
		if view == nil {
			continue
		}
		if view.GetId() == viewID {
			return view.GetName()
		}
		if name := v.viewNameIn(view.GetChildren(), viewID); name != "" {
			return name
		}
	}
	return ""
}

// edgeChangeFromTags reads the scene connector change tag written at scene
// build time. Anything else carries no claim.
func edgeChangeFromTags(tags []string) string {
	for _, tag := range tags {
		switch tag {
		case "change:added":
			return "added"
		case "change:removed":
			return "removed"
		case "change:modified":
			return "modified"
		}
	}
	return ""
}
