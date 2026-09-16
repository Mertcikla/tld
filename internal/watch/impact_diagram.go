package watch

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DiagramStyle selects how an impact report is projected onto a Mermaid graph.
// The projection never changes the report; it only chooses which confirmed
// context is drawn.
type DiagramStyle string

const (
	// DiagramStyleFull draws every changed, candidate, and related element.
	DiagramStyleFull DiagramStyle = "full"
	// DiagramStyleBounded keeps changed elements and their most connected
	// neighbors within a soft node/edge budget.
	DiagramStyleBounded DiagramStyle = "bounded"
	// DiagramStyleLanes is the bounded selection grouped into incoming, outgoing,
	// and bidirectional dependency lanes.
	DiagramStyleLanes DiagramStyle = "lanes"
	// DiagramStyleGroups collapses the report into authored-owner buckets.
	DiagramStyleGroups DiagramStyle = "groups"
)

const (
	defaultDiagramNodeBudget = 10
	defaultDiagramEdgeBudget = 16
)

// NormalizeDiagramStyle parses a user-supplied style, defaulting to full.
func NormalizeDiagramStyle(value string) DiagramStyle {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(DiagramStyleBounded), "neighborhood":
		return DiagramStyleBounded
	case string(DiagramStyleLanes), "lane":
		return DiagramStyleLanes
	case string(DiagramStyleGroups), "grouped":
		return DiagramStyleGroups
	default:
		return DiagramStyleFull
	}
}

// DiagramGroup summarizes one authored-owner bucket in a grouped diagram.
type DiagramGroup struct {
	Name    string   `json:"name"`
	Touched int      `json:"touched"`
	Members []string `json:"members"`
}

// ImpactDiagram is a style-specific projection of an impact report. Nodes and
// Edges are the subset actually drawn; OmittedNodes and OmittedEdges account for
// the confirmed context the projection dropped. InternalEdges counts
// relationships collapsed inside a group.
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

// BuildImpactDiagram projects a report into the requested style. Candidates are
// never promoted into confirmed context: bounded, lanes, and groups draw only
// changed and related elements.
func BuildImpactDiagram(report ImpactReport, style DiagramStyle) ImpactDiagram {
	style = NormalizeDiagramStyle(string(style))
	changedRefs := diagramRefSet(report.Changed)
	switch style {
	case DiagramStyleGroups:
		nodes, edges, groups, internal, touched := groupedDiagram(report)
		return ImpactDiagram{
			Style:         style,
			Code:          renderImpactDiagram(report, nodes, edges, touched, false),
			Nodes:         nodes,
			Edges:         edges,
			Groups:        groups,
			InternalEdges: internal,
		}
	case DiagramStyleBounded, DiagramStyleLanes:
		nodes, edges, omittedNodes, omittedEdges := boundedDiagram(report, defaultDiagramNodeBudget, defaultDiagramEdgeBudget)
		return ImpactDiagram{
			Style:        style,
			Code:         renderImpactDiagram(report, nodes, edges, changedRefs, style == DiagramStyleLanes),
			Nodes:        nodes,
			Edges:        edges,
			OmittedNodes: omittedNodes,
			OmittedEdges: omittedEdges,
		}
	default:
		nodes := fullDiagramNodes(report)
		edges := validDiagramEdges(nodes, report.Edges)
		return ImpactDiagram{
			Style: DiagramStyleFull,
			Code:  renderImpactDiagram(report, nodes, edges, changedRefs, false),
			Nodes: nodes,
			Edges: edges,
		}
	}
}

