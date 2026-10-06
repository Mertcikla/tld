package mermaid

import (
	"fmt"
	"sort"
	"strings"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// ImpactExportColors configures the classDef colors used when exporting a
// repository change/impact diagram to Mermaid. Empty values fall back to the
// defaults that match the canvas change overlay palette.
type ImpactExportColors struct {
	Added     string
	Removed   string
	Modified  string
	Unchanged string
	Context   string
}

// ImpactExportOptions controls how ExportImpactDiagram renders its output.
type ImpactExportOptions struct {
	// IncludeMetadata prepends a %% tld-impact comment with the repository,
	// comparison key, and radius so the block can be round-tripped later.
	IncludeMetadata bool
	Colors          ImpactExportColors
}

// DefaultImpactExportColors returns the palette used by the change overlay
// canvas (see RepositoryChangeCanvas colors).
func DefaultImpactExportColors() ImpactExportColors {
	return ImpactExportColors{
		Added:     "#48bb78",
		Removed:   "#fc8181",
		Modified:  "#ecc94b",
		Unchanged: "#718096",
		Context:   "#4a5568",
	}
}

func (c ImpactExportColors) withDefaults() ImpactExportColors {
	defaults := DefaultImpactExportColors()
	if strings.TrimSpace(c.Added) == "" {
		c.Added = defaults.Added
	}
	if strings.TrimSpace(c.Removed) == "" {
		c.Removed = defaults.Removed
	}
	if strings.TrimSpace(c.Modified) == "" {
		c.Modified = defaults.Modified
	}
	if strings.TrimSpace(c.Unchanged) == "" {
		c.Unchanged = defaults.Unchanged
	}
	if strings.TrimSpace(c.Context) == "" {
		c.Context = defaults.Context
	}
	return c
}

// ExportImpactDiagram renders a repository change/impact diagram as a Mermaid
// flowchart. It is deliberately independent of any view/workspace state so it
// can be reused for other change-oriented surfaces.
func ExportImpactDiagram(diagram *codeindexv1.ImpactDiagram, opts ImpactExportOptions) string {
	if diagram == nil {
		return "flowchart LR\n"
	}
	colors := opts.Colors.withDefaults()
	stats := impactLineStats(diagram)

	lines := []string{"flowchart LR"}
	if opts.IncludeMetadata {
		parts := []string{"%% tld-impact"}
		appendEntry := func(key, value string) {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				parts = append(parts, key+"="+EscapeMetadataValue(trimmed))
			}
		}
		appendEntry("repo", diagram.GetRepositoryId())
		appendEntry("key", diagram.GetComparisonKey())
		parts = append(parts, fmt.Sprintf("radius=%d", diagram.GetRadius()))
		lines = append(lines, strings.Join(parts, " "))
	}

	classByNode := map[string]string{}
	nodeIDs := map[string]string{}
	usedIDs := map[string]bool{}
	renders := make([]impactNodeRender, 0, len(diagram.GetNodes()))
	renderByKey := make(map[string]impactNodeRender, len(diagram.GetNodes()))
	for _, node := range diagram.GetNodes() {
		if node == nil {
			continue
		}
		ref := uniqueImpactNodeID(node, nodeIDs, usedIDs)
		label := impactNodeLabel(node, stats)
		classByNode[ref] = impactNodeClass(node)
		item := impactNodeRender{key: node.GetKey(), ref: ref, line: fmt.Sprintf(`%s["%s"]`, ref, escapeMermaidLabel(label))}
		renders = append(renders, item)
		renderByKey[item.key] = item
	}

	if len(diagram.GetGroups()) == 0 {
		for _, item := range renders {
			lines = append(lines, "  "+item.line)
		}
		edgeLines := make([]string, 0, len(diagram.GetEdges()))
		for _, edge := range diagram.GetEdges() {
			if line, ok := impactEdgeLine(edge, nodeIDs); ok {
				edgeLines = append(edgeLines, "  "+line)
			}
		}
		if len(edgeLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, edgeLines...)
		}
	} else {
		usedGroups := map[string]bool{}
		index := 0
		groups := buildImpactGroupRenders(diagram.GetGroups(), usedGroups, &index)
		pathByKey := map[string][]string{}
		assignImpactGroupPaths(groups, nil, pathByKey)
		edgesByLevel := map[string][]string{}
		topEdges := make([]string, 0, len(diagram.GetEdges()))
		for _, edge := range diagram.GetEdges() {
			line, ok := impactEdgeLine(edge, nodeIDs)
			if !ok {
				continue
			}
			level := impactEdgeLevel(edge.GetFromKey(), edge.GetToKey(), pathByKey)
			if level == "" {
				topEdges = append(topEdges, line)
				continue
			}
			edgesByLevel[level] = append(edgesByLevel[level], line)
		}
		groupedRefs := map[string]bool{}
		lines = appendImpactGroups(lines, groups, renderByKey, edgesByLevel, groupedRefs, 0)

		ungrouped := make([]impactNodeRender, 0, len(renders))
		for _, item := range renders {
			if !groupedRefs[item.ref] {
				ungrouped = append(ungrouped, item)
			}
		}
		if len(ungrouped) > 0 {
			lines = append(lines, "")
			for _, item := range ungrouped {
				lines = append(lines, "  "+item.line)
			}
		}
		if len(topEdges) > 0 {
			lines = append(lines, "")
			for _, line := range topEdges {
				lines = append(lines, "  "+line)
			}
		}
	}

	classLines := impactClassLines(classByNode, colors)
	if len(classLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, classLines...)
	}

	return strings.Join(lines, "\n") + "\n"
}

