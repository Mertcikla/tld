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
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"google.golang.org/protobuf/proto"
)

// buildAnalysis resolves file changes and dependency witnesses from immutable
// snapshots. Workspace selection and placement belong exclusively to assemble.
func buildAnalysis(ctx context.Context, idx *cstore.Store, repositoryID, key, fromID, toID string, blastRadius uint32) (*model.Diagram, error) {
	diff, err := idx.ImpactDiff(ctx, fromID, toID)
	if err != nil {
		return nil, err
	}
	oldEdges, oldKinds, err := idx.FilePairStats(ctx, fromID)
	if err != nil {
		return nil, err
	}
	newEdges, newKinds, err := idx.FilePairStats(ctx, toID)
	if err != nil {
		return nil, err
	}
	diagram := &model.Diagram{RepositoryId: repositoryID, ComparisonKey: key, Diff: diff, Version: uuid.NewString(), RequestedRadius: proto.Uint32(blastRadius)}
	snapshotPaths := map[string]map[string]bool{}
	diagram.ManifestComplete = true
	for _, id := range []string{fromID, toID} {
		snapshot, loadErr := idx.Snapshot(ctx, id)
		if loadErr != nil {
			return nil, loadErr
		}
		snapshotPaths[id] = map[string]bool{}
		for _, source := range snapshot.Sources {
			snapshotPaths[id][source.Path] = true
		}
		for _, file := range snapshot.GetManifest().GetFiles() {
			snapshotPaths[id][file.Path] = true
		}
		if !snapshot.GetManifest().GetComplete() {
			diagram.ManifestComplete = false
			diagram.Warnings = append(diagram.Warnings, "Incomplete Git manifest; file coverage is limited to captured sources")
		}
		diagram.Warnings = append(diagram.Warnings, snapshot.Warnings...)
	}
	changes := map[string]*model.Node{}
	distance := map[string]uint32{}
	queue := []string{}
	for _, change := range diff.Sources {
		node := &model.Node{Key: "file|" + change.Path, Path: change.Path, Name: filepath.Base(change.Path), Change: change.Change, Symbols: &pb.CodeFactDelta{}}
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
	witnesses := map[string][]*model.Witness{}
	for side, pairs := range []map[[2]string]float64{oldEdges, newEdges} {
		snapshotID := fromID
		if side == 1 {
			snapshotID = toID
		}
		for path, witness := range dependencyWitnesses(presentSeeds(queue, snapshotPaths[snapshotID]), pairs, snapshotID, blastRadius) {
			hops := uint32(len(witness.Paths) - 1)
			if previous, found := distance[path]; !found || hops < previous {
				distance[path] = hops
			}
			witnesses[path] = append(witnesses[path], witness)
		}
	}
	for path, hops := range distance {
		if node := changes[path]; node != nil {
			node.Witnesses = witnesses[path]
			continue
		}
		if hops > blastRadius {
			continue
		}
		node := &model.Node{Key: "context|" + path, Path: path, Name: filepath.Base(path), Distance: hops, Witnesses: witnesses[path]}
		diagram.Nodes = append(diagram.Nodes, node)
	}
	sort.Slice(diagram.Nodes, func(i, j int) bool { return diagram.Nodes[i].Key < diagram.Nodes[j].Key })
	nodesByPath := map[string][]*model.Node{}
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
		case weight != oldWeight || fmt.Sprint(oldKinds[edge]) != fmt.Sprint(newKinds[edge]):
			kind = pb.ChangeKind_CHANGE_KIND_MODIFIED
		}
		for _, from := range nodesByPath[edge[0]] {
			for _, to := range nodesByPath[edge[1]] {
				diagram.Edges = append(diagram.Edges, &model.Edge{FromKey: from.Key, ToKey: to.Key, Change: kind, Weight: weight})
			}
		}
	}
	sort.Slice(diagram.Edges, func(i, j int) bool {
		a, b := diagram.Edges[i], diagram.Edges[j]
		return a.FromKey+"\x00"+a.ToKey < b.FromKey+"\x00"+b.ToKey
	})
	for side, pairs := range []map[[2]string]float64{oldEdges, newEdges} {
		label, kinds := "base", oldKinds
		if side == 1 {
			label, kinds = "head", newKinds
		}
		for _, edge := range diagram.Edges {
			pair := [2]string{strings.TrimPrefix(strings.TrimPrefix(edge.FromKey, "file|"), "context|"), strings.TrimPrefix(strings.TrimPrefix(edge.ToKey, "file|"), "context|")}
			if count, ok := pairs[pair]; ok {
				diagram.Dependencies = append(diagram.Dependencies, &pb.ImpactDependency{FromPath: pair[0], ToPath: pair[1], Side: label, Kinds: kinds[pair], Observations: count})
			}
		}
	}
	if err := captureComparison(ctx, idx, diagram); err != nil {
		return nil, err
	}
	return diagram, nil
}

func presentSeeds(seeds []string, paths map[string]bool) []string {
	out := []string{}
	for _, path := range seeds {
		if paths[path] {
			out = append(out, path)
		}
	}
	return out
}
