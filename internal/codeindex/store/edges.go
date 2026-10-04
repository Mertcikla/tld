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

// FileEdges resolves a snapshot's symbol-to-symbol edges to the file facts that
// contain them. Edges whose endpoints cannot be resolved to distinct file facts
// (external targets, unresolved symbols, or intra-file edges) are omitted.
func (s *Store) FileEdges(ctx context.Context, snapshotID string) ([]FileEdge, error) {
	rows, err := s.bun.QueryContext(ctx, `
		SELECT ffrom.id, fto.id, COALESCE(e.weight, 0)
		FROM codeindex_edges e
		JOIN codeindex_facts sf ON sf.id = e.from_fact_id AND sf.snapshot_id = e.snapshot_id
		JOIN codeindex_facts tf ON tf.id = e.to_fact_id AND tf.snapshot_id = e.snapshot_id
		JOIN codeindex_facts ffrom ON ffrom.snapshot_id = e.snapshot_id AND ffrom.kind = ? AND ffrom.path = sf.path
		JOIN codeindex_facts fto ON fto.snapshot_id = e.snapshot_id AND fto.kind = ? AND fto.path = tf.path
		WHERE e.snapshot_id = ?
			AND sf.path <> '' AND tf.path <> '' AND sf.path <> tf.path
		ORDER BY e.id`, int(pb.FactKind_FACT_KIND_FILE), int(pb.FactKind_FACT_KIND_FILE), snapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []FileEdge{}
	for rows.Next() {
		var from, to string
		var weight float64
		if err := rows.Scan(&from, &to, &weight); err != nil {
			return nil, err
		}
		out = append(out, FileEdge{FromFactID: from, ToFactID: to, Weight: weight})
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
	rows, err := s.bun.QueryContext(ctx, `
		SELECT ffrom.id, fto.id, e.kind, COALESCE(SUM(e.weight), 0) AS total, COUNT(*) AS observations
		FROM codeindex_edges e
		JOIN codeindex_facts sf ON sf.id = e.from_fact_id AND sf.snapshot_id = e.snapshot_id
		JOIN codeindex_facts tf ON tf.id = e.to_fact_id AND tf.snapshot_id = e.snapshot_id
		JOIN codeindex_facts ffrom ON ffrom.snapshot_id = e.snapshot_id AND ffrom.kind = ? AND ffrom.path = sf.path
		JOIN codeindex_facts fto ON fto.snapshot_id = e.snapshot_id AND fto.kind = ? AND fto.path = tf.path
		WHERE e.snapshot_id = ?
			AND sf.path <> '' AND tf.path <> '' AND sf.path <> tf.path
		GROUP BY ffrom.id, fto.id, e.kind
		ORDER BY ffrom.id, fto.id, e.kind`, int(pb.FactKind_FACT_KIND_FILE), int(pb.FactKind_FACT_KIND_FILE), snapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type pairKey struct{ from, to string }
	type accumulator struct {
		edge   FileEdge
		weight float64
		kinds  map[pb.EdgeKind]float64
	}
	acc := map[pairKey]*accumulator{}
	order := []pairKey{}
	for rows.Next() {
		var from, to string
		var kind int
		var total float64
		var observations int
		if err := rows.Scan(&from, &to, &kind, &total, &observations); err != nil {
			return nil, err
		}
		// Snapshots that store no edge weights fall back to the observation
		// count per kind, so both the pair weight and the dominant kind stay
		// meaningful.
		effective := total
		if effective <= 0 {
			effective = float64(observations)
		}
		key := pairKey{from: from, to: to}
		entry := acc[key]
		if entry == nil {
			entry = &accumulator{edge: FileEdge{FromFactID: from, ToFactID: to}, kinds: map[pb.EdgeKind]float64{}}
			acc[key] = entry
			order = append(order, key)
		}
		entry.weight += effective
		entry.kinds[pb.EdgeKind(kind)] += effective
	}
	out := make([]FileEdge, 0, len(order))
	for _, key := range order {
		entry := acc[key]
		edge := entry.edge
		edge.Weight = entry.weight
		edge.Kind = dominantEdgeKind(entry.kinds)
		out = append(out, edge)
	}
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
	rows, err := s.bun.QueryContext(ctx, `
		SELECT ffile.id, sf.imports_json
		FROM codeindex_facts sf
		JOIN codeindex_facts ffile ON ffile.snapshot_id = sf.snapshot_id AND ffile.kind = ? AND ffile.path = sf.path
		WHERE sf.snapshot_id = ?
			AND sf.path <> ''
			AND sf.imports_json NOT IN ('', 'null', '[]')`, int(pb.FactKind_FACT_KIND_FILE), snapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	seen := map[[2]string]struct{}{}
	out := []FileImport{}
	for rows.Next() {
		var fileID, raw string
		if err := rows.Scan(&fileID, &raw); err != nil {
			return nil, err
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
