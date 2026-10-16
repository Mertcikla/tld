package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

// LoadGraph reconstructs a published snapshot's code graph so an incremental
// build can carry forward unchanged facts and edges.
func (s *Store) LoadGraph(ctx context.Context, snapshotID string) (*graph.Graph, error) {
	snap, err := s.Snapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	g := graph.NewGraph(snap.RepositoryId, snap.Id)
	sourceWhere, sourceScopeArgs := scope(ctx).clause("s.org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT s.path, s.hash, b.content, s.language, s.input_blob, s.dirty, b.syntax_cache, b.file_cache, COALESCE(b.compression, 0)
		FROM codeindex_sources s
		LEFT JOIN codeindex_blobs b ON b.org_id = s.org_id AND b.hash = s.hash AND b.language = s.language
		WHERE s.snapshot_id = ?`+sourceWhere, append([]any{snap.Id}, sourceScopeArgs...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		src := &graph.Source{}
		var syntaxCache, fileCache []byte
		var compression int
		if err := rows.Scan(&src.Path, &src.Hash, &src.Text, &src.Language, &src.InputBlob, &src.Dirty, &syntaxCache, &fileCache, &compression); err != nil {
			_ = rows.Close()
			return nil, err
		}
		for i, field := range []*[]byte{&src.Text, &syntaxCache, &fileCache} {
			decoded, err := decompressBlob(*field, compression, 1<<i)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("source %s: %w", src.Path, err)
			}
			*field = decoded
		}
		src.SyntaxCache, src.FileCache = string(syntaxCache), string(fileCache)
		src.SyntaxCache = sourceCache(src.SyntaxCache, snap.RepositoryId, src.Path)
		src.FileCache = sourceCache(src.FileCache, snap.RepositoryId, src.Path)
		g.Sources[src.Path] = src
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	artifactWhere, artifactScopeArgs := scope(ctx).clause("p.org_id")
	rows, err = s.bun.QueryContext(ctx, `SELECT p.project_key, p.fingerprint, a.data
		FROM codeindex_project_artifacts p
		JOIN codeindex_artifacts a ON a.org_id = p.org_id AND a.project_key = p.project_key AND a.fingerprint = p.fingerprint
		WHERE p.snapshot_id = ?`+artifactWhere, append([]any{snap.Id}, artifactScopeArgs...)...)
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
	edges, err := s.EdgeFacts(ctx, snap.Id, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", "", graphLoadLimit)
	if err != nil {
		return nil, err
	}
	for _, e := range edges {
		g.EdgeFacts[e.Id] = e
	}
	return g, nil
}

// Shared blobs can carry caches extracted from another identical file. Facts
// retain source-specific names, logical keys and IDs even when re-anchored, so
// only reuse a cache for its original repository and path. The indexer rebuilds
// rejected caches from the captured source without losing content deduplication.
func sourceCache(raw, repositoryID, path string) string {
	var cache struct {
		Fact  *pb.CodeFact
		Decls []struct{ Fact *pb.CodeFact }
	}
	if json.Unmarshal([]byte(raw), &cache) != nil {
		// The indexer owns cache format/version validation and ignores malformed
		// caches; only source identity needs checking here.
		return raw
	}
	matches := func(fact *pb.CodeFact) bool {
		return fact == nil || (fact.RepositoryId == repositoryID && fact.GetAnchor().GetPath() == path)
	}
	if !matches(cache.Fact) {
		return ""
	}
	for _, declaration := range cache.Decls {
		if !matches(declaration.Fact) {
			return ""
		}
	}
	return raw
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

// ImpactDiff computes the comparison payload used by change overlays. Impact
// diagrams carry file-level dependency edges (FilePairCounts) and symbol
// deltas, so the edge delta is left out entirely: materializing every edge of
// both snapshots produced a payload nothing consumed.
func (s *Store) ImpactDiff(ctx context.Context, fromID, toID string) (*pb.SnapshotDiff, error) {
	return s.diff(ctx, fromID, toID, false, true)
}

func (s *Store) diff(ctx context.Context, fromID, toID string, sourcesOnly, skipEdges bool) (*pb.SnapshotDiff, error) {
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
	if err := s.EnsureManifest(ctx, from); err != nil {
		return nil, err
	}
	if err := s.EnsureManifest(ctx, to); err != nil {
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
	if skipEdges {
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
	if from.GetManifest().GetComplete() && to.GetManifest().GetComplete() {
		return diffManifest(from, to, fromHashes, toHashes)
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
	// A logical key can describe several physical declarations (for example
	// C++ macro expansions). Load the entire affected group on both sides so
	// unchanged occurrences can be matched before pairing changed ones.
	changedKeys := map[string]bool{}
	for id, key := range fromKey {
		if deltaIDs[id] {
			changedKeys[key] = true
		}
	}
	for id, key := range toKey {
		if deltaIDs[id] {
			changedKeys[key] = true
		}
	}
	for _, keys := range []map[string]string{fromKey, toKey} {
		for id, key := range keys {
			if changedKeys[key] {
				deltaIDs[id] = true
			}
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
			copy := proto.Clone(fact).(*pb.CodeFact)
			copy.SnapshotId = fromID
			fromFacts = append(fromFacts, copy)
		}
		if _, ok := toKey[id]; ok {
			copy := proto.Clone(fact).(*pb.CodeFact)
			copy.SnapshotId = toID
			toFacts = append(toFacts, copy)
		}
	}
	if err := s.hydrateFactSymbolVersions(ctx, fromFacts); err != nil {
		return nil, nil, err
	}
	if err := s.hydrateFactSymbolVersions(ctx, toFacts); err != nil {
		return nil, nil, err
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
	return a.Documentation == b.Documentation && strings.Join(a.Imports, "\x00") == strings.Join(b.Imports, "\x00") && a.GetBodyHash() == b.GetBodyHash()
}

func diffFacts(from, to []*pb.CodeFact) *pb.CodeFactDelta {
	fromByKey := make(map[string][]*pb.CodeFact, len(from))
	for _, f := range from {
		fromByKey[logicalFact(f)] = append(fromByKey[logicalFact(f)], f)
	}
	toByKey := make(map[string][]*pb.CodeFact, len(to))
	for _, f := range to {
		toByKey[logicalFact(f)] = append(toByKey[logicalFact(f)], f)
	}
	delta := &pb.CodeFactDelta{}
	for key, next := range toByKey {
		previous := fromByKey[key]
		sortFacts(previous)
		sortFacts(next)
		// Match identical content first, including declarations moved by an
		// insertion. Never collapse repeated keys to a random map entry.
		used := make([]bool, len(previous))
		var unmatched []*pb.CodeFact
		for _, f := range next {
			match := -1
			for i, prev := range previous {
				if !used[i] && sameFact(prev, f) {
					match = i
					break
				}
			}
			if match >= 0 {
				used[match] = true
			} else {
				unmatched = append(unmatched, f)
			}
		}
		var remaining []*pb.CodeFact
		for i, prev := range previous {
			if !used[i] {
				remaining = append(remaining, prev)
			}
		}
		paired := min(len(remaining), len(unmatched))
		delta.Modified = append(delta.Modified, unmatched[:paired]...)
		delta.Added = append(delta.Added, unmatched[paired:]...)
		delta.Removed = append(delta.Removed, remaining[paired:]...)
	}
	for key, facts := range fromByKey {
		if _, ok := toByKey[key]; !ok {
			delta.Removed = append(delta.Removed, facts...)
		}
	}
	sortFacts(delta.Added)
	sortFacts(delta.Removed)
	sortFacts(delta.Modified)
	return delta
}

func sortFacts(facts []*pb.CodeFact) {
	sort.Slice(facts, func(i, j int) bool {
		a, b := facts[i], facts[j]
		if logicalFact(a) != logicalFact(b) {
			return logicalFact(a) < logicalFact(b)
		}
		if a.GetAnchor().GetStartByte() != b.GetAnchor().GetStartByte() {
			return a.GetAnchor().GetStartByte() < b.GetAnchor().GetStartByte()
		}
		if a.GetAnchor().GetEndByte() != b.GetAnchor().GetEndByte() {
			return a.GetAnchor().GetEndByte() < b.GetAnchor().GetEndByte()
		}
		return a.Id < b.Id
	})
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
