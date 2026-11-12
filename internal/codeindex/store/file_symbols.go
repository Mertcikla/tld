package store

import (
	"context"
	"encoding/json"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// FileSymbolEdge retains declaration locations when a relationship is rolled up
// to files. Lines use the one-based coordinates of source-preview handles.
type FileSymbolEdge struct {
	FromPath, ToPath       string
	Kind                   pb.EdgeKind
	SourceLine, TargetLine uint32
}

func (s *Store) FileSymbolEdges(ctx context.Context, snapshotID string) ([]FileSymbolEdge, error) {
	where, args := scope(ctx).clause("e.org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT DISTINCT sf.path,tf.path,e.kind,sf.anchor_json,tf.anchor_json
 FROM codeindex_edges e
 JOIN codeindex_snapshot_edges m ON m.edge_id=e.id AND m.org_id=e.org_id
 JOIN codeindex_facts sf ON sf.id=e.from_fact_id AND sf.org_id=e.org_id
 JOIN codeindex_facts tf ON tf.id=e.to_fact_id AND tf.org_id=e.org_id
 WHERE m.snapshot_id=? AND sf.path<>'' AND tf.path<>'' AND sf.path<>tf.path`+where, append([]any{snapshotID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []FileSymbolEdge{}
	for rows.Next() {
		var edge FileSymbolEdge
		var from, to string
		if err := rows.Scan(&edge.FromPath, &edge.ToPath, &edge.Kind, &from, &to); err != nil {
			return nil, err
		}
		var source, target pb.SourceAnchor
		if err := json.Unmarshal([]byte(from), &source); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(to), &target); err != nil {
			return nil, err
		}
		edge.SourceLine, edge.TargetLine = source.StartLine+1, target.StartLine+1
		out = append(out, edge)
	}
	return out, rows.Err()
}
