package store

import (
	"context"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

const (
	factColumns  = `id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, code, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json`
	edgeColumns  = `id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight, anchor_json, evidence_json`
	chunkColumns = `id, fact_id, snapshot_id, anchor_json, text, context, idx, total`
)

func (s *Store) scanFacts(ctx context.Context, query string, args ...any) ([]*pb.CodeFact, error) {
	rows, err := s.bun.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*pb.CodeFact{}
	for rows.Next() {
		var (
			f                pb.CodeFact
			kind             int
			path             string
			anchor, evidence string
			imports          string
		)
		if err := rows.Scan(&f.Id, &f.RepositoryId, &f.SnapshotId, &f.Language, &kind, &f.Name, &f.QualifiedName,
			&f.SymbolKey, &f.Signature, &f.Documentation, &f.Code, &f.ParentFactId, &f.LogicalKey, &path,
			&anchor, &evidence, &imports); err != nil {
			return nil, err
		}
		f.Kind = pb.FactKind(kind)
		f.Anchor = unmarshalAnchor(anchor)
		f.Evidence = unmarshalEvidence(evidence)
		f.Imports = unmarshalStrings(imports)
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (s *Store) scanEdges(ctx context.Context, query string, args ...any) ([]*pb.EdgeFact, error) {
	rows, err := s.bun.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*pb.EdgeFact{}
	for rows.Next() {
		var (
			e                pb.EdgeFact
			kind             int
			anchor, evidence string
		)
		if err := rows.Scan(&e.Id, &e.RepositoryId, &e.SnapshotId, &kind, &e.FromFactId, &e.ToFactId,
			&e.TargetSymbolKey, &e.LogicalKey, &e.Weight, &anchor, &evidence); err != nil {
			return nil, err
		}
		e.Kind = pb.EdgeKind(kind)
		e.Anchor = unmarshalAnchor(anchor)
		e.Evidence = unmarshalEvidence(evidence)
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (s *Store) scanChunks(ctx context.Context, query string, args ...any) ([]*pb.Chunk, error) {
	rows, err := s.bun.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*pb.Chunk{}
	for rows.Next() {
		var (
			c      pb.Chunk
			anchor string
			index  int
			total  int
		)
		if err := rows.Scan(&c.Id, &c.FactId, &c.SnapshotId, &anchor, &c.Text, &c.Context, &index, &total); err != nil {
			return nil, err
		}
		c.Anchor = unmarshalAnchor(anchor)
		c.Index = uint32(index)
		c.Total = uint32(total)
		out = append(out, &c)
	}
	return out, rows.Err()
}
