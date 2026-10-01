package store

import (
	"context"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// LoadGraph reconstructs a published snapshot's code graph so an incremental
// build can carry forward unchanged facts, chunks, and edges.
func (s *Store) LoadGraph(ctx context.Context, snapshotID string) (*graph.Graph, error) {
	snap, err := s.Snapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	g := graph.NewGraph(snap.RepositoryId, snap.Id)
	facts, err := s.Facts(ctx, snap.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		g.Facts[f.Id] = f
	}
	chunks, err := s.Chunks(ctx, snap.Id)
	if err != nil {
		return nil, err
	}
	for _, c := range chunks {
		g.Chunks[c.Id] = c
	}
	edges, err := s.EdgeFacts(ctx, snap.Id, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	for _, e := range edges {
		g.EdgeFacts[e.Id] = e
	}
	return g, nil
}

// SnapshotSources returns a path-to-hash map for a snapshot's captured sources.
func (s *Store) SnapshotSources(ctx context.Context, snapshotID string) (map[string]string, error) {
	snap, err := s.Snapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(snap.Sources))
	for _, src := range snap.Sources {
		out[src.Path] = src.Hash
	}
	return out, nil
}

// Diff compares two snapshots of the same repository. Source changes are always
// reported; fact and edge deltas are added unless sourcesOnly is set. Deltas are
// keyed by stable logical identity, so renames surface as add plus remove and a
// changed body surfaces as modified.
func (s *Store) Diff(ctx context.Context, fromID, toID string, sourcesOnly bool) (*pb.SnapshotDiff, error) {
	from, err := s.Snapshot(ctx, fromID)
	if err != nil {
		return nil, err
	}
	to, err := s.Snapshot(ctx, toID)
	if err != nil {
		return nil, err
	}
	diff := &pb.SnapshotDiff{
		RepositoryId:    to.RepositoryId,
		FromSnapshotId:  from.Id,
		ToSnapshotId:    to.Id,
		FromGitRevision: from.GitRevision,
		ToGitRevision:   to.GitRevision,
		Sources:         diffSources(from, to),
	}
	if sourcesOnly {
		return diff, nil
	}
	fromFacts, err := s.Facts(ctx, from.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	toFacts, err := s.Facts(ctx, to.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	diff.Facts = diffFacts(fromFacts, toFacts)
	fromEdges, err := s.EdgeFacts(ctx, from.Id, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	toEdges, err := s.EdgeFacts(ctx, to.Id, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	diff.EdgeFacts = diffEdgeFacts(fromEdges, toEdges)
	return diff, nil
}

func diffSources(from, to *pb.Snapshot) []*pb.SourceChange {
	fromHashes := map[string]string{}
	for _, src := range from.Sources {
		fromHashes[src.Path] = src.Hash
	}
	toHashes := map[string]string{}
	for _, src := range to.Sources {
		toHashes[src.Path] = src.Hash
	}
	paths := make([]string, 0, len(fromHashes)+len(toHashes))
	seen := map[string]bool{}
	for path := range fromHashes {
		seen[path] = true
		paths = append(paths, path)
	}
	for path := range toHashes {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	changes := make([]*pb.SourceChange, 0)
	for _, path := range paths {
		fromHash, had := fromHashes[path]
		toHash, has := toHashes[path]
		change := &pb.SourceChange{Path: path, FromHash: fromHash, ToHash: toHash}
		switch {
		case !had:
			change.Change = pb.ChangeKind_CHANGE_KIND_ADDED
		case !has:
			change.Change = pb.ChangeKind_CHANGE_KIND_REMOVED
		case fromHash != toHash:
			change.Change = pb.ChangeKind_CHANGE_KIND_MODIFIED
		default:
			continue
		}
		changes = append(changes, change)
	}
	return changes
}

func logicalFact(f *pb.CodeFact) string {
	if f.LogicalKey != "" {
		return f.LogicalKey
	}
	return f.Id
}

func sameFact(a, b *pb.CodeFact) bool {
	if a.Kind != b.Kind || a.Name != b.Name || a.QualifiedName != b.QualifiedName ||
		a.Signature != b.Signature || a.Language != b.Language || a.SymbolKey != b.SymbolKey {
		return false
	}
	if (a.Anchor == nil) != (b.Anchor == nil) {
		return false
	}
	if a.Anchor != nil {
		if a.Anchor.Path != b.Anchor.Path || a.Anchor.StartByte != b.Anchor.StartByte || a.Anchor.EndByte != b.Anchor.EndByte {
			return false
		}
	}
	return graph.Hash([]byte(a.Code)) == graph.Hash([]byte(b.Code))
}

func diffFacts(from, to []*pb.CodeFact) *pb.CodeFactDelta {
	fromByKey := make(map[string]*pb.CodeFact, len(from))
	for _, f := range from {
		fromByKey[logicalFact(f)] = f
	}
	toByKey := make(map[string]*pb.CodeFact, len(to))
	for _, f := range to {
		toByKey[logicalFact(f)] = f
	}
	delta := &pb.CodeFactDelta{}
	for key, f := range toByKey {
		prev, ok := fromByKey[key]
		switch {
		case !ok:
			delta.Added = append(delta.Added, f)
		case !sameFact(prev, f):
			delta.Modified = append(delta.Modified, f)
		}
	}
	for key, f := range fromByKey {
		if _, ok := toByKey[key]; !ok {
			delta.Removed = append(delta.Removed, f)
		}
	}
	sortFacts(delta.Added)
	sortFacts(delta.Removed)
	sortFacts(delta.Modified)
	return delta
}

func sortFacts(facts []*pb.CodeFact) {
	sort.Slice(facts, func(i, j int) bool { return logicalFact(facts[i]) < logicalFact(facts[j]) })
}

func logicalEdge(e *pb.EdgeFact) string {
	if e.LogicalKey != "" {
		return e.LogicalKey
	}
	return e.Id
}

// diffEdgeFacts diffs edges at the logical relationship level. A relationship
// present in both snapshots is modified when its observation count changes.
func diffEdgeFacts(from, to []*pb.EdgeFact) *pb.EdgeFactDelta {
	type observation struct {
		edge  *pb.EdgeFact
		count int
	}
	fromByKey := make(map[string]*observation, len(from))
	for _, e := range from {
		key := logicalEdge(e)
		if existing := fromByKey[key]; existing != nil {
			existing.count++
			continue
		}
		fromByKey[key] = &observation{edge: e, count: 1}
	}
	toByKey := make(map[string]*observation, len(to))
	for _, e := range to {
		key := logicalEdge(e)
		if existing := toByKey[key]; existing != nil {
			existing.count++
			continue
		}
		toByKey[key] = &observation{edge: e, count: 1}
	}
	delta := &pb.EdgeFactDelta{}
	for key, current := range toByKey {
		prev, ok := fromByKey[key]
		switch {
		case !ok:
			delta.Added = append(delta.Added, current.edge)
		case prev.count != current.count:
			delta.Modified = append(delta.Modified, current.edge)
		}
	}
	for key, prev := range fromByKey {
		if _, ok := toByKey[key]; !ok {
			delta.Removed = append(delta.Removed, prev.edge)
		}
	}
	sortEdgeFacts(delta.Added)
	sortEdgeFacts(delta.Removed)
	sortEdgeFacts(delta.Modified)
	return delta
}

func sortEdgeFacts(edges []*pb.EdgeFact) {
	sort.Slice(edges, func(i, j int) bool { return logicalEdge(edges[i]) < logicalEdge(edges[j]) })
}
