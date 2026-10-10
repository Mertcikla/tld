package mermaid

import (
	"fmt"
	"strconv"
	"strings"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

// SceneExportOptions controls how ExportImpactScene renders a portable change
// scene. Plain mirrors the canvas plain toggle: hide placements without
// change impact, keeping only the container elements that reach impacted
// nested views.
type SceneExportOptions struct {
	// IncludeMetadata prepends a %% tld-scene comment naming the repository
	// and comparison key so the block can be round-tripped.
	IncludeMetadata bool
	Plain           bool
}

// ExportImpactScene renders a repository change scene — the exact payload the
// canvas loads — as a Mermaid flowchart, one subgraph per retained view. Node
// ids, labels, connector arrows, and change link styles follow the same rules
// as the canvas overlay, so the text diagram matches the UI node for node.
// Membership follows the grounded rule: authored views pinned, every directly
// changed file kept. With no authored views in the scene it degrades to the
// mapped views, exactly like the canvas. It is deliberately independent of
// any index or snapshot state.
func ExportImpactScene(scene *codeindexv1.ImpactScene, opts SceneExportOptions) string {
	if scene == nil {
		return "flowchart LR\n"
	}
	view := sceneViewer{scene: scene, plain: opts.Plain}
	view.index()

	lines := []string{"flowchart LR"}
	if opts.IncludeMetadata {
		parts := []string{"%% tld-scene"}
		appendEntry := func(key, value string) {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				parts = append(parts, key+"="+EscapeMetadataValue(trimmed))
			}
		}
		appendEntry("repo", scene.GetRepositoryId())
		appendEntry("key", scene.GetComparisonKey())
		lines = append(lines, strings.Join(parts, " "))
	}

	for _, id := range view.retainedOrder {
		lines = append(lines, view.renderView(id)...)
	}
	if len(view.linkStyles) > 0 {
		lines = append(lines, "")
		lines = append(lines, view.linkStyles...)
	}
	return strings.Join(lines, "\n") + "\n"
}

// sceneViewer carries the canvas membership rules over one scene render.
type sceneViewer struct {
	scene *codeindexv1.ImpactScene
	plain bool
	// degraded mirrors the canvas fallback: with no authored views in the
	// scene, membership follows the mapped rule instead of the grounded one.
	degraded bool
	// linkElements holds the container elements that own retained child views
	// (plain mode): structural only, never annotated as changes.
	linkElements map[int32]map[int32]bool

	authored   map[int32]bool
	overlayOf  map[int32]*codeindexv1.ImpactSceneOverlay
	linkIndex  int
	linkStyles []string

	retained      map[int32]bool
	retainedOrder []int32
	// drilldown holds child views pulled in because a retained element owns
	// them. They keep full output so nested nodes render instead of
	// collapsing to singular flat elements.
	drilldown map[int32]bool
}

func (v *sceneViewer) index() {
	v.authored = map[int32]bool{}
	for _, id := range v.scene.GetAuthoredViewIds() {
		v.authored[int32(id)] = true
	}
	v.overlayOf = map[int32]*codeindexv1.ImpactSceneOverlay{}
	for _, content := range v.scene.GetViews() {
		for _, placement := range content.GetPlacements() {
			if placement == nil || placement.GetOverlay() == nil {
				continue
			}
			v.overlayOf[placement.GetElement().GetElementId()] = placement.GetOverlay()
		}
	}
	v.degraded = len(v.scene.GetAuthoredViewIds()) == 0
	v.retained = map[int32]bool{}
	v.retainedOrder = nil
	v.drilldown = map[int32]bool{}
	v.keep(v.scene.GetTree())
	v.expandDrilldown()
	// Plain mode still needs the container elements that own retained child
	// views; without them the hierarchy can't be traversed.
	v.linkElements = map[int32]map[int32]bool{}
	if v.plain {
		for _, link := range v.scene.GetNavigations() {
			if link.GetRelationType() != "child" || link.ElementId == nil || !v.retained[link.GetToViewId()] {
				continue
			}
			set := v.linkElements[link.GetFromViewId()]
			if set == nil {
				set = map[int32]bool{}
				v.linkElements[link.GetFromViewId()] = set
			}
			set[link.GetElementId()] = true
		}
	}
}

func (v *sceneViewer) impacted(elementID int32) bool {
	_, ok := v.overlayOf[elementID]
	return ok
}

