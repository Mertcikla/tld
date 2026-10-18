package mermaid

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

// SceneExportOptions controls Mermaid presentation independently of canvas data.
type SceneExportOptions struct {
	// Plain uses change-focused membership and omits hierarchy wrappers.
	Plain bool

	// Theme enables themed labels and styles. Nil preserves legacy presentation
	// for existing Go consumers. The CLI and server supply DefaultTheme.
	Theme *Theme
	// NoTheme emits no theme frontmatter, classDef, class, or linkStyle statements, retaining text
	// change markers so the unstyled diagram still communicates change status.
	NoTheme bool
	// IncludeMetadata prepends a %% tld-scene comment naming the repository
	// and comparison key so the block can be round-tripped.
	IncludeMetadata bool
	// SubgraphDirection pins layout inside views (LR or TB). Empty preserves
	// Mermaid's default layout. PR comments use LR to avoid wide fan-out rows.
	SubgraphDirection string
}

// ExportImpactScene renders a bounded Mermaid projection of a frozen scene.
// Declared connectors bring authored context into the radius; hierarchy does not.
// Required changes remain visible. Plain omits hierarchy wrappers. The input is
// never mutated and rendering never consults live workspace or source facts.
func ExportImpactScene(scene *codeindexv1.ImpactScene, opts SceneExportOptions) string {
	if scene == nil {
		return strings.Join(themedHeader(opts.Theme, opts.NoTheme), "\n") + "\n"
	}
	scene = SelectImpactScene(scene, scene.GetComparison().GetEffectiveRadius(), opts.Plain || scene.GetComparison().GetPlain())
	view := sceneViewer{plain: opts.Plain || scene.GetComparison().GetPlain(), scene: scene, theme: opts.Theme, noTheme: opts.NoTheme, classes: map[string][]string{}}
	if opts.SubgraphDirection == "LR" || opts.SubgraphDirection == "TB" {
		view.direction = opts.SubgraphDirection
	}
	view.index()

	lines := themedHeader(opts.Theme, opts.NoTheme)
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
	for _, state := range []string{"context", "modified", "added", "removed"} {
		if ids := view.classes[state]; len(ids) > 0 {
			style := themeStyle(view.theme.Nodes.style(state))
			if style != "" {
				lines = append(lines, "classDef tld_"+state+" "+style,
					"class "+strings.Join(ids, ",")+" tld_"+state)
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// sceneViewer renders the frozen membership and relationships without selection.
type sceneViewer struct {
	plain         bool
	theme         *Theme
	noTheme       bool
	classes       map[string][]string
	direction     string
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

func (v *sceneViewer) sceneNodeLabel(element *diagv1.PlacedElement, separator string) string {
	label := strings.TrimSpace(element.GetName())
	if label == "" {
		label = fmt.Sprintf("element %d", element.GetElementId())
	}
	if overlay, ok := v.overlayOf[element.GetElementId()]; ok {
		showChange, showContext, showChurn := true, true, true
		if v.theme != nil && !v.noTheme {
			showChange, showContext, showChurn = v.theme.Labels.Change, v.theme.Labels.Context, v.theme.Labels.Churn
		}
		if word := sceneChangeWord(overlay.GetChange()); word != "unchanged" {
			var detail []string
			if showChange {
				detail = append(detail, word)
			}
			if showChurn && overlay.LinesAdded != nil && overlay.LinesRemoved != nil {
				addedColor, removedColor := "", ""
				if v.theme != nil && !v.noTheme {
					addedColor, removedColor = v.theme.Labels.AddedColor, v.theme.Labels.RemovedColor
				}
				if overlay.GetLinesAdded() > 0 {
					detail = append(detail, churnLabel("+", overlay.GetLinesAdded(), addedColor))
				}
				if overlay.GetLinesRemoved() > 0 {
					detail = append(detail, churnLabel("−", overlay.GetLinesRemoved(), removedColor))
				}
			}
			if len(detail) > 0 {
				label += separator + strings.Join(detail, " ")
			}
		} else if showContext {
			label += separator + "(context)"
		}
	}
	return label
}

func churnLabel(sign string, count uint32, color string) string {
	label := fmt.Sprintf("%s%d", sign, count)
	// Validate even for Go callers that construct Theme directly.
	if themeProperties["color"].MatchString(color) {
		return "<span style='color:" + color + "!important'>" + label + "</span>"
	}
	return label
}

func (v *sceneViewer) nodeState(element *diagv1.PlacedElement) string {
	change := sceneChangeWord(v.overlayOf[element.GetElementId()].GetChange())
	if change == "unchanged" {
		return "context"
	}
	return change
}

func (v *sceneViewer) styleNode(id, state string) {
	if v.theme != nil && !v.noTheme {
		v.classes[state] = append(v.classes[state], id)
	}
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
	var lines []string
	if !v.plain {
		lines = []string{"", fmt.Sprintf(`%ssubgraph %s["%s"]`, head, sceneViewRef(viewID), escapeMermaidLabel(name))}
	}
	if v.direction != "" && !v.plain {
		lines = append(lines, pad+"direction "+v.direction)
	}
	rendered := map[int32]bool{}
	var isolated []*diagv1.PlacedElement
	var placements []*codeindexv1.ScenePlacement
	isolatedCount := 0
	for _, placement := range content.GetPlacements() {
		if placement == nil || placement.GetElement() == nil {
			continue
		}
		if v.redundantOwner(viewID, placement, kids, seen) {
			continue
		}
		placements = append(placements, placement)
		if !v.connected[sceneNodeRef(viewID, placement.GetElement().GetElementId())] {
			isolatedCount++
		}
	}
	for _, placement := range placements {
		elementID := placement.GetElement().GetElementId()
		if isolatedCount >= 3 && !v.connected[sceneNodeRef(viewID, elementID)] {
			isolated = append(isolated, placement.GetElement())
			continue
		}
		rendered[elementID] = true
		lines = append(lines, fmt.Sprintf(`%s%s("%s")`, pad, sceneNodeRef(viewID, elementID),
			escapeMermaidLabel(v.sceneNodeLabel(placement.GetElement(), "<br/>"))))
		v.styleNode(sceneNodeRef(viewID, elementID), v.nodeState(placement.GetElement()))
	}
	if len(isolated) >= 3 {
		// A compact node must never combine different change colors. Legacy
		// and unstyled labels carry explicit status, so they can still share it.
		states := []string{""}
		if v.theme != nil && !v.noTheme {
			states = []string{"context", "modified", "added", "removed"}
		}
		for _, state := range states {
			var labels []string
			for _, element := range isolated {
				if state == "" || state == v.nodeState(element) {
					labels = append(labels, escapeMermaidLabel(v.sceneNodeLabel(element, " ")))
				}
			}
			if len(labels) == 0 {
				continue
			}
			id := sceneViewRef(viewID) + "_isolated"
			if state != "" {
				id += "_" + state
			}
			lines = append(lines, fmt.Sprintf(`%s%s("%s")`, pad, id, strings.Join(labels, "<br/>")))
			v.styleNode(id, state)
		}
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
	if !v.plain {
		lines = append(lines, head+"end")
	}
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
	if v.theme != nil && !v.noTheme {
		if style := themeStyle(v.theme.Edges.style(change)); style != "" {
			v.linkStyles = append(v.linkStyles, fmt.Sprintf("linkStyle %d %s", v.linkIndex, style))
		}
	}
	text := strings.TrimSpace(label)
	if v.noTheme && change != "" {
		text = strings.TrimSpace(text + " (" + change + ")")
	}
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
	if v.noTheme || v.theme != nil {
		return
	}
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
