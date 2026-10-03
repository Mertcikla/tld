// Package store persists the codeindex graph on tld's bun-backed SQLite or
// Postgres database. It replaces the SurrealDB storage used by the standalone
// codeindex project while preserving its semantics: immutable snapshots,
// logical keys as canonical identity, and source-anchored evidence.
package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
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
	Chunk(ctx context.Context, id string) (*pb.Chunk, error)
	Chunks(ctx context.Context, snapshotID string) ([]*pb.Chunk, error)
	UpdateSnapshot(ctx context.Context, snap *pb.Snapshot) error
	GetCachedEmbedding(ctx context.Context, key string) ([]float32, error)
	CacheEmbedding(ctx context.Context, key string, vector []float32) error
	SaveEmbedding(ctx context.Context, embedding *pb.Embedding) error
	SaveFactEmbedding(ctx context.Context, embedding *pb.Embedding) error
	SimilarFacts(ctx context.Context, snapshotID, profile string, query []float32, limit int) ([]FactScore, error)
	FactSimilarities(ctx context.Context, snapshotID, profile string, query []float32, factIDs []string) (map[string]float32, error)
	MajorityProfile(ctx context.Context, snapshotID string) (string, error)
	FactEmbeddings(ctx context.Context, snapshotID, profile string, kind pb.FactKind) ([]FactVector, error)
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

	vecOnce sync.Once
	vecErr  error
}

var _ CodeIndexStore = (*Store)(nil)

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

// encodeVector serialises float32s as little-endian bytes.
func encodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector parses little-endian float32 bytes.
func decodeVector(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
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