func (v *sceneViewer) isTransient(elementID int32) bool {
	return elementID < 0
}

func (v *sceneViewer) isFallback(view *diagv1.View) bool {
	return view.GetId() != 0 && int64(view.GetId()) == v.scene.GetFallbackViewId()
}

func (v *sceneViewer) viewHasTransient(viewID int32) bool {
	for _, placement := range v.scene.GetViews()[strconv.FormatInt(int64(viewID), 10)].GetPlacements() {
		if placement != nil && v.isTransient(placement.GetElement().GetElementId()) {
			return true
		}
	}
	return false
}

// keep ports the canvas tree filter: retired impact views always drop;
// authored keeps authored views and ancestors; grounded additionally retains
// the fallback Changes view and transient carriers pruned to change evidence;
// mapped keeps any view with children or shown placements.
func (v *sceneViewer) keep(tree []*diagv1.View) []int32 {
	var kept []int32
	for _, view := range tree {
		if view == nil {
			continue
		}
		if strings.Contains(view.GetName(), " impact · ") {
			continue
		}
		children := v.keep(view.GetChildren())
		id := view.GetId()
		if v.degraded {
			// No authored views: follow the mapped rule so the diagram still
			// shows the generated map instead of going empty.
			placements := v.scene.GetViews()[strconv.FormatInt(int64(id), 10)].GetPlacements()
			if len(children) == 0 && len(placements) == 0 {
				continue
			}
		} else if !v.isFallback(view) && !v.authored[id] && len(children) == 0 && !v.viewHasTransient(id) {
			continue
		}
		v.retained[id] = true
		v.retainedOrder = append(v.retainedOrder, id)
		kept = append(kept, id)
		_ = children
	}
	return kept
}

