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

// ElementStatus is one authored placement resolved against the diff.
type ElementStatus struct {
	ViewID   int64  `json:"viewId"`
	ViewName string `json:"viewName,omitempty"`
	Name     string `json:"name"`
	Link     string `json:"link,omitempty"`
	Status   string `json:"status"`
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
}

// Summary is the grounded overlay: which authored elements the change
// touches, which views they live in, and where fan-out was collapsed.
type Summary struct {
	Mode          string          `json:"mode"`
	ViewFilter    string          `json:"viewFilter,omitempty"`
	Affected      int             `json:"affected"`
	Context       int             `json:"context"`
	Ungrounded    int             `json:"ungrounded"`
	Views         []ViewMatch     `json:"views"`
	Elements      []ElementStatus `json:"elements"`
	Fanout        []Fanout        `json:"fanout,omitempty"`
	FallbackToRaw bool            `json:"fallbackToRaw,omitempty"`
}

// Summarize intersects the impact diagram's changed paths with the
// workspace's authored placements. Linked elements whose file matches a
// changed path are affected; linked elements elsewhere are context;
// placements without a file link are ungrounded. viewFilter optionally
// restricts to one view id or case-insensitive name substring.
func Summarize(diagram *pb.ImpactDiagram, explore app.ExploreData, viewFilter string, allEdges bool) Summary {
	sum := Summary{Mode: "grounded", ViewFilter: viewFilter, Views: []ViewMatch{}, Elements: []ElementStatus{}}
	if diagram == nil {
		sum.FallbackToRaw = true
		return sum
	}
	changed := changedPaths(diagram)
	viewNames := map[int64]string{}
	collectViewNames(explore.Tree, viewNames)
	perViewAffected := map[int64]int{}
	perViewTotal := map[int64]int{}
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
			if link != "" {
				if intersects(link, changed) {
					status = StatusAffected
				} else {
					status = StatusContext
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
			// Only list elements worth a human's attention: affected first,
			// then ungrounded. Pure context stays counted, not listed.
			if status != StatusContext {
				sum.Elements = append(sum.Elements, ElementStatus{
					ViewID: viewID, ViewName: viewNames[viewID],
					Name: el.Name, Link: link, Status: status,
				})
			}
		}
	}
	sort.Slice(sum.Elements, func(i, j int) bool {
		if sum.Elements[i].Status != sum.Elements[j].Status {
			return sum.Elements[i].Status < sum.Elements[j].Status
		}
		return sum.Elements[i].Name < sum.Elements[j].Name
	})
	for viewID, total := range perViewTotal {
		if perViewAffected[viewID] == 0 {
			continue
		}
		sum.Views = append(sum.Views, ViewMatch{ViewID: viewID, ViewName: viewNames[viewID], Affected: perViewAffected[viewID], Total: total})
	}
	sort.Slice(sum.Views, func(i, j int) bool { return sum.Views[i].Affected > sum.Views[j].Affected })
	if len(sum.Views) == 0 {
		sum.FallbackToRaw = true
	}
	if !allEdges {
		sum.Fanout = RollupFanout(diagram, MaxFanoutShown)
	}
	return sum
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
