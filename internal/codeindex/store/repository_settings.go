package store

import (
	"context"
	"database/sql"
	"errors"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// RepositoryMapOverrides reads the sparse overrides; missing fields inherit
// global defaults. A repository without settings has no overrides.
func (s *Store) RepositoryMapOverrides(ctx context.Context, repositoryID string) (*pb.RepositoryMapConfiguration, error) {
	var raw string
	err := s.bun.NewRaw(`SELECT map_overrides FROM codeindex_repository_settings WHERE repository_id = ?`, repositoryID).Scan(ctx, &raw)
	out := &pb.RepositoryMapConfiguration{}
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := protojson.Unmarshal([]byte(raw), out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveRepositoryMapOverrides replaces the complete sparse override set.
func (s *Store) SaveRepositoryMapOverrides(ctx context.Context, repositoryID string, overrides *pb.RepositoryMapConfiguration) error {
	if overrides == nil {
		overrides = &pb.RepositoryMapConfiguration{}
	}
	raw, err := protojson.Marshal(overrides)
	if err != nil {
		return err
	}
	_, err = s.bun.NewRaw(`INSERT INTO codeindex_repository_settings (repository_id, map_overrides)
		VALUES (?, ?) ON CONFLICT (repository_id) DO UPDATE SET map_overrides = excluded.map_overrides`, repositoryID, string(raw)).Exec(ctx)
	return err
}