// expandDrilldown retains the child views owned by retained placements,
// recursively, so nested elements render instead of collapsing flat. It
// mirrors the canvas tree filter's drill-down expansion.
func (v *sceneViewer) expandDrilldown() {
	names := map[int32]string{}
	var walk func(tree []*diagv1.View)
	walk = func(tree []*diagv1.View) {
		for _, view := range tree {
			if view == nil {
				continue
			}
			names[view.GetId()] = view.GetName()
			walk(view.GetChildren())
		}
	}
	walk(v.scene.GetTree())
	childByElement := map[int32]int32{}
	for _, link := range v.scene.GetNavigations() {
		if link.GetRelationType() != "child" || link.ElementId == nil {
			continue
		}
		if _, ok := childByElement[link.GetElementId()]; !ok {
			childByElement[link.GetElementId()] = link.GetToViewId()
		}
	}
	queue := make([]int32, 0, len(v.retained))
	for id := range v.retained {
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		for _, placement := range v.scene.GetViews()[strconv.FormatInt(int64(from), 10)].GetPlacements() {
			if placement == nil || placement.GetElement() == nil {
				continue
			}
			child, ok := childByElement[placement.GetElement().GetElementId()]
			if !ok || v.retained[child] {
				continue
			}
			if strings.Contains(names[child], " impact · ") {
				continue
			}
			v.retained[child] = true
			v.drilldown[child] = true
			v.retainedOrder = append(v.retainedOrder, child)
			queue = append(queue, child)
		}
	}
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

// sceneProvenanceGlyph mirrors the canvas provenance chips: graph-augmented
// transients and map-generated nodes are marked, authored nodes are plain.
func (v *sceneViewer) sceneProvenanceGlyph(viewID, elementID int32) string {
	if v.isTransient(elementID) {
		return "◇ "
	}
	if v.authored[viewID] {
		return ""
	}
	return "▦ "
}

func (v *sceneViewer) sceneNodeLabel(viewID int32, element *diagv1.PlacedElement) string {
	label := strings.TrimSpace(element.GetName())
	if label == "" {
		label = fmt.Sprintf("element %d", element.GetElementId())
	}
	label = v.sceneProvenanceGlyph(viewID, element.GetElementId()) + label
	if overlay, ok := v.overlayOf[element.GetElementId()]; ok {
		if word := sceneChangeWord(overlay.GetChange()); word != "unchanged" {
			label += fmt.Sprintf("<br/>%s +%d \u2212%d", word, overlay.GetLinesAdded(), overlay.GetLinesRemoved())
		} else {
			label += "<br/>(context)"
		}
	}
	return label
}

// renderView emits one retained view: its placements (pruned to change
// evidence in non-authored grounded views, exactly like the canvas) and its
// connectors filtered to rendered endpoints, with change arrows and link
// styles for new/removed/modified edges.
func (v *sceneViewer) renderView(viewID int32) []string {
	content := v.scene.GetViews()[strconv.FormatInt(int64(viewID), 10)]
	if content == nil {
		return nil
	}
	pruned := !v.degraded && !v.authored[viewID] && !v.drilldown[viewID]
	name := v.viewName(viewID)
	lines := []string{"", fmt.Sprintf(`subgraph %s["%s"]`, sceneViewRef(viewID), escapeMermaidLabel(name))}
	rendered := map[int32]bool{}
	links := v.linkElements[viewID]
	for _, placement := range content.GetPlacements() {
		if placement == nil || placement.GetElement() == nil {
			continue
		}
		elementID := placement.GetElement().GetElementId()
		// Mirrors the canvas: plain keeps impacted placements plus the
		// container elements that reach impacted nested views; otherwise
		// non-authored views are pruned to change evidence, authored views
		// keep their full membership.
		if v.plain {
			if !v.impacted(elementID) && !links[elementID] {
				continue
			}
		} else if pruned && !v.isTransient(elementID) && !v.impacted(elementID) {
			continue
		}
		rendered[elementID] = true
		lines = append(lines, fmt.Sprintf(`  %s["%s"]`, sceneNodeRef(viewID, elementID),
			escapeMermaidLabel(v.sceneNodeLabel(viewID, placement.GetElement()))))
	}
	for _, connector := range content.GetConnectors() {
		if connector == nil {
			continue
		}
		source, target := connector.GetSourceElementId(), connector.GetTargetElementId()
		if !rendered[source] || !rendered[target] {
			continue
		}
		lines = append(lines, "  "+v.sceneConnectorLine(viewID, source, target, connector.GetLabel(), connector.GetTags()))
	}
	lines = append(lines, "end")
	return lines
}

func (v *sceneViewer) viewName(viewID int32) string {
	name := v.viewNameIn(v.scene.GetTree(), viewID)
	if name == "" {
		name = fmt.Sprintf("view %d", viewID)
	}
	return name
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

// sceneConnectorLine mirrors the canvas edge language: user labels verbatim,
// thick arrows for added edges, crossed arrows for removed ones, and link
// styles carrying the change palette (modified keeps its arrow, color does
// the talking).
func (v *sceneViewer) sceneConnectorLine(viewID, source, target int32, label string, tags []string) string {
	from, to := sceneNodeRef(viewID, source), sceneNodeRef(viewID, target)
	change := edgeChangeFromTags(tags)
	text := strings.TrimSpace(label)
	switch change {
	case "added":
		line := fmt.Sprintf(`%s ==> %s`, from, to)
		if text != "" {
			line = fmt.Sprintf(`%s ==>|"%s"| %s`, from, escapeMermaidLabel(text), to)
		}
		v.styleLink("#48bb78", "")
		return line
	case "removed":
		line := fmt.Sprintf(`%s--x%s`, from, to)
		if text != "" {
			line = fmt.Sprintf(`%s--x|"%s"|%s`, from, escapeMermaidLabel(text), to)
		}
		v.styleLink("#fc8181", "5 5")
		return line
	case "modified":
		v.styleLink("#ecc94b", "")
		if text != "" {
			return fmt.Sprintf(`%s -- "%s" --> %s`, from, escapeMermaidLabel(text), to)
		}
		return fmt.Sprintf("%s --> %s", from, to)
	default:
		if text != "" {
			return fmt.Sprintf(`%s -- "%s" --> %s`, from, escapeMermaidLabel(text), to)
		}
		return fmt.Sprintf("%s --> %s", from, to)
	}
}

// styleLink records a linkStyle statement for the connector line just
// emitted. Indices count emitted connector lines across views, in order. No
// trailing semicolon: the mermaid grammar rejects it (and its own error
// message mangles the line beyond recognition).
func (v *sceneViewer) styleLink(color, dash string) {
	line := fmt.Sprintf("linkStyle %d stroke:%s", v.linkIndex, color)
	if dash != "" {
		line += ",stroke-dasharray:" + dash
	}
	v.linkStyles = append(v.linkStyles, line)
	v.linkIndex++
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
