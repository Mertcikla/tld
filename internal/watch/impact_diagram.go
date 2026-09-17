package watch

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DiagramStyle selects how an impact report is projected onto a Mermaid graph.
//
// Deprecated: diagram styles were consolidated into a single reviewer
// projection (bounded selection, dependency lanes, compact change badges).
// All values normalize to DiagramStyleReview and BuildImpactDiagram ignores
// the style argument. The type and constructors are kept so existing callers
// keep compiling.
type DiagramStyle string

const (
	// DiagramStyleReview is the single reviewer projection: the bounded
	// selection grouped into dependency lanes, with changed elements carrying
	// a compact change badge instead of file paths. Every node remains an
	// authored element.
	DiagramStyleReview DiagramStyle = "review"
	// Deprecated: maps to the single reviewer projection.
	DiagramStyleFull DiagramStyle = "full"
	// Deprecated: maps to the single reviewer projection.
	DiagramStyleBounded DiagramStyle = "bounded"
	// Deprecated: maps to the single reviewer projection.
	DiagramStyleLanes DiagramStyle = "lanes"
	// Deprecated: maps to the single reviewer projection.
	DiagramStyleGroups DiagramStyle = "groups"
)

const (
	defaultDiagramNodeBudget = 10
	defaultDiagramEdgeBudget = 16
)

// NormalizeDiagramStyle parses a user-supplied style.
//
// Deprecated: every input normalizes to DiagramStyleReview.
func NormalizeDiagramStyle(_ string) DiagramStyle {
	return DiagramStyleReview
}

// DiagramGroup summarizes one authored-owner bucket in a grouped diagram.
//
// Deprecated: the grouped projection was removed; this type is kept for
// compatibility and is always empty.
type DiagramGroup struct {
	Name    string   `json:"name"`
	Touched int      `json:"touched"`
	Members []string `json:"members"`
}

// ImpactDiagram is the single reviewer projection of an impact report. Nodes
// and Edges are the subset actually drawn; OmittedNodes and OmittedEdges
// account for the confirmed context the projection dropped.
type ImpactDiagram struct {
	Style         DiagramStyle
	Code          string
	Nodes         []ImpactElement
	Edges         []ImpactEdge
	OmittedNodes  int
	OmittedEdges  int
	Groups        []DiagramGroup
	InternalEdges int
}

// BuildImpactDiagram projects a report into the single reviewer diagram:
// the bounded selection (changed elements plus their most connected
// neighbors) grouped into dependency lanes. Candidates are never promoted
// into confirmed context. The style argument is ignored and kept only for
// compatibility.
func BuildImpactDiagram(report ImpactReport, _ DiagramStyle) ImpactDiagram {
	changedRefs := diagramRefSet(report.Changed)
	nodes, edges, omittedNodes, omittedEdges := boundedDiagram(report, defaultDiagramNodeBudget, defaultDiagramEdgeBudget)
	return ImpactDiagram{
		Style:        DiagramStyleReview,
		Code:         renderImpactDiagram(report, nodes, edges, changedRefs, true, changeBadges(report, nodes, changedRefs), omittedNodes, omittedEdges),
		Nodes:        nodes,
		Edges:        edges,
		OmittedNodes: omittedNodes,
		OmittedEdges: omittedEdges,
	}
}

// changeBadges computes a compact change badge per changed element, e.g.
// "3 files (+24 -5)". File paths and line deltas live in the report body and
// side panels; the diagram carries only the aggregate so node labels stay
// legible.
func changeBadges(report ImpactReport, nodes []ImpactElement, changed map[string]struct{}) map[string]string {
	stats := make(map[string]ChangedFile, len(report.ChangedFiles))
	for _, file := range report.ChangedFiles {
		stats[file.Path] = file
	}
	out := map[string]string{}
	for _, node := range nodes {
		if _, ok := changed[node.Ref]; !ok {
			continue
		}
		if badge := changeBadge(node, stats); badge != "" {
			out[node.Ref] = badge
		}
	}
	return out
}

