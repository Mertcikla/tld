// Package store persists the codeindex graph on tld's bun-backed SQLite or
// Postgres database. It replaces the SurrealDB storage used by the standalone
// codeindex project while preserving its semantics: immutable snapshots,
// logical keys as canonical identity, and source-anchored evidence.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	codeindexstore "github.com/mertcikla/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
)

// CodeIndexStore is the persistence surface shared by the indexer, the
// projection layer, and the read APIs. The SQLite/Postgres bun implementation
// satisfies it.
type CodeIndexStore interface {
	Close() error
	Publish(ctx context.Context, root string, snap *pb.Snapshot, g *graph.Graph) error
	Snapshot(ctx context.Context, id string) (*pb.Snapshot, error)
	Snapshots(ctx context.Context, repositoryID string) ([]*pb.Snapshot, error)
	Latest(ctx context.Context, repositoryID string) (string, error)
	Repository(ctx context.Context, id string) (*pb.Repository, error)
	Source(ctx context.Context, hash string) ([]byte, error)
	Fact(ctx context.Context, id string) (*pb.CodeFact, error)
	Facts(ctx context.Context, snapshotID string, kind pb.FactKind, pathPrefix, after string, limit int) ([]*pb.CodeFact, error)
	FactVersions(ctx context.Context, logicalKey string) ([]*pb.CodeFact, error)
	EdgeFact(ctx context.Context, id string) (*pb.EdgeFact, error)
	EdgeFacts(ctx context.Context, snapshotID string, kind pb.EdgeKind, logicalKey, after string, limit int) ([]*pb.EdgeFact, error)
	EdgeVersions(ctx context.Context, logicalKey string) ([]*pb.EdgeFact, error)
	FileEdges(ctx context.Context, snapshotID string) ([]FileEdge, error)
	FileImports(ctx context.Context, snapshotID string) ([]FileImport, error)
	SaveAnalysis(ctx context.Context, run AnalysisRun) error
	LoadGraph(ctx context.Context, snapshotID string) (*graph.Graph, error)
	SnapshotSources(ctx context.Context, snapshotID string) (map[string]string, error)
	Diff(ctx context.Context, fromID, toID string, sourcesOnly bool) (*pb.SnapshotDiff, error)
	SaveMappings(ctx context.Context, mappings []ResourceMapping) error
	MappingByLogicalKey(ctx context.Context, logicalKey string) (ResourceMapping, bool, error)
	MappingByResource(ctx context.Context, kind MappingKind, resourceID int64) (ResourceMapping, bool, error)
	MappingsBySnapshot(ctx context.Context, snapshotID string) ([]ResourceMapping, error)
	MappingsByRepository(ctx context.Context, repositoryID string) ([]ResourceMapping, error)
	DeleteMapping(ctx context.Context, logicalKey string) error
	DeleteMappingsForSnapshot(ctx context.Context, snapshotID string) error
}

// Store is the bun-backed CodeIndexStore.
type Store struct {
	db      *sql.DB
	bun     *bun.DB
	dialect dbrepo.Dialect
}

var _ CodeIndexStore = (*Store)(nil)

// Store satisfies the codeindex module's thin persistence contract, so the
// engine's ingest and identity layers run unmodified against this database.
var _ codeindexstore.Store = (*Store)(nil)

// NewStore wraps an existing handle. Tables are created by the embedded
// migrations when the database is opened.
func NewStore(db *sql.DB, bunDB *bun.DB, dialect dbrepo.Dialect) *Store {
	return &Store{db: db, bun: bunDB, dialect: dialect}
}

// NewStoreFromHandle adapts a dbrepo handle.
func NewStoreFromHandle(handle *dbrepo.Handle) *Store {
	return &Store{db: handle.DB, bun: handle.Bun, dialect: handle.Dialect}
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Dialect() dbrepo.Dialect { return s.dialect }

// graphLoadLimit caps rows loaded for a full-graph read (diff, incremental base).
const graphLoadLimit = 1000000

func marshalJSON(v any) (string, error) {
	if v == nil {
		return "null", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalAnchor(raw string) *pb.SourceAnchor {
	if raw == "" || raw == "null" {
		return nil
	}
	var a pb.SourceAnchor
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil
	}
	return &a
}

func unmarshalEvidence(raw string) []*pb.Evidence {
	if raw == "" || raw == "null" {
		return nil
	}
	var out []*pb.Evidence
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalStrings(raw string) []string {
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 100
	case limit > graphLoadLimit:
		return graphLoadLimit
	default:
		return limit
	}
}

func notFound(kind, id string) error {
	return fmt.Errorf("%s %q not found", kind, id)
}
