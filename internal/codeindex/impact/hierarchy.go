package impact

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// impactGroupMinDepth skips the workspace root and the repository's map view.
// Both are always present, already known to the reader, and read better as the
// diagram title than as wrapping subgraphs.
const impactGroupMinDepth = 2

// buildImpactGroups resolves the nested hierarchy the impact diagram should be
// displayed under. When the displayed files are materialized in the workspace
// the workspace's nested views are used verbatim; otherwise the same
// dependency-graph grouping the map pipeline runs synthesizes a hierarchy so
// the diagram still reads as nested subgraphs.
func buildImpactGroups(ctx context.Context, idx *cstore.Store, diagram *pb.ImpactDiagram, mappings []cstore.ResourceMapping, data core.ExploreData) []*pb.ImpactGroup {
	if groups := buildViewGroups(diagram, data, mappings); len(groups) > 0 {
		return groups
	}
	return buildCommunityGroups(ctx, idx, diagram, diagram.GetRepositoryId())
}

// viewHierarchy indexes the workspace's nested views and the repository-owned
// elements placed in them.
type viewHierarchy struct {
	nodes    map[int64]core.ViewTreeNode
	parentOf map[int64]int64
	children map[int64][]int64
	depth    map[int64]int
	element  map[int64]int64
	file     map[string]int64
}

func newViewHierarchy(data core.ExploreData, mappings []cstore.ResourceMapping) viewHierarchy {
	owned := make(map[int64]bool, len(mappings))
	for _, mapping := range mappings {
		if mapping.Kind == cstore.MappingElement && !strings.HasPrefix(mapping.LogicalKey, "impact|") {
			owned[mapping.ResourceID] = true
		}
	}
	hierarchy := viewHierarchy{
		nodes:    map[int64]core.ViewTreeNode{},
		parentOf: map[int64]int64{},
		children: map[int64][]int64{},
		depth:    map[int64]int{},
		element:  map[int64]int64{},
		file:     map[string]int64{},
	}
	var walk func(node core.ViewTreeNode, depth int)
	walk = func(node core.ViewTreeNode, depth int) {
		hierarchy.nodes[node.ID] = node
		hierarchy.depth[node.ID] = depth
		if node.ParentViewID != nil {
			hierarchy.parentOf[node.ID] = *node.ParentViewID
			hierarchy.children[*node.ParentViewID] = append(hierarchy.children[*node.ParentViewID], node.ID)
		}
		for _, child := range node.Children {
			walk(child, depth+1)
		}
	}
	for _, root := range data.Tree {
		walk(root, 0)
	}
	for _, view := range data.Views {
		for _, placement := range view.Placements {
			if !owned[placement.ElementID] {
				continue
			}
			if _, ok := hierarchy.element[placement.ElementID]; !ok {
				hierarchy.element[placement.ElementID] = placement.ViewID
			}
			if placement.FilePath == nil || *placement.FilePath == "" {
				continue
			}
			file := filepath.ToSlash(*placement.FilePath)
			if _, ok := hierarchy.file[file]; !ok {
				hierarchy.file[file] = placement.ViewID
			}
		}
	}
	return hierarchy
}

