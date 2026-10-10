// Package grounded builds the user-diagram-first overlay for repository
// comparisons: the authored skeleton stays canonical, code facts verify each
// linked element, and the lossy generated map contributes collapsed context
// only. The CLI emits this summary by default; the raw file-level impact
// diagram remains available behind a hidden flag.
package grounded

import (
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/pkg/app"
)

// Verification statuses for a linked element against a pinned comparison.
const (
	StatusAffected   = "affected"
	StatusContext    = "context"
	StatusUngrounded = "ungrounded"
)

// MaxFanoutShown bounds how many rolled-up edge groups are listed per
// high-fan-out node before the remainder collapses into an overflow count.
// It answers "100 outgoing edges" with 7 groups + "+N more", never 100 arrows.
const MaxFanoutShown = 7

// ElementStatus is one authored placement resolved against the diff. Status
// is file-based: affected means the linked file itself changed, context means
// it did not. Near names changed paths one dependency hop away, so an
// unlinked-but-indexed change still points at the neighbours it may disturb.
type ElementStatus struct {
	ViewID    int64    `json:"viewId"`
	ViewName  string   `json:"viewName,omitempty"`
	ElementID int64    `json:"elementId"`
	Name      string   `json:"name"`
	Link      string   `json:"link,omitempty"`
	Status    string   `json:"status"`
	Near      []string `json:"near,omitempty"`
}

