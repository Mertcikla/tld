package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	sourceWhere, sourceScopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT path, hash, content, language, input_blob, dirty, syntax_cache, file_cache FROM codeindex_sources WHERE snapshot_id = ?`+sourceWhere, append([]any{snap.Id}, sourceScopeArgs...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		src := &graph.Source{}
		if err := rows.Scan(&src.Path, &src.Hash, &src.Text, &src.Language, &src.InputBlob, &src.Dirty, &src.SyntaxCache, &src.FileCache); err != nil {
			_ = rows.Close()
			return nil, err
		}
		g.Sources[src.Path] = src
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	artifactWhere, artifactScopeArgs := scope(ctx).clause("org_id")
	rows, err = s.bun.QueryContext(ctx, `SELECT project_key, fingerprint, data FROM codeindex_project_artifacts WHERE snapshot_id = ?`+artifactWhere, append([]any{snap.Id}, artifactScopeArgs...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var artifact graph.ProjectArtifact
		if err := rows.Scan(&key, &artifact.Fingerprint, &artifact.Data); err != nil {
			_ = rows.Close()
			return nil, err
		}
		g.ProjectArtifacts[key] = artifact
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
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
	return s.diff(ctx, fromID, toID, sourcesOnly, false)
}

// ImpactDiff computes the comparison payload used by change overlays. It is
// equivalent to Diff, but the edge delta is resolved with a keyed aggregate so
// only changed relationships are materialized instead of every edge in both
// snapshots.
func (s *Store) ImpactDiff(ctx context.Context, fromID, toID string) (*pb.SnapshotDiff, error) {
	return s.diff(ctx, fromID, toID, false, true)
}

func (s *Store) diff(ctx context.Context, fromID, toID string, sourcesOnly, aggregateEdges bool) (*pb.SnapshotDiff, error) {
	from, err := s.Snapshot(ctx, fromID)
	if err != nil {
		return nil, err
	}
	to, err := s.Snapshot(ctx, toID)
	if err != nil {
		return nil, err
	}
	if from.RepositoryId != to.RepositoryId {
		return nil, fmt.Errorf("snapshots must belong to the same repository")
	}
	diff := &pb.SnapshotDiff{
		RepositoryId:    to.RepositoryId,
		FromSnapshotId:  from.Id,
		ToSnapshotId:    to.Id,
		FromGitRevision: from.GitRevision,
		ToGitRevision:   to.GitRevision,
		Sources:         diffSources(from, to),
	}
	if err := s.sourceLineStats(ctx, diff.Sources); err != nil {
		return nil, err
	}
	if sourcesOnly {
		return diff, nil
	}
	fromFacts, toFacts, err := s.diffFactsByMembership(ctx, from.Id, to.Id)
	if err != nil {
		return nil, err
	}
	diff.Facts = diffFacts(fromFacts, toFacts)
	if aggregateEdges {
		diff.EdgeFacts, err = s.diffEdgeFactsAggregated(ctx, from.Id, to.Id)
		if err != nil {
			return nil, err
		}
		return diff, nil
	}
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

// diffFactsByMembership loads full facts for only the entities that differ
// between two snapshots. Unchanged facts share an id under stable reuse, so
// added and removed ids identify the delta and a shared logical key with
// different ids identifies a modified declaration.
func (s *Store) diffFactsByMembership(ctx context.Context, fromID, toID string) ([]*pb.CodeFact, []*pb.CodeFact, error) {
	fromKey, err := s.factLogicalKeys(ctx, fromID)
	if err != nil {
		return nil, nil, err
	}
	toKey, err := s.factLogicalKeys(ctx, toID)
	if err != nil {
		return nil, nil, err
	}
	deltaIDs := map[string]bool{}
	for id := range toKey {
		if _, ok := fromKey[id]; !ok {
			deltaIDs[id] = true
		}
	}
	for id := range fromKey {
		if _, ok := toKey[id]; !ok {
			deltaIDs[id] = true
		}
	}
	// A changed declaration has a new id but shares its logical key with the
	// removed version; include the matching side so the diff pairs them.
	fromByKey := map[string]string{}
	for id, key := range fromKey {
		fromByKey[key] = id
	}
	toByKey := map[string]string{}
	for id, key := range toKey {
		toByKey[key] = id
	}
	for key, fromFactID := range fromByKey {
		if toFactID, ok := toByKey[key]; ok && toFactID != fromFactID {
			deltaIDs[fromFactID] = true
			deltaIDs[toFactID] = true
		}
	}
	if len(deltaIDs) == 0 {
		return nil, nil, nil
	}
	facts, err := s.factsByIDs(ctx, deltaIDs)
	if err != nil {
		return nil, nil, err
	}
	var fromFacts, toFacts []*pb.CodeFact
	for id, fact := range facts {
		if _, ok := fromKey[id]; ok {
			fromFacts = append(fromFacts, fact)
		}
		if _, ok := toKey[id]; ok {
			toFacts = append(toFacts, fact)
		}
	}
	return fromFacts, toFacts, nil
}

// factLogicalKeys maps a snapshot's fact ids to their logical identity.
func (s *Store) factLogicalKeys(ctx context.Context, snapshotID string) (map[string]string, error) {
	where, scopeArgs := scope(ctx).clause("f.org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT f.id, f.logical_key FROM codeindex_facts f JOIN codeindex_snapshot_facts m ON m.fact_id = f.id AND m.org_id = f.org_id WHERE m.snapshot_id = ?`+where, append([]any{snapshotID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, err
		}
		out[id] = key
	}
	return out, rows.Err()
}

