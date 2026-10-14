package store

import (
	"context"
	"encoding/json"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/symbols"
)

func factFingerprint(f *pb.CodeFact, versions map[string]string) (string, error) {
	return rowFingerprint(factValues(symbols.ForFingerprint(f, versions)))
}

// The shared row retains its original literal symbol strings. Snapshot reads
// substitute the requesting snapshot's package versions without cloning rows.
func (s *Store) hydrateFactSymbolVersions(ctx context.Context, facts []*pb.CodeFact) error {
	ids := map[string]bool{}
	for _, f := range facts {
		if f.SymbolKey != "" {
			ids[f.SnapshotId] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids))
	for id := range ids {
		args = append(args, id)
	}
	where, scopeArgs := scope(ctx).clause("org_id")
	rows, err := s.bun.QueryContext(ctx, `SELECT id,symbol_versions_json FROM codeindex_snapshots WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`+where, append(args, scopeArgs...)...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	versions := map[string]map[string]string{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var value map[string]string
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return err
		}
		versions[id] = value
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, f := range facts {
		symbols.Restore(f, versions[f.SnapshotId])
	}
	return nil
}
