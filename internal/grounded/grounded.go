// Package grounded projects authored impact summaries from canonical scenes.
package grounded

import (
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
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
	Uncovered []string `json:"uncovered,omitempty"`
	// FallbackToRaw retains the JSON contract and reports the absence of authored views.
	// Mapped and raw coverage still come from the canonical scene.
	FallbackToRaw bool `json:"fallbackToRaw,omitempty"`
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