// buildViewGroups assigns every impact node to the workspace view that owns it
// and returns the retained nested view subtree below the always-present
// Workspace and repository map views. Views that only contain other retained
// views are kept so the subgraphs stay nested; nodes without a view fall to the
// pipeline-selected diagram view and remain ungrouped when none exists.
func buildViewGroups(diagram *pb.ImpactDiagram, data core.ExploreData, mappings []cstore.ResourceMapping) []*pb.ImpactGroup {
	if len(diagram.GetGroups()) > 0 {
		return nil
	}
	hierarchy := newViewHierarchy(data, mappings)
	members := map[int64][]string{}
	included := map[int64]bool{}
	for _, node := range diagram.GetNodes() {
		if node == nil {
			continue
		}
		view := int64(0)
		if node.GetContext() {
			view = hierarchy.element[node.GetElementId()]
		} else {
			view = hierarchy.file[filepath.ToSlash(node.GetPath())]
		}
		if view == 0 {
			view = diagram.GetViewId()
		}
		if view == 0 || hierarchy.depth[view] < impactGroupMinDepth {
			continue
		}
		members[view] = append(members[view], node.GetKey())
		for current := view; current != 0 && hierarchy.depth[current] >= impactGroupMinDepth; {
			if included[current] {
				break
			}
			included[current] = true
			current = hierarchy.parentOf[current]
		}
	}
	if len(members) == 0 {
		return nil
	}
	roots := make([]int64, 0, len(included))
	for view := range included {
		if parent, ok := hierarchy.parentOf[view]; ok && included[parent] {
			continue
		}
		roots = append(roots, view)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	return viewGroups(hierarchy, roots, included, members)
}

func viewGroups(hierarchy viewHierarchy, views []int64, included map[int64]bool, members map[int64][]string) []*pb.ImpactGroup {
	sorted := append([]int64(nil), views...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	groups := make([]*pb.ImpactGroup, 0, len(sorted))
	for _, view := range sorted {
		if !included[view] {
			continue
		}
		name := hierarchy.nodes[view].Name
		if name == "" {
			name = "View " + strconv.FormatInt(view, 10)
		}
		group := &pb.ImpactGroup{
			Key:      "view:" + strconv.FormatInt(view, 10),
			Name:     name,
			Source:   "view",
			ViewId:   view,
			NodeKeys: members[view],
		}
		group.Children = viewGroups(hierarchy, hierarchy.children[view], included, members)
		groups = append(groups, group)
	}
	return groups
}

// buildCommunityGroups groups the diagram's nodes with the same Louvain
// community pipeline the map uses, for repositories with no materialized view
// hierarchy. Grouping is best-effort: a grouping failure leaves the diagram
// flat rather than failing the comparison.
func buildCommunityGroups(ctx context.Context, idx *cstore.Store, diagram *pb.ImpactDiagram, repositoryID string) []*pb.ImpactGroup {
	files := make([]community.File, 0, len(diagram.GetNodes()))
	indexOf := make(map[string]int, len(diagram.GetNodes()))
	for _, node := range diagram.GetNodes() {
		if node == nil || node.GetKey() == "" {
			continue
		}
		indexOf[node.GetKey()] = len(files)
		files = append(files, community.File{ID: node.GetKey(), Path: node.GetPath(), DisplayName: node.GetName()})
	}
	if len(files) == 0 {
		return nil
	}
	edges := make([]community.Edge, 0, len(diagram.GetEdges()))
	for _, edge := range diagram.GetEdges() {
		if edge == nil {
			continue
		}
		from, fromOK := indexOf[edge.GetFromKey()]
		to, toOK := indexOf[edge.GetToKey()]
		if !fromOK || !toOK || from == to {
			continue
		}
		edges = append(edges, community.Edge{A: from, B: to, Weight: edge.GetWeight()})
	}
	options := mapconfig.FromGlobal(nil)
	if overrides, err := idx.RepositoryMapOverrides(ctx, repositoryID); err == nil {
		if merged, err := options.WithOverrides(overrides); err == nil {
			options = merged
		}
	}
	grouping, err := community.Build(files, edges, options.Grouping)
	if err != nil || len(grouping.Groups) == 0 {
		return nil
	}
	materialize.SortGroups(grouping.Groups)
	return communityGroups(grouping.Groups, files)
}

func communityGroups(groups []*community.Group, files []community.File) []*pb.ImpactGroup {
	out := make([]*pb.ImpactGroup, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		keys := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			if member >= 0 && member < len(files) {
				keys = append(keys, files[member].ID)
			}
		}
		converted := &pb.ImpactGroup{Key: group.Key, Name: group.Name, Source: "community", NodeKeys: keys}
		converted.Children = communityGroups(group.Children, files)
		out = append(out, converted)
	}
	return out
}