func fullDiagramNodes(report ImpactReport) []ImpactElement {
	return uniqueDiagramNodes(append(append(append([]ImpactElement{}, report.Changed...), report.Candidates...), report.Related...))
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

// groupedDiagram collapses elements sharing an authored owner into one node.
// Unowned elements remain ungrouped.
func groupedDiagram(report ImpactReport) (nodes []ImpactElement, edges []ImpactEdge, groups []DiagramGroup, internalEdges int, touched map[string]struct{}) {
	source := sortedDiagramNodes(append(append([]ImpactElement{}, report.Changed...), report.Related...))
	changedRefs := diagramRefSet(report.Changed)
	refToGroup := map[string]string{}
	for _, node := range source {
		refToGroup[node.Ref] = ownerGroupRef(node)
	}

	buckets := map[string][]ImpactElement{}
	var bucketOrder []string
	for _, node := range source {
		ref := refToGroup[node.Ref]
		if _, ok := buckets[ref]; !ok {
			bucketOrder = append(bucketOrder, ref)
		}
		buckets[ref] = append(buckets[ref], node)
	}
	sort.Strings(bucketOrder)

	touched = map[string]struct{}{}
	for _, ref := range bucketOrder {
		members := buckets[ref]
		touchedCount := 0
		for _, member := range members {
			if _, ok := changedRefs[member.Ref]; ok {
				touchedCount++
			}
		}
		if touchedCount > 0 {
			touched[ref] = struct{}{}
		}
		name := members[0].Name
		if owner := strings.TrimSpace(members[0].Owner); owner != "" {
			name = owner
		}
		groups = append(groups, DiagramGroup{Name: name, Touched: touchedCount, Members: diagramMemberNames(members)})
		node := members[0]
		node.Ref = ref
		node.Name = fmt.Sprintf("%s (%d/%d touched)", name, touchedCount, len(members))
		nodes = append(nodes, node)
	}

	type aggregate struct {
		edge  ImpactEdge
		count int
	}
	aggregated := map[string]*aggregate{}
	var aggregateOrder []string
	for _, edge := range uniqueDiagramEdges(validDiagramEdges(source, report.Edges)) {
		sourceRef := refToGroup[edge.SourceRef]
		targetRef := refToGroup[edge.TargetRef]
		if sourceRef == targetRef {
			internalEdges++
			continue
		}
		key := sourceRef + "\x00" + targetRef + "\x00" + strconv.FormatBool(edge.Observed)
		if existing, ok := aggregated[key]; ok {
			existing.count++
			continue
		}
		aggregated[key] = &aggregate{edge: ImpactEdge{SourceRef: sourceRef, TargetRef: targetRef, Observed: edge.Observed}, count: 1}
		aggregateOrder = append(aggregateOrder, key)
	}
	sort.Strings(aggregateOrder)
	for _, key := range aggregateOrder {
		entry := aggregated[key]
		origin := "declared"
		if entry.edge.Observed {
			origin = "observed"
		}
		entry.edge.Label = fmt.Sprintf("%d %s", entry.count, origin)
		edges = append(edges, entry.edge)
	}
	return nodes, edges, groups, internalEdges, touched
}

func ownerGroupRef(node ImpactElement) string {
	if owner := strings.TrimSpace(node.Owner); owner != "" {
		return "owner:" + owner
	}
	return "element:" + node.Ref
}

func diagramMemberNames(members []ImpactElement) []string {
	out := make([]string, 0, len(members))
	for _, member := range members {
		out = append(out, member.Name)
	}
	return out
}

// renderImpactDiagram writes the report header and the style-specific graph.
func renderImpactDiagram(report ImpactReport, nodes []ImpactElement, edges []ImpactEdge, changed map[string]struct{}, lanes bool) string {
	lines := make([]string, 0, len(nodes)+len(edges)+4)
	if report.Coverage.Applicable {
		lines = append(lines, fmt.Sprintf("%%%% coverage: %d%% (%s)", report.Coverage.Percent, report.Coverage.Confidence))
	}
	lines = append(lines, renderImpactGraph(nodes, edges, changed, lanes))
	return strings.Join(lines, "\n")
}

func renderImpactGraph(nodes []ImpactElement, edges []ImpactEdge, changed map[string]struct{}, lanes bool) string {
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
		if edge.Observed {
			arrow = "-.->"
		}
		if strings.TrimSpace(edge.Label) != "" {
			lines = append(lines, fmt.Sprintf("  %s %s|%s| %s", sourceID, arrow, escapeMermaidLabel(edge.Label), targetID))
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
