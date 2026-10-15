package store

import (
	"context"
	"fmt"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
)

// ImpactDiffGraphs uses the same fact matching and captured-source line counts
// as ImpactDiff, without loading immutable membership from the database again.
func (s *Store) ImpactDiffGraphs(ctx context.Context, before, after *pb.Snapshot, from, to *graph.Graph) (*pb.SnapshotDiff, error) {
	if before == nil || after == nil || from == nil || to == nil || before.RepositoryId != after.RepositoryId || from.RepositoryID != before.RepositoryId || to.RepositoryID != after.RepositoryId || from.SnapshotID != before.Id || to.SnapshotID != after.Id {
		return nil, fmt.Errorf("comparison graphs do not match snapshots")
	}
	diff := &pb.SnapshotDiff{RepositoryId: after.RepositoryId, FromSnapshotId: before.Id, ToSnapshotId: after.Id, FromGitRevision: before.GitRevision, ToGitRevision: after.GitRevision, Sources: diffSources(before, after)}
	if err := s.sourceLineStats(ctx, diff.Sources); err != nil {
		return nil, err
	}
	// Shared immutable IDs prove equality. Include the entire logical group for
	// changed IDs so repeated declarations match exactly as the SQL diff does.
	changed := map[string]bool{}
	for id, f := range from.Facts {
		if to.Facts[id] == nil {
			changed[logicalFact(f)] = true
		}
	}
	for id, f := range to.Facts {
		if from.Facts[id] == nil {
			changed[logicalFact(f)] = true
		}
	}
	var oldFacts, newFacts []*pb.CodeFact
	for _, f := range from.Facts {
		if changed[logicalFact(f)] {
			oldFacts = append(oldFacts, f)
		}
	}
	for _, f := range to.Facts {
		if changed[logicalFact(f)] {
			newFacts = append(newFacts, f)
		}
	}
	diff.Facts = diffFacts(oldFacts, newFacts)
	return diff, nil
}

// GraphFilePairStats is the in-memory equivalent of FilePairStats. Counts are
// observations, rather than weights; repeated equal edges remain distinct.
func GraphFilePairStats(g *graph.Graph) (map[[2]string]float64, map[[2]string][]pb.EdgeKind) {
	counts := map[[2]string]float64{}
	sets := map[[2]string]map[pb.EdgeKind]bool{}
	for _, e := range g.EdgeFacts {
		from, to := g.Facts[e.FromFactId], g.Facts[e.ToFactId]
		if from == nil || to == nil {
			continue
		}
		a, b := from.GetAnchor().GetPath(), to.GetAnchor().GetPath()
		if a == "" || b == "" || a == b {
			continue
		}
		pair := [2]string{a, b}
		counts[pair]++
		if sets[pair] == nil {
			sets[pair] = map[pb.EdgeKind]bool{}
		}
		sets[pair][e.Kind] = true
	}
	kinds := map[[2]string][]pb.EdgeKind{}
	for pair, set := range sets {
		for kind := range set {
			kinds[pair] = append(kinds[pair], kind)
		}
		sort.Slice(kinds[pair], func(i, j int) bool { return kinds[pair][i] < kinds[pair][j] })
	}
	return counts, kinds
}
