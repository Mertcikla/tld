package store

import (
	"context"
	"encoding/hex"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

const (
	factColumns = `id, repository_id, snapshot_id, language, kind, name, qualified_name, symbol_key, signature, documentation, body_hash, parent_fact_id, logical_key, path, anchor_json, evidence_json, imports_json`
	edgeColumns = `id, repository_id, snapshot_id, kind, from_fact_id, to_fact_id, target_symbol_key, logical_key, weight`

	factColumnsQualified = `f.id, f.repository_id, m.snapshot_id, f.language, f.kind, f.name, f.qualified_name, f.symbol_key, f.signature, f.documentation, f.body_hash, f.parent_fact_id, f.logical_key, f.path, f.anchor_json, f.evidence_json, f.imports_json`
	edgeColumnsQualified = `e.id, e.repository_id, m.snapshot_id, e.kind, e.from_fact_id, e.to_fact_id, e.target_symbol_key, e.logical_key, e.weight`
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
			&f.SymbolKey, &f.Signature, &f.Documentation, &f.BodyHash, &f.ParentFactId, &f.LogicalKey, &path,
			&anchor, &evidence, &imports); err != nil {
			return nil, err
		}
		f.Kind = pb.FactKind(kind)
		if f.ParentFactId == "0" {
			f.ParentFactId = ""
		}
		f.Anchor = unmarshalAnchor(anchor)
		f.Evidence = unmarshalEvidence(evidence)
		f.Imports = unmarshalStrings(imports)
		out = append(out, &f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.hydrateFactSymbolVersions(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
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
			e    pb.EdgeFact
			kind int
			key  []byte
		)
		if err := rows.Scan(&e.Id, &e.RepositoryId, &e.SnapshotId, &kind, &e.FromFactId, &e.ToFactId,
			&e.TargetSymbolKey, &key, &e.Weight); err != nil {
			return nil, err
		}
		e.Kind = pb.EdgeKind(kind)
		if e.FromFactId == "0" {
			e.FromFactId = ""
		}
		if e.ToFactId == "0" {
			e.ToFactId = ""
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("edge %s: invalid stored logical key length", e.Id)
		}
		e.LogicalKey = hex.EncodeToString(key)
		out = append(out, &e)
	}
	return out, rows.Err()
}