// impactNodeRender is one exported node: its stable render line and the node
// key that places it in the hierarchy.
type impactNodeRender struct {
	key  string
	ref  string
	line string
}

// impactGroupRender is one nested hierarchy group with its resolved subgraph id.
type impactGroupRender struct {
	ref      string
	name     string
	nodes    []string
	children []*impactGroupRender
}

func buildImpactGroupRenders(groups []*codeindexv1.ImpactGroup, used map[string]bool, index *int) []*impactGroupRender {
	out := make([]*impactGroupRender, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		key := strings.TrimSpace(group.GetKey())
		if key == "" {
			key = fmt.Sprintf("%d", *index)
		}
		*index++
		ref := sanitizeMermaidID("group_" + key)
		if ref == "" {
			ref = "group"
		}
		if used[ref] {
			for n := 2; ; n++ {
				candidate := fmt.Sprintf("%s_%d", ref, n)
				if !used[candidate] {
					ref = candidate
					break
				}
			}
		}
		used[ref] = true
		item := &impactGroupRender{ref: ref, name: group.GetName(), nodes: group.GetNodeKeys()}
		item.children = buildImpactGroupRenders(group.GetChildren(), used, index)
		out = append(out, item)
	}
	return out
}

func assignImpactGroupPaths(groups []*impactGroupRender, prefix []string, pathByKey map[string][]string) {
	for _, group := range groups {
		path := append(append([]string(nil), prefix...), group.ref)
		for _, key := range group.nodes {
			if _, exists := pathByKey[key]; !exists {
				pathByKey[key] = path
			}
		}
		assignImpactGroupPaths(group.children, path, pathByKey)
	}
}

func appendImpactGroups(lines []string, groups []*impactGroupRender, renderByKey map[string]impactNodeRender, edgesByLevel map[string][]string, groupedRefs map[string]bool, depth int) []string {
	indent := strings.Repeat("  ", depth+1)
	inner := indent + "  "
	for _, group := range groups {
		lines = append(lines, "", fmt.Sprintf(`%ssubgraph %s["%s"]`, indent, group.ref, escapeMermaidLabel(group.name)))
		lines = appendImpactGroups(lines, group.children, renderByKey, edgesByLevel, groupedRefs, depth+1)
		for _, key := range group.nodes {
			item, ok := renderByKey[key]
			if !ok || groupedRefs[item.ref] {
				continue
			}
			groupedRefs[item.ref] = true
			lines = append(lines, inner+item.line)
		}
		for _, line := range edgesByLevel[group.ref] {
			lines = append(lines, inner+line)
		}
		lines = append(lines, indent+"end")
	}
	return lines
}

func impactEdgeLine(edge *codeindexv1.ImpactEdge, nodeIDs map[string]string) (string, bool) {
	if edge == nil {
		return "", false
	}
	sourceID, ok := nodeIDs[edge.GetFromKey()]
	if !ok {
		return "", false
	}
	targetID, ok := nodeIDs[edge.GetToKey()]
	if !ok {
		return "", false
	}
	label := impactEdgeLabel(edge.GetWeight())
	if label != "" {
		return fmt.Sprintf(`%s -- "%s" --> %s`, sourceID, escapeMermaidLabel(label), targetID), true
	}
	return fmt.Sprintf("%s --> %s", sourceID, targetID), true
}