func (s *Store) factsByIDs(ctx context.Context, ids map[string]bool) (map[string]*pb.CodeFact, error) {
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	args = append(args, scopeArgs...)
	facts, err := s.scanFacts(ctx, `SELECT `+factColumns+` FROM codeindex_facts WHERE id IN (`+strings.Join(placeholders, ",")+`)`+where, args...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*pb.CodeFact, len(facts))
	for _, f := range facts {
		out[f.Id] = f
	}
	return out, nil
}

func logicalFact(f *pb.CodeFact) string {
	if f.LogicalKey != "" {
		return f.LogicalKey
	}
	return f.Id
}

func sameFact(a, b *pb.CodeFact) bool {
	// QualifiedName and SymbolKey carry the SCIP package version (and package
	// manager/name); the indexer deliberately treats those as non-identifying
	// when deriving LogicalKey, and facts already pair by that key, so comparing
	// them would report every symbol in a file as modified whenever the package
	// version differs between snapshots.
	if a.Kind != b.Kind || a.Name != b.Name ||
		a.Signature != b.Signature || a.Language != b.Language {
		return false
	}
	if (a.Anchor == nil) != (b.Anchor == nil) {
		return false
	}
	if a.Anchor != nil {
		if a.Anchor.Path != b.Anchor.Path {
			return false
		}
	}
	if a.Kind == pb.FactKind_FACT_KIND_FILE && a.Anchor != nil && a.Anchor.SourceHash != b.Anchor.SourceHash {
		return false
	}
	return a.Documentation == b.Documentation && strings.Join(a.Imports, "\x00") == strings.Join(b.Imports, "\x00") && graph.Hash([]byte(a.Code)) == graph.Hash([]byte(b.Code))
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

// edgeObservation is one logical relationship's representative edge id and the
// number of times that relationship was observed in a snapshot.
type edgeObservation struct {
	id    string
	count int
}

// edgeLogicalCounts aggregates a snapshot's edges by logical identity. The
// representative id is the first row in id order, matching the representative
// diffEdgeFacts keeps when it walks a fully loaded, id-ordered edge slice.
func (s *Store) edgeLogicalCounts(ctx context.Context, snapshotID string) (map[string]edgeObservation, error) {
	where, scopeArgs := scope(ctx).clause("e.org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT e.id, e.logical_key FROM codeindex_edges e JOIN codeindex_snapshot_edges m ON m.edge_id = e.id AND m.org_id = e.org_id WHERE m.snapshot_id = ?`+where+` ORDER BY e.id`, append([]any{snapshotID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]edgeObservation{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, err
		}
		if key == "" {
			key = id
		}
		if existing, ok := out[key]; ok {
			existing.count++
			out[key] = existing
			continue
		}
		out[key] = edgeObservation{id: id, count: 1}
	}
	return out, rows.Err()
}

// diffEdgeFactsAggregated computes the same edge delta as diffEdgeFacts without
// loading every edge of both snapshots. It aggregates observations by logical
// key, then materializes only the representative edges of changed
// relationships.
func (s *Store) diffEdgeFactsAggregated(ctx context.Context, fromID, toID string) (*pb.EdgeFactDelta, error) {
	from, err := s.edgeLogicalCounts(ctx, fromID)
	if err != nil {
		return nil, err
	}
	to, err := s.edgeLogicalCounts(ctx, toID)
	if err != nil {
		return nil, err
	}
	needed := map[string]bool{}
	for key, current := range to {
		if prev, ok := from[key]; !ok || prev.count != current.count {
			needed[current.id] = true
		}
	}
	for key, prev := range from {
		if _, ok := to[key]; !ok {
			needed[prev.id] = true
		}
	}
	loaded, err := s.edgesByIds(ctx, needed)
	if err != nil {
		return nil, err
	}
	delta := &pb.EdgeFactDelta{}
	for key, current := range to {
		prev, ok := from[key]
		switch {
		case !ok:
			if edge := edgeFromID(loaded, current.id, toID); edge != nil {
				delta.Added = append(delta.Added, edge)
			}
		case prev.count != current.count:
			if edge := edgeFromID(loaded, current.id, toID); edge != nil {
				delta.Modified = append(delta.Modified, edge)
			}
		}
	}
	for key, prev := range from {
		if _, ok := to[key]; !ok {
			if edge := edgeFromID(loaded, prev.id, fromID); edge != nil {
				delta.Removed = append(delta.Removed, edge)
			}
		}
	}
	sortEdgeFacts(delta.Added)
	sortEdgeFacts(delta.Removed)
	sortEdgeFacts(delta.Modified)
	return delta, nil
}

func edgeFromID(edges map[string]*pb.EdgeFact, id, snapshotID string) *pb.EdgeFact {
	edge := edges[id]
	if edge == nil {
		return nil
	}
	edge.SnapshotId = snapshotID
	return edge
}

// edgesByIds batch-loads edges, mirroring factsByIDs.
func (s *Store) edgesByIds(ctx context.Context, ids map[string]bool) (map[string]*pb.EdgeFact, error) {
	if len(ids) == 0 {
		return map[string]*pb.EdgeFact{}, nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	args = append(args, scopeArgs...)
	edges, err := s.scanEdges(ctx, `SELECT `+edgeColumns+` FROM codeindex_edges WHERE id IN (`+strings.Join(placeholders, ",")+`)`+where, args...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*pb.EdgeFact, len(edges))
	for _, edge := range edges {
		out[edge.Id] = edge
	}
	return out, nil
}