func changeBadge(node ImpactElement, stats map[string]ChangedFile) string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(node.Evidence))
	for _, evidence := range node.Evidence {
		// "contains" evidence trails ancestor rollup, not owned files, and
		// must not inflate the badge.
		if evidence.Kind == "contains" {
			continue
		}
		file := normalizeCodePath(evidence.Path)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		paths = append(paths, file)
	}
	if len(paths) == 0 {
		return ""
	}
	added, removed := 0, 0
	uniformChange := ""
	for _, file := range paths {
		stat := stats[file]
		added += stat.Added
		removed += stat.Removed
		change := strings.ToLower(strings.TrimSpace(stat.Change))
		if uniformChange == "" {
			uniformChange = change
		} else if uniformChange != change {
			uniformChange = "mixed"
		}
	}
	files := "1 file"
	if len(paths) > 1 {
		files = fmt.Sprintf("%d files", len(paths))
	}
	switch {
	case added+removed > 0:
		return fmt.Sprintf("%s (+%d -%d)", files, added, removed)
	case uniformChange == "added":
		return fmt.Sprintf("%s (added)", files)
	case uniformChange == "deleted":
		return fmt.Sprintf("%s (deleted)", files)
	default:
		return files
	}
}

// boundedDiagram keeps every changed element, ranks related neighbors by
// distinct changed neighbors, then observed relationship count, then ref, and
// spends the remaining node/edge budget in that order.
func boundedDiagram(report ImpactReport, nodeBudget, edgeBudget int) (nodes []ImpactElement, edges []ImpactEdge, omittedNodes, omittedEdges int) {
	changed := sortedDiagramNodes(report.Changed)
	changedRefs := diagramRefSet(changed)
	related := make([]ImpactElement, 0, len(report.Related))
	for _, node := range sortedDiagramNodes(report.Related) {
		if _, ok := changedRefs[node.Ref]; ok {
			continue
		}
		related = append(related, node)
	}
	allNodes := append(append([]ImpactElement{}, changed...), related...)
	available := uniqueDiagramEdges(validDiagramEdges(allNodes, report.Edges))

	incident := func(ref string) []ImpactEdge {
		var out []ImpactEdge
		for _, edge := range available {
			switch {
			case edge.SourceRef == ref:
				if _, ok := changedRefs[edge.TargetRef]; ok {
					out = append(out, edge)
				}
			case edge.TargetRef == ref:
				if _, ok := changedRefs[edge.SourceRef]; ok {
					out = append(out, edge)
				}
			}
		}
		return out
	}

	type neighbor struct {
		node        ImpactElement
		connections []ImpactEdge
		touched     int
		observed    int
	}
	var ranked []neighbor
	for _, node := range related {
		connections := incident(node.Ref)
		if len(connections) == 0 {
			continue
		}
		touched := map[string]struct{}{}
		observed := 0
		for _, edge := range connections {
			if edge.SourceRef == node.Ref {
				touched[edge.TargetRef] = struct{}{}
			} else {
				touched[edge.SourceRef] = struct{}{}
			}
			if edge.Observed {
				observed++
			}
		}
		ranked = append(ranked, neighbor{node: node, connections: connections, touched: len(touched), observed: observed})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].touched != ranked[j].touched {
			return ranked[i].touched > ranked[j].touched
		}
		if ranked[i].observed != ranked[j].observed {
			return ranked[i].observed > ranked[j].observed
		}
		return ranked[i].node.Ref < ranked[j].node.Ref
	})
	if limit := nodeBudget - len(changed); limit < len(ranked) {
		if limit < 0 {
			limit = 0
		}
		ranked = ranked[:limit]
	}

	nodes = append([]ImpactElement{}, changed...)
	for _, entry := range ranked {
		nodes = append(nodes, entry.node)
	}

	eligible := validDiagramEdges(nodes, available)
	selected := map[string]ImpactEdge{}
	selectedOrder := make([]string, 0, edgeBudget)
	add := func(edge ImpactEdge) {
		key := diagramEdgeKey(edge)
		if _, ok := selected[key]; ok {
			return
		}
		selected[key] = edge
		selectedOrder = append(selectedOrder, key)
	}
	for _, edge := range eligible {
		if _, ok := changedRefs[edge.SourceRef]; !ok {
			continue
		}
		if _, ok := changedRefs[edge.TargetRef]; !ok {
			continue
		}
		add(edge)
	}
	for _, entry := range ranked {
		best := append([]ImpactEdge{}, entry.connections...)
		sort.SliceStable(best, func(i, j int) bool {
			if best[i].Observed != best[j].Observed {
				return best[i].Observed
			}
			return diagramEdgeKey(best[i]) < diagramEdgeKey(best[j])
		})
		if len(best) > 0 {
			add(best[0])
		}
	}
	rank := map[string]int{}
	for index, entry := range ranked {
		rank[entry.node.Ref] = index
	}
	rankOf := func(edge ImpactEdge) int {
		if index, ok := rank[edge.SourceRef]; ok {
			return index
		}
		if index, ok := rank[edge.TargetRef]; ok {
			return index
		}
		return -1
	}
	remaining := append([]ImpactEdge{}, eligible...)
	sort.SliceStable(remaining, func(i, j int) bool {
		if left, right := rankOf(remaining[i]), rankOf(remaining[j]); left != right {
			return left < right
		}
		if remaining[i].Observed != remaining[j].Observed {
			return remaining[i].Observed
		}
		return diagramEdgeKey(remaining[i]) < diagramEdgeKey(remaining[j])
	})
	for _, edge := range remaining {
		if len(selected) >= edgeBudget {
			break
		}
		add(edge)
	}

	edges = make([]ImpactEdge, 0, len(selectedOrder))
	for _, key := range selectedOrder {
		edges = append(edges, selected[key])
	}
	edges = uniqueDiagramEdges(edges)
	return nodes, edges, len(changed) + len(related) - len(nodes), len(available) - len(selected)
}

