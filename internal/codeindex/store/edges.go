package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// FileEdge is a dependency between two file facts, resolved from a symbol edge
// whose endpoints live in those files. Kind is the dominant relationship when
// several edge kinds were aggregated into this pair.
type FileEdge struct {
	FromFactID string
	ToFactID   string
	Weight     float64
	Kind       pb.EdgeKind
}

// snapshotFileIDs maps file path to the file fact that belongs to a snapshot
// through the membership table. Facts are immutable shared rows whose
// snapshot_id is the snapshot that first wrote them, so incremental snapshots
// must resolve files via codeindex_snapshot_facts rather than
// codeindex_facts.snapshot_id.
func (s *Store) snapshotFileIDs(ctx context.Context, snapshotID string) (map[string]string, error) {
	where, scopeArgs := scope(ctx).clause("f.org_id")
	rows, err := s.bun.QueryContext(ctx, `
		SELECT f.id, f.path
		FROM codeindex_facts f
		JOIN codeindex_snapshot_facts m ON m.fact_id = f.id AND m.org_id = f.org_id
		WHERE m.snapshot_id = ? AND f.kind = ?`+where, append([]any{snapshotID, int(pb.FactKind_FACT_KIND_FILE)}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		if path != "" {
			out[path] = id
		}
	}
	return out, rows.Err()
}

// FileEdges resolves a snapshot's symbol-to-symbol edges to the file facts that
// contain them. Edges whose endpoints cannot be resolved to distinct file facts
// (external targets, unresolved symbols, or intra-file edges) are omitted.
func (s *Store) FileEdges(ctx context.Context, snapshotID string) ([]FileEdge, error) {
	files, err := s.snapshotFileIDs(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	where, scopeArgs := scope(ctx).clause("e.org_id")
	rows, err := s.bun.QueryContext(ctx, `
		SELECT sf.path, tf.path, COALESCE(e.weight, 0)
		FROM codeindex_edges e
		JOIN codeindex_facts sf ON sf.id = e.from_fact_id AND sf.org_id = e.org_id
		JOIN codeindex_facts tf ON tf.id = e.to_fact_id AND tf.org_id = e.org_id
		WHERE e.snapshot_id = ?`+where+`
		ORDER BY e.id`, append([]any{snapshotID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []FileEdge{}
	for rows.Next() {
		var fromPath, toPath string
		var weight float64
		if err := rows.Scan(&fromPath, &toPath, &weight); err != nil {
			return nil, err
		}
		if fromPath == "" || toPath == "" || fromPath == toPath {
			continue
		}
		from, to := files[fromPath], files[toPath]
		if from == "" || to == "" {
			continue
		}
		out = append(out, FileEdge{FromFactID: from, ToFactID: to, Weight: weight})
	}
	return out, rows.Err()
}

// FilePairCounts aggregates a snapshot's symbol-to-symbol edges into the number
// of observations crossing each ordered pair of distinct files. It is the SQL
// equivalent of walking a loaded graph's facts and edges, without materializing
// either, so change overlays can build file adjacency cheaply.
func (s *Store) FilePairCounts(ctx context.Context, snapshotID string) (map[[2]string]float64, error) {
	where, scopeArgs := scope(ctx).clause("e.org_id")
	rows, err := s.bun.QueryContext(ctx, `
		SELECT sf.path, tf.path, COUNT(*)
		FROM codeindex_edges e
		JOIN codeindex_snapshot_edges m ON m.edge_id = e.id AND m.org_id = e.org_id
		JOIN codeindex_facts sf ON sf.id = e.from_fact_id AND sf.org_id = e.org_id
		JOIN codeindex_facts tf ON tf.id = e.to_fact_id AND tf.org_id = e.org_id
		WHERE m.snapshot_id = ? AND sf.path <> '' AND tf.path <> '' AND sf.path <> tf.path`+where+`
		GROUP BY sf.path, tf.path`, append([]any{snapshotID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[[2]string]float64{}
	for rows.Next() {
		var from, to string
		var count int
		if err := rows.Scan(&from, &to, &count); err != nil {
			return nil, err
		}
		out[[2]string{from, to}] = float64(count)
	}
	return out, rows.Err()
}

// FileImport is one external import declared by a file fact.
type FileImport struct {
	FileFactID string
	Import     string
}

// AggregatedFileEdges returns one weighted file-to-file edge per pair, summing
// every resolved symbol observation. When a snapshot stores no edge weights the
// observation count is used instead, so grouping still sees real coupling. The
// dominant edge kind (highest summed weight, ties broken by kind order) is kept
// so materialized connectors can carry a relationship label.
func (s *Store) AggregatedFileEdges(ctx context.Context, snapshotID string) ([]FileEdge, error) {
	files, err := s.snapshotFileIDs(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	where, scopeArgs := scope(ctx).clause("e.org_id")
	rows, err := s.bun.QueryContext(ctx, `
		SELECT sf.path, tf.path, e.kind, COALESCE(e.weight, 0)
		FROM codeindex_edges e
		JOIN codeindex_facts sf ON sf.id = e.from_fact_id AND sf.org_id = e.org_id
		JOIN codeindex_facts tf ON tf.id = e.to_fact_id AND tf.org_id = e.org_id
		WHERE e.snapshot_id = ?`+where+`
		ORDER BY e.id`, append([]any{snapshotID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type pairKey struct{ from, to string }
	type kindStat struct {
		weight       float64
		observations int
	}
	type accumulator struct {
		kindStats map[pb.EdgeKind]*kindStat
	}
	acc := map[pairKey]*accumulator{}
	order := []pairKey{}
	for rows.Next() {
		var fromPath, toPath string
		var kind int
		var weight float64
		if err := rows.Scan(&fromPath, &toPath, &kind, &weight); err != nil {
			return nil, err
		}
		if fromPath == "" || toPath == "" || fromPath == toPath {
			continue
		}
		from, to := files[fromPath], files[toPath]
		if from == "" || to == "" {
			continue
		}
		key := pairKey{from: from, to: to}
		entry := acc[key]
		if entry == nil {
			entry = &accumulator{kindStats: map[pb.EdgeKind]*kindStat{}}
			acc[key] = entry
			order = append(order, key)
		}
		stat := entry.kindStats[pb.EdgeKind(kind)]
		if stat == nil {
			stat = &kindStat{}
			entry.kindStats[pb.EdgeKind(kind)] = stat
		}
		stat.weight += weight
		stat.observations++
	}
	out := make([]FileEdge, 0, len(order))
	for _, key := range order {
		entry := acc[key]
		kinds := make(map[pb.EdgeKind]float64, len(entry.kindStats))
		total := 0.0
		for kind, stat := range entry.kindStats {
			// Snapshots that store no edge weights fall back to the observation
			// count per kind, so both the pair weight and the dominant kind stay
			// meaningful.
			effective := stat.weight
			if effective <= 0 {
				effective = float64(stat.observations)
			}
			kinds[kind] = effective
			total += effective
		}
		out = append(out, FileEdge{FromFactID: key.from, ToFactID: key.to, Weight: total, Kind: dominantEdgeKind(kinds)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FromFactID != out[j].FromFactID {
			return out[i].FromFactID < out[j].FromFactID
		}
		return out[i].ToFactID < out[j].ToFactID
	})
	return out, rows.Err()
}

// dominantEdgeKind picks the kind with the highest accumulated weight. Ties are
// broken by the lower kind value so the result is deterministic.
func dominantEdgeKind(kinds map[pb.EdgeKind]float64) pb.EdgeKind {
	best := pb.EdgeKind_EDGE_KIND_UNSPECIFIED
	bestWeight := -1.0
	for kind, weight := range kinds {
		if weight > bestWeight || (weight == bestWeight && kind < best) {
			best = kind
			bestWeight = weight
		}
	}
	return best
}

// AllFacts loads every fact of one kind for a snapshot, paging past the
// per-call row cap. Results stay in fact-id order.
func (s *Store) AllFacts(ctx context.Context, snapshotID string, kind pb.FactKind) ([]*pb.CodeFact, error) {
	const pageSize = 1000
	out := []*pb.CodeFact{}
	after := ""
	for {
		page, err := s.Facts(ctx, snapshotID, kind, "", after, pageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return out, nil
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
		after = page[len(page)-1].Id
	}
}

// FileImports returns the deduplicated external imports declared by a
// snapshot's files, resolved to their file facts. Relative imports (those
// targeting the repository itself) are omitted.
func (s *Store) FileImports(ctx context.Context, snapshotID string) ([]FileImport, error) {
	files, err := s.snapshotFileIDs(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	where, scopeArgs := scope(ctx).clause("sf.org_id")
	rows, err := s.bun.QueryContext(ctx, `
		SELECT sf.path, sf.imports_json
		FROM codeindex_facts sf
		JOIN codeindex_snapshot_facts msf ON msf.snapshot_id = ? AND msf.fact_id = sf.id AND msf.org_id = sf.org_id
		WHERE sf.path <> ''
			AND sf.imports_json NOT IN ('', 'null', '[]')`+where, append([]any{snapshotID}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	seen := map[[2]string]struct{}{}
	out := []FileImport{}
	for rows.Next() {
		var path, raw string
		if err := rows.Scan(&path, &raw); err != nil {
			return nil, err
		}
		fileID := files[path]
		if fileID == "" {
			continue
		}
		var imports []string
		if err := json.Unmarshal([]byte(raw), &imports); err != nil {
			continue
		}
		for _, rawImport := range imports {
			imp := strings.TrimSpace(rawImport)
			if imp == "" || isRelativeImport(imp) {
				continue
			}
			key := [2]string{fileID, imp}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, FileImport{FileFactID: fileID, Import: imp})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FileFactID != out[j].FileFactID {
			return out[i].FileFactID < out[j].FileFactID
		}
		return out[i].Import < out[j].Import
	})
	return out, rows.Err()
}

func isRelativeImport(path string) bool {
	return strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/")
}
