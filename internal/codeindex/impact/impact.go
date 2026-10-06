// Package impact builds comparison payloads for transient ZUI change overlays.
package impact

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// Build describes changed files, attaches their symbol deltas, and adds
// eligible existing workspace resources within the requested file-dependency
// radius. Every node carries its hop distance so callers can scope the diagram
// client-side without another server round trip.
func Build(ctx context.Context, ws core.Store, idx *cstore.Store, repositoryID, key, fromID, toID string, contextDepth uint32) (*pb.ImpactDiagram, error) {
	diff, err := idx.ImpactDiff(ctx, fromID, toID)
	if err != nil {
		return nil, err
	}
	oldEdges, err := idx.FilePairCounts(ctx, fromID)
	if err != nil {
		return nil, err
	}
	newEdges, err := idx.FilePairCounts(ctx, toID)
	if err != nil {
		return nil, err
	}
	toSources, err := idx.SnapshotSources(ctx, toID)
	if err != nil {
		return nil, err
	}
	diagram := &pb.ImpactDiagram{RepositoryId: repositoryID, ComparisonKey: key, Diff: diff, Version: uuid.NewString()}
	changes := map[string]*pb.ImpactNode{}
	distance := map[string]uint32{}
	queue := []string{}
	for _, change := range diff.Sources {
		node := &pb.ImpactNode{Key: "file|" + change.Path, Path: change.Path, Name: filepath.Base(change.Path), Change: change.Change, Symbols: &pb.CodeFactDelta{}}
		changes[change.Path] = node
		distance[change.Path] = 0
		queue = append(queue, change.Path)
		diagram.Nodes = append(diagram.Nodes, node)
	}
	addSymbols := func(facts []*pb.CodeFact, which pb.ChangeKind) {
		for _, fact := range facts {
			if fact.Anchor == nil || fact.Kind == pb.FactKind_FACT_KIND_FILE {
				continue
			}
			node := changes[fact.Anchor.Path]
			if node == nil {
				continue
			}
			switch which {
			case pb.ChangeKind_CHANGE_KIND_ADDED:
				node.Symbols.Added = append(node.Symbols.Added, fact)
			case pb.ChangeKind_CHANGE_KIND_REMOVED:
				node.Symbols.Removed = append(node.Symbols.Removed, fact)
			case pb.ChangeKind_CHANGE_KIND_MODIFIED:
				node.Symbols.Modified = append(node.Symbols.Modified, fact)
			}
		}
	}
	addSymbols(diff.GetFacts().GetAdded(), pb.ChangeKind_CHANGE_KIND_ADDED)
	addSymbols(diff.GetFacts().GetRemoved(), pb.ChangeKind_CHANGE_KIND_REMOVED)
	addSymbols(diff.GetFacts().GetModified(), pb.ChangeKind_CHANGE_KIND_MODIFIED)
	adjacency := map[string]map[string]bool{}
	link := func(a, b string) {
		if adjacency[a] == nil {
			adjacency[a] = map[string]bool{}
		}
		adjacency[a][b] = true
	}
	for edge := range oldEdges {
		link(edge[0], edge[1])
		link(edge[1], edge[0])
	}
	for edge := range newEdges {
		link(edge[0], edge[1])
		link(edge[1], edge[0])
	}
	for i := 0; i < len(queue); i++ {
		path := queue[i]
		neighbors := make([]string, 0, len(adjacency[path]))
		for next := range adjacency[path] {
			neighbors = append(neighbors, next)
		}
		sort.Strings(neighbors)
		for _, next := range neighbors {
			if _, seen := distance[next]; seen {
				continue
			}
			distance[next] = distance[path] + 1
			queue = append(queue, next)
		}
	}
	mappings, err := idx.MappingsByRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	// Explore once for both placement and hierarchy resolution. A clean overlay
	// has no nodes to place or group, so skip the query entirely.
	data := core.ExploreData{}
	if len(diagram.Nodes) > 0 {
		data, err = ws.Explore(ctx)
		if err != nil {
			return nil, err
		}
	}
	seenResources := map[int64]bool{}
	for _, mapping := range mappings {
		if mapping.Kind != cstore.MappingElement || strings.HasPrefix(mapping.LogicalKey, "impact|") || seenResources[mapping.ResourceID] {
			continue
		}
		element, err := ws.ElementByID(ctx, mapping.ResourceID)
		if err != nil {
			continue
		}
		if element.FilePath == nil {
			continue
		}
		path := *element.FilePath
		hops, reachable := distance[path]
		if !reachable || changes[path] != nil {
			continue
		}
		if _, present := toSources[path]; !present {
			continue
		}
		diagram.MaxRadius = max(diagram.MaxRadius, hops)
		seenResources[element.ID] = true
		if hops > contextDepth {
			continue
		}
		diagram.Nodes = append(diagram.Nodes, &pb.ImpactNode{Key: fmt.Sprintf("context|%d", element.ID), Path: path, Name: element.Name, Distance: hops, ElementId: element.ID})
	}
	sort.Slice(diagram.Nodes, func(i, j int) bool { return diagram.Nodes[i].Key < diagram.Nodes[j].Key })
	nodesByPath := map[string][]*pb.ImpactNode{}
	for _, node := range diagram.Nodes {
		nodesByPath[node.Path] = append(nodesByPath[node.Path], node)
	}
	allEdges := map[[2]string]bool{}
	for edge := range oldEdges {
		allEdges[edge] = true
	}
	for edge := range newEdges {
		allEdges[edge] = true
	}
	for edge := range allEdges {
		kind := pb.ChangeKind_CHANGE_KIND_UNSPECIFIED
		oldWeight, had := oldEdges[edge]
		weight, has := newEdges[edge]
		switch {
		case !had:
			kind = pb.ChangeKind_CHANGE_KIND_ADDED
		case !has:
			kind = pb.ChangeKind_CHANGE_KIND_REMOVED
			weight = oldWeight
		case weight != oldWeight:
			kind = pb.ChangeKind_CHANGE_KIND_MODIFIED
		}
		for _, from := range nodesByPath[edge[0]] {
			for _, to := range nodesByPath[edge[1]] {
				diagram.Edges = append(diagram.Edges, &pb.ImpactEdge{FromKey: from.Key, ToKey: to.Key, Change: kind, Weight: weight})
			}
		}
	}
	sort.Slice(diagram.Edges, func(i, j int) bool {
		a, b := diagram.Edges[i], diagram.Edges[j]
		return a.FromKey+"\x00"+a.ToKey < b.FromKey+"\x00"+b.ToKey
	})
	// Placement runs last so new overlay nodes can be laid out next to the
	// context and already-mapped elements they are connected to.
	if err := placeAddedFiles(ctx, ws, diagram, mappings, data); err != nil {
		return nil, err
	}
	diagram.Groups = buildImpactGroups(ctx, idx, diagram, mappings, data)
	return diagram, nil
}