// laneFor classifies a node's dependency role relative to the changed set.
func laneFor(ref string, changed map[string]struct{}, edges []ImpactEdge) string {
	if _, ok := changed[ref]; ok {
		return "changed"
	}
	incoming, outgoing := false, false
	for _, edge := range edges {
		if edge.SourceRef == ref {
			if _, ok := changed[edge.TargetRef]; ok {
				incoming = true
			}
		}
		if edge.TargetRef == ref {
			if _, ok := changed[edge.SourceRef]; ok {
				outgoing = true
			}
		}
	}
	switch {
	case incoming && outgoing:
		return "both"
	case incoming:
		return "incoming"
	default:
		return "outgoing"
	}
}

// renderImpactDiagram writes the report header and the reviewer graph: the
// bounded selection grouped into dependency lanes, with changed nodes
// carrying a compact change badge. A non-nil detail map carries those badges
// and marks unlabelled observed edges.
func renderImpactDiagram(report ImpactReport, nodes []ImpactElement, edges []ImpactEdge, changed map[string]struct{}, lanes bool, detail map[string]string, omittedNodes, omittedEdges int) string {
	lines := make([]string, 0, len(nodes)+len(edges)+5)
	if report.Coverage.Applicable {
		lines = append(lines, fmt.Sprintf("%%%% coverage: %d%% (%s)", report.Coverage.Percent, report.Coverage.Confidence))
	}
	if omitted := omittedDiagramLine(omittedNodes, omittedEdges); omitted != "" {
		lines = append(lines, omitted)
	}
	lines = append(lines, renderImpactGraph(nodes, edges, changed, lanes, detail))
	return strings.Join(lines, "\n")
}

// omittedDiagramLine accounts for confirmed context the budget dropped so
// reviewers know the graph is trimmed. It is a Mermaid comment, invisible in
// the rendered graph but present in the source.
func omittedDiagramLine(omittedNodes, omittedEdges int) string {
	if omittedNodes < 0 {
		omittedNodes = 0
	}
	if omittedEdges < 0 {
		omittedEdges = 0
	}
	if omittedNodes == 0 && omittedEdges == 0 {
		return ""
	}
	parts := []string{}
	if omittedNodes > 0 {
		parts = append(parts, pluralizeDiagram(omittedNodes, "node"))
	}
	if omittedEdges > 0 {
		parts = append(parts, pluralizeDiagram(omittedEdges, "edge"))
	}
	return "%% +" + strings.Join(parts, ", +") + " omitted"
}

func pluralizeDiagram(count int, unit string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, unit)
	}
	return fmt.Sprintf("%d %ss", count, unit)
}