// impactEdgeLevel returns the deepest group shared by both endpoints, or "" when
// they share no group and the edge belongs at the top level.
func impactEdgeLevel(fromKey, toKey string, pathByKey map[string][]string) string {
	from, fromOK := pathByKey[fromKey]
	to, toOK := pathByKey[toKey]
	if !fromOK || !toOK {
		return ""
	}
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	if common == 0 {
		return ""
	}
	return from[common-1]
}

func impactLineStats(diagram *codeindexv1.ImpactDiagram) map[string]*codeindexv1.SourceChange {
	stats := map[string]*codeindexv1.SourceChange{}
	for _, source := range diagram.GetDiff().GetSources() {
		if source == nil || strings.TrimSpace(source.GetPath()) == "" {
			continue
		}
		stats[source.GetPath()] = source
	}
	return stats
}

func uniqueImpactNodeID(node *codeindexv1.ImpactNode, known map[string]string, used map[string]bool) string {
	key := node.GetKey()
	if key == "" {
		key = node.GetPath()
	}
	if known, ok := known[key]; ok {
		return known
	}
	ref := sanitizeMermaidID("node_" + key)
	if ref == "" {
		ref = "node"
	}
	if used[ref] {
		for index := 2; ; index++ {
			candidate := fmt.Sprintf("%s_%d", ref, index)
			if !used[candidate] {
				ref = candidate
				break
			}
		}
	}
	used[ref] = true
	known[key] = ref
	return ref
}

func impactNodeLabel(node *codeindexv1.ImpactNode, stats map[string]*codeindexv1.SourceChange) string {
	label := strings.TrimSpace(node.GetName())
	if label == "" {
		label = strings.TrimSpace(node.GetPath())
	}
	if label == "" {
		label = node.GetKey()
	}
	lines := []string{label}
	if node.GetContext() {
		lines = append(lines, "(context)")
	} else if stat, ok := stats[node.GetPath()]; ok {
		added, removed := uint32(0), uint32(0)
		if stat.LinesAdded != nil {
			added = *stat.LinesAdded
		}
		if stat.LinesRemoved != nil {
			removed = *stat.LinesRemoved
		}
		if added > 0 || removed > 0 {
			lines = append(lines, fmt.Sprintf("+%d \u2212%d", added, removed))
		}
	}
	return strings.Join(lines, "<br/>")
}

func impactNodeClass(node *codeindexv1.ImpactNode) string {
	if node.GetContext() {
		return "context"
	}
	switch node.GetChange() {
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

func impactEdgeLabel(weight float64) string {
	if weight <= 0 {
		return ""
	}
	rounded := int(weight)
	if float64(rounded) == weight && rounded == 1 {
		return "1 dependency"
	}
	return fmt.Sprintf("%d dependencies", rounded)
}

func impactClassLines(classByNode map[string]string, colors ImpactExportColors) []string {
	nodesByClass := map[string][]string{}
	for ref, class := range classByNode {
		nodesByClass[class] = append(nodesByClass[class], ref)
	}
	order := []string{"added", "removed", "modified", "unchanged", "context"}
	lines := make([]string, 0, len(order)*2)
	for _, class := range order {
		refs := nodesByClass[class]
		if len(refs) == 0 {
			continue
		}
		sort.Strings(refs)
		lines = append(lines, impactClassDef(class, colors))
		lines = append(lines, fmt.Sprintf("  class %s %s", strings.Join(refs, ","), class))
	}
	return lines
}

func impactClassDef(class string, colors ImpactExportColors) string {
	switch class {
	case "added":
		return fmt.Sprintf("  classDef added fill:%s26,stroke:%s,color:#ffffff", colors.Added, colors.Added)
	case "removed":
		return fmt.Sprintf("  classDef removed fill:%s26,stroke:%s,color:#ffffff", colors.Removed, colors.Removed)
	case "modified":
		return fmt.Sprintf("  classDef modified fill:%s26,stroke:%s,color:#ffffff", colors.Modified, colors.Modified)
	case "context":
		return fmt.Sprintf("  classDef context fill:%s26,stroke:%s,color:#ffffff,stroke-dasharray: 3 3", colors.Context, colors.Context)
	default:
		return fmt.Sprintf("  classDef unchanged fill:%s26,stroke:%s,color:#ffffff", colors.Unchanged, colors.Unchanged)
	}
}