// Save persists the comparison payload only. Canvas overlays are ephemeral and
// must never create workspace elements, connectors, or views.
func Save(ctx context.Context, ws core.Store, idx *cstore.Store, repo, key, from, to string, radius uint32) (*pb.ImpactDiagram, error) {
	if err := removeLegacyMaterialization(ctx, ws, idx, repo); err != nil {
		return nil, err
	}
	diagram, err := Build(ctx, ws, idx, repo, key, from, to, radius)
	if err != nil {
		return nil, err
	}
	if err = idx.SaveImpact(ctx, diagram); err != nil {
		return nil, err
	}
	return diagram, nil
}

// Remove only resources owned by the retired impact materializer. Shared
// context elements have no impact ownership mapping and remain untouched.
func removeLegacyMaterialization(ctx context.Context, ws core.Store, idx *cstore.Store, repo string) error {
	mappings, err := idx.MappingsByRepository(ctx, repo)
	if err != nil {
		return err
	}
	for _, kind := range []cstore.MappingKind{cstore.MappingConnector, cstore.MappingView, cstore.MappingElement} {
		for _, mapping := range mappings {
			if mapping.Kind != kind || !strings.HasPrefix(mapping.LogicalKey, "impact|") {
				continue
			}
			switch kind {
			case cstore.MappingConnector:
				err = ws.DeleteConnector(ctx, mapping.ResourceID)
			case cstore.MappingView:
				err = ws.DeleteView(ctx, mapping.ResourceID)
			case cstore.MappingElement:
				err = ws.DeleteElement(ctx, mapping.ResourceID)
			}
			if err != nil {
				return err
			}
			if err = idx.DeleteMapping(ctx, mapping.LogicalKey); err != nil {
				return err
			}
		}
	}
	return nil
}