func renderImpactGraph(nodes []ImpactElement, edges []ImpactEdge, changed map[string]struct{}, lanes bool, detail map[string]string) string {
	lines := make([]string, 0, len(nodes)+len(edges)+8)
	if lanes {
		lines = append(lines, "flowchart LR")
	} else {
		lines = append(lines, "flowchart TD")
	}
	ids := map[string]string{}
	for index, node := range nodes {
		ids[node.Ref] = fmt.Sprintf("n%d", index+1)
	}
	nodeLine := func(node ImpactElement) string {
		label := strings.TrimSpace(node.Name)
		if label == "" {
			label = node.Ref
		}
		if annotation := strings.TrimSpace(detail[node.Ref]); annotation != "" {
			label += "<br/>" + annotation
		}
		return fmt.Sprintf("  %s[\"%s\"]", ids[node.Ref], escapeMermaidLabel(label))
	}

	if lanes {
		laneOrder := []struct{ key, title string }{
			{"incoming", "Incoming context"},
			{"changed", "Code touched"},
			{"outgoing", "Outgoing context"},
			{"both", "Bidirectional context"},
		}
		for _, lane := range laneOrder {
			members := make([]ImpactElement, 0, len(nodes))
			for _, node := range nodes {
				if laneFor(node.Ref, changed, edges) == lane.key {
					members = append(members, node)
				}
			}
			if len(members) == 0 {
				continue
			}
			lines = append(lines, fmt.Sprintf("  subgraph lane_%s[\"%s\"]", lane.key, lane.title), "    direction TB")
			for _, node := range members {
				lines = append(lines, "  "+nodeLine(node))
			}
			lines = append(lines, "  end", fmt.Sprintf("  style lane_%s fill:#172234,stroke:#42536a,color:#cbd5e0", lane.key))
		}
	} else {
		for _, node := range nodes {
			lines = append(lines, nodeLine(node))
		}
	}

	for _, edge := range edges {
		sourceID, ok := ids[edge.SourceRef]
		if !ok {
			continue
		}
		targetID, ok := ids[edge.TargetRef]
		if !ok {
			continue
		}
		arrow := "-->"
		switch {
		case edge.Observed:
			arrow = "-.->"
		case isContainmentEdge(edge):
			arrow = "--o"
		}
		label := strings.TrimSpace(edge.Label)
		if label == "" && detail != nil {
			switch {
			case edge.Observed:
				label = "observed"
			case isContainmentEdge(edge):
				label = "contains"
			}
		}
		if label != "" {
			lines = append(lines, fmt.Sprintf("  %s %s|%s| %s", sourceID, arrow, escapeMermaidLabel(label), targetID))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s %s", sourceID, arrow, targetID))
		}
	}

	changedIDs := make([]string, 0, len(changed))
	for _, node := range nodes {
		if _, ok := changed[node.Ref]; ok {
			changedIDs = append(changedIDs, ids[node.Ref])
		}
	}
	if len(changedIDs) > 0 {
		sort.Strings(changedIDs)
		lines = append(lines,
			fmt.Sprintf("  class %s changed", strings.Join(changedIDs, ",")),
			"  classDef changed fill:#fde68a,stroke:#b45309,stroke-width:2px,color:#78350f;")
	}
	return strings.Join(lines, "\n")
}

// isContainmentEdge reports whether an edge represents folder containment
// between bound elements (parent folder owns the child folder's path). The
// check keys on the reserved "contains" label so containment survives the
// persisted-run and proto shapes, which carry label and observed only.
func isContainmentEdge(edge ImpactEdge) bool {
	return !edge.Observed && strings.EqualFold(strings.TrimSpace(edge.Label), "contains")
}

func uniqueDiagramNodes(nodes []ImpactElement) []ImpactElement {
	seen := map[string]struct{}{}
	out := make([]ImpactElement, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := seen[node.Ref]; ok {
			continue
		}
		seen[node.Ref] = struct{}{}
		out = append(out, node)
	}
	return out
}

func sortedDiagramNodes(nodes []ImpactElement) []ImpactElement {
	out := uniqueDiagramNodes(nodes)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

func diagramRefSet(nodes []ImpactElement) map[string]struct{} {
	out := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		out[node.Ref] = struct{}{}
	}
	return out
}

func diagramEdgeKey(edge ImpactEdge) string {
	return edge.SourceRef + "\x00" + edge.TargetRef + "\x00" + strconv.FormatBool(edge.Observed) + "\x00" + edge.Label
}

func uniqueDiagramEdges(edges []ImpactEdge) []ImpactEdge {
	seen := map[string]struct{}{}
	out := make([]ImpactEdge, 0, len(edges))
	for _, edge := range edges {
		key := diagramEdgeKey(edge)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, edge)
	}
	sort.SliceStable(out, func(i, j int) bool { return diagramEdgeKey(out[i]) < diagramEdgeKey(out[j]) })
	return out
}

func validDiagramEdges(nodes []ImpactElement, edges []ImpactEdge) []ImpactEdge {
	refs := diagramRefSet(nodes)
	out := make([]ImpactEdge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := refs[edge.SourceRef]; !ok {
			continue
		}
		if _, ok := refs[edge.TargetRef]; !ok {
			continue
		}
		out = append(out, edge)
	}
	return out
}