// GroundedConnector is a user-authored connector inside a matched view,
// carried so bundle consumers see the same edges as the canvas.
type GroundedConnector struct {
	ViewID   int64  `json:"viewId"`
	ViewName string `json:"viewName,omitempty"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Label    string `json:"label,omitempty"`
}

// FanoutGroup rolls outgoing edges up by target prefix.
type FanoutGroup struct {
	Prefix string `json:"prefix"`
	Count  int    `json:"count"`
}

// Fanout describes a changed node whose out-degree exceeds the display
// budget: total edges, top groups, and the collapsed remainder.
type Fanout struct {
	Source string        `json:"source"`
	Total  int           `json:"total"`
	Groups []FanoutGroup `json:"groups"`
	Hidden int           `json:"hidden"`
}

// ViewMatch names an authored view intersecting the change.
type ViewMatch struct {
	ViewID   int64  `json:"viewId"`
	ViewName string `json:"viewName,omitempty"`
	Affected int    `json:"affected"`
	Total    int    `json:"total"`
	// Indirect counts elements one hop from a change (near or blast-radius
	// context) without their own file changing.
	Indirect int `json:"indirect,omitempty"`
}

// Summary is the grounded overlay: which authored elements the change
// touches, which views they live in, and where fan-out was collapsed.
type Summary struct {
	Mode       string              `json:"mode"`
	ViewFilter string              `json:"viewFilter,omitempty"`
	Affected   int                 `json:"affected"`
	Context    int                 `json:"context"`
	Ungrounded int                 `json:"ungrounded"`
	Views      []ViewMatch         `json:"views"`
	Elements   []ElementStatus     `json:"elements"`
	Connectors []GroundedConnector `json:"connectors,omitempty"`
	Fanout     []Fanout            `json:"fanout,omitempty"`
	// Uncovered lists changed paths no workspace element covers (no file,
	// symbol, or folder link resolves to them). The canvas renders these as
	// transient placements augmented from the graph facts, so an empty list
	// means the user workspace fully captures the change.
	Uncovered     []string `json:"uncovered,omitempty"`
	FallbackToRaw bool     `json:"fallbackToRaw,omitempty"`
}

// Summarize intersects the impact diagram's changed paths with the
// workspace's authored placements. Linked elements whose file matches a
// changed path are affected; linked elements elsewhere are context;
// placements without a file link are ungrounded. neighbours optionally maps
// an indexed path to its dependency-adjacent paths (undirected); linked files
// adjacent to a change are recorded in Near so unlinked-but-indexed edits
// still resolve to the neighbours they may disturb. viewFilter optionally
// restricts to one view id or case-insensitive name substring.
func Summarize(diagram *pb.ImpactDiagram, explore app.ExploreData, viewFilter string, allEdges bool, neighbours map[string][]string) Summary {
	sum := Summary{Mode: "grounded", ViewFilter: viewFilter, Views: []ViewMatch{}, Elements: []ElementStatus{}}
	if diagram == nil {
		sum.FallbackToRaw = true
		return sum
	}
	changed := changedPaths(diagram)
	// reached holds workspace element ids the comparison already placed in
	// the blast radius (context|<id> nodes), i.e. map-owned neighbours.
	reached := map[int64]bool{}
	if diagram != nil {
		for _, node := range diagram.GetNodes() {
			if node == nil || !strings.HasPrefix(node.GetKey(), "context|") || node.GetElementId() == 0 {
				continue
			}
			reached[node.GetElementId()] = true
		}
	}
	viewNames := map[int64]string{}
	collectViewNames(explore.Tree, viewNames)
	perViewAffected := map[int64]int{}
	perViewIndirect := map[int64]int{}
	perViewTotal := map[int64]int{}
	type placed struct {
		viewID int64
		el     app.PlacedElement
		link   string
		status string
		near   []string
	}
	var all []placed
	for viewIDStr, content := range explore.Views {
		viewID, err := strconv.ParseInt(viewIDStr, 10, 64)
		if err != nil {
			continue
		}
		if !matchViewFilter(viewID, viewNames[viewID], viewFilter) {
			continue
		}
		for _, el := range content.Placements {
			if isGenerated(el.Tags) {
				continue
			}
			link := ""
			if el.FilePath != nil {
				link = *el.FilePath
			}
			perViewTotal[viewID]++
			status := StatusUngrounded
			var near []string
			if link != "" {
				if intersects(link, changed) {
					status = StatusAffected
				} else {
					status = StatusContext
					near = nearChanges(link, changed, neighbours)
				}
			}
			switch status {
			case StatusAffected:
				sum.Affected++
				perViewAffected[viewID]++
			case StatusContext:
				sum.Context++
			default:
				sum.Ungrounded++
			}
			if len(near) > 0 || reached[el.ElementID] {
				perViewIndirect[viewID]++
			}
			all = append(all, placed{viewID: viewID, el: el, link: link, status: status, near: near})
		}
	}
	matched := map[int64]bool{}
	for viewID, total := range perViewTotal {
		if perViewAffected[viewID] == 0 && perViewIndirect[viewID] == 0 {
			continue
		}
		matched[viewID] = true
		sum.Views = append(sum.Views, ViewMatch{ViewID: viewID, ViewName: viewNames[viewID], Affected: perViewAffected[viewID], Total: total, Indirect: perViewIndirect[viewID]})
	}
	sort.Slice(sum.Views, func(i, j int) bool { return sum.Views[i].Affected > sum.Views[j].Affected })
	// Elements lists every placement in the matched views — affected, unlinked,
	// and context neighbours alike — so text renders show the same authored
	// nodes as the canvas. Pure-context views stay counted, not listed.
	for _, item := range all {
		if !matched[item.viewID] {
			continue
		}
		sum.Elements = append(sum.Elements, ElementStatus{
			ViewID: item.viewID, ViewName: viewNames[item.viewID],
			ElementID: item.el.ElementID,
			Name:      item.el.Name, Link: item.link, Status: item.status, Near: item.near,
		})
	}
	sort.Slice(sum.Elements, func(i, j int) bool {
		if sum.Elements[i].Status != sum.Elements[j].Status {
			return sum.Elements[i].Status < sum.Elements[j].Status
		}
		return sum.Elements[i].Name < sum.Elements[j].Name
	})
	// Connectors carries the user-authored edges inside matched views so the
	// mermaid export draws the same connectors as the canvas overlay.
	for viewIDStr, content := range explore.Views {
		viewID, err := strconv.ParseInt(viewIDStr, 10, 64)
		if err != nil || !matched[viewID] {
			continue
		}
		names := map[int64]string{}
		for _, el := range content.Placements {
			if _, ok := names[el.ElementID]; !ok {
				names[el.ElementID] = el.Name
			}
		}
		for _, conn := range content.Connectors {
			source, okSource := names[conn.SourceElementID]
			target, okTarget := names[conn.TargetElementID]
			if !okSource || !okTarget {
				continue
			}
			label := ""
			if conn.Label != nil {
				label = *conn.Label
			}
			sum.Connectors = append(sum.Connectors, GroundedConnector{
				ViewID: viewID, ViewName: viewNames[viewID],
				Source: source, Target: target, Label: label,
			})
		}
	}
	sort.Slice(sum.Connectors, func(i, j int) bool {
		if sum.Connectors[i].Source != sum.Connectors[j].Source {
			return sum.Connectors[i].Source < sum.Connectors[j].Source
		}
		return sum.Connectors[i].Target < sum.Connectors[j].Target
	})
	if len(sum.Views) == 0 {
		sum.FallbackToRaw = true
	}
	// Uncovered changed paths are the ones no workspace placement resolves
	// to — the same files the canvas renders as transient placements. Links
	// are collected workspace-wide (including generated map elements, which
	// also cover files on the canvas), independent of any view filter.
	var links []string
	for _, content := range explore.Views {
		for _, el := range content.Placements {
			if el.FilePath == nil {
				continue
			}
			if link := strings.TrimSpace(*el.FilePath); link != "" {
				links = append(links, link)
			}
		}
	}
	for path := range changed {
		if !coveredBy(path, links) {
			sum.Uncovered = append(sum.Uncovered, path)
		}
	}
	sort.Strings(sum.Uncovered)
	if !allEdges {
		sum.Fanout = RollupFanout(diagram, MaxFanoutShown)
	}
	return sum
}

// coveredBy reports whether any workspace link resolves to the changed path:
// an exact file/symbol link, or a folder link whose prefix contains it.
func coveredBy(path string, links []string) bool {
	for _, link := range links {
		base := strings.TrimSpace(basePath(link))
		if base == "" {
			continue
		}
		if base == path || strings.TrimSuffix(base, "/") == path {
			return true
		}
		if strings.HasPrefix(path, strings.TrimSuffix(base, "/")+"/") {
			return true
		}
	}
	return false
}

// RollupFanout collapses high out-degree change nodes by target prefix so a
// file with 100 outgoing EdgeFacts renders as a handful of groups plus an
// overflow count. Nodes at or under maxShown carry no entry.
func RollupFanout(diagram *pb.ImpactDiagram, maxShown int) []Fanout {
	if diagram == nil || maxShown < 1 {
		return nil
	}
	byKey := map[string]*pb.ImpactNode{}
	for _, node := range diagram.GetNodes() {
		if node != nil {
			byKey[node.GetKey()] = node
		}
	}
	outgoing := map[string][]*pb.ImpactEdge{}
	for _, edge := range diagram.GetEdges() {
		if edge == nil {
			continue
		}
		outgoing[edge.GetFromKey()] = append(outgoing[edge.GetFromKey()], edge)
	}
	var out []Fanout
	for key, edges := range outgoing {
		if len(edges) <= maxShown {
			continue
		}
		source := key
		if node, ok := byKey[key]; ok && node.GetPath() != "" {
			source = node.GetPath()
		}
		groups := map[string]int{}
		for _, edge := range edges {
			target := edge.GetToKey()
			if node, ok := byKey[target]; ok && node.GetPath() != "" {
				target = node.GetPath()
			}
			groups[targetPrefix(target)]++
		}
		list := make([]FanoutGroup, 0, len(groups))
		for prefix, count := range groups {
			list = append(list, FanoutGroup{Prefix: prefix, Count: count})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Count != list[j].Count {
				return list[i].Count > list[j].Count
			}
			return list[i].Prefix < list[j].Prefix
		})
		hidden := 0
		if len(list) > maxShown {
			for _, g := range list[maxShown:] {
				hidden += g.Count
			}
			list = list[:maxShown]
		}
		out = append(out, Fanout{Source: source, Total: len(edges), Groups: list, Hidden: hidden})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}

func changedPaths(diagram *pb.ImpactDiagram) map[string]bool {
	changed := map[string]bool{}
	for _, change := range diagram.GetDiff().GetSources() {
		if path := strings.TrimSpace(change.GetPath()); path != "" {
			changed[path] = true
		}
	}
	for _, node := range diagram.GetNodes() {
		if node == nil || node.GetDistance() != 0 || node.GetPath() == "" {
			continue
		}
		changed[strings.TrimSuffix(node.GetPath(), "/")] = true
	}
	return changed
}

// nearChanges returns the sorted changed paths one dependency hop from the
// linked file: neighbours of the link base that appear in the change set.
// Folder links resolve through every changed path under their prefix.
func nearChanges(link string, changed map[string]bool, neighbours map[string][]string) []string {
	base := strings.TrimSpace(basePath(link))
	if base == "" || len(neighbours) == 0 {
		return nil
	}
	bases := []string{strings.TrimSuffix(base, "/")}
	if strings.HasSuffix(base, "/") {
		for path := range changed {
			if strings.HasPrefix(path, base) {
				bases = append(bases, path)
			}
		}
	}
	seen := map[string]bool{}
	var near []string
	for _, from := range bases {
		for _, to := range neighbours[from] {
			if changed[to] && !seen[to] {
				seen[to] = true
				near = append(near, to)
			}
		}
	}
	sort.Strings(near)
	return near
}

// basePath strips a #line/#symbol anchor from a source link.
func basePath(link string) string {
	if i := strings.IndexByte(link, '#'); i >= 0 {
		return link[:i]
	}
	return link
}

func intersects(link string, changed map[string]bool) bool {
	base := strings.TrimSpace(basePath(link))
	if base == "" {
		return false
	}
	if changed[base] || changed[strings.TrimSuffix(base, "/")] {
		return true
	}
	// Folder-granularity link: any changed path under the prefix matches.
	prefix := strings.TrimSuffix(base, "/") + "/"
	for path := range changed {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func matchViewFilter(viewID int64, viewName, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	if strconv.FormatInt(viewID, 10) == filter {
		return true
	}
	return strings.Contains(strings.ToLower(viewName), strings.ToLower(filter))
}

func collectViewNames(tree []app.ViewTreeNode, out map[int64]string) {
	for _, view := range tree {
		out[view.ID] = view.Name
		collectViewNames(view.Children, out)
	}
}

func isGenerated(tags []string) bool {
	for _, tag := range tags {
		if strings.Contains(tag, "generated") || strings.Contains(tag, "map:") {
			return true
		}
	}
	return false
}

func targetPrefix(target string) string {
	cleaned := strings.TrimPrefix(target, "file|")
	cleaned = strings.TrimPrefix(cleaned, "context|")
	if i := strings.IndexByte(cleaned, '/'); i >= 0 {
		return cleaned[:i] + "/"
	}
	if cleaned == "" {
		return "(external)"
	}
	return cleaned
}
