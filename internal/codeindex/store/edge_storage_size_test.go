package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/proto"
)

// Run against a backup, never the user's active database:
// TLD_EDGE_STORAGE_BENCHMARK_DB=/tmp/backup.db go test ./internal/codeindex/store -run TestEdgeStorageSize -v
func TestEdgeStorageSize(t *testing.T) {
	path := os.Getenv("TLD_EDGE_STORAGE_BENCHMARK_DB")
	if path == "" {
		t.Skip("set TLD_EDGE_STORAGE_BENCHMARK_DB to an isolated legacy SQLite backup")
	}
	source, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	rows, err := source.Query(`SELECT e.id,e.repository_id,e.snapshot_id,e.kind,e.from_fact_id,e.to_fact_id,
		e.target_symbol_key,e.logical_key,e.weight,e.anchor_json,e.evidence_json,e.org_id,
		COALESCE(sf.logical_key,''),COALESCE(tf.logical_key,'') FROM codeindex_edges e
		LEFT JOIN codeindex_facts sf ON sf.id=e.from_fact_id AND sf.org_id=e.org_id
		LEFT JOIN codeindex_facts tf ON tf.id=e.to_fact_id AND tf.org_id=e.org_id`)
	if err != nil {
		t.Fatal(err)
	}
	graphs := map[uuid.UUID]*graph.Graph{}
	var oldPayload, newPayload int64
	count := 0
	for rows.Next() {
		e := &pb.EdgeFact{}
		var kind int
		var oldKey, anchorJSON, evidenceJSON, org, fromKey, toKey string
		if err := rows.Scan(&e.Id, &e.RepositoryId, &e.SnapshotId, &kind, &e.FromFactId, &e.ToFactId,
			&e.TargetSymbolKey, &oldKey, &e.Weight, &anchorJSON, &evidenceJSON, &org, &fromKey, &toKey); err != nil {
			t.Fatal(err)
		}
		e.Kind = pb.EdgeKind(kind)
		if e.FromFactId == "0" {
			e.FromFactId = ""
		}
		if e.ToFactId == "0" {
			e.ToFactId = ""
		}
		if err := json.Unmarshal([]byte(anchorJSON), &e.Anchor); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(evidenceJSON), &e.Evidence); err != nil {
			t.Fatal(err)
		}
		target := e.TargetSymbolKey
		if e.ToFactId != "" {
			target = toKey
		}
		var qualifiers []string
		if _, name, ok := strings.Cut(oldKey, "|public-export|"); ok {
			qualifiers = []string{"public-export", name}
		}
		e.LogicalKey = graph.EdgeLogicalKey(e.Kind, fromKey, target, qualifiers...)
		details, err := marshalEdgeDetails(e.Anchor, e.Evidence)
		if err != nil {
			t.Fatal(err)
		}
		oldPayload += int64(len(oldKey) + len(anchorJSON) + len(evidenceJSON))
		newPayload += int64(32 + len(details))
		orgID, err := uuid.Parse(org)
		if err != nil {
			t.Fatal(err)
		}
		g := graphs[orgID]
		if g == nil {
			g = graph.NewGraph("", "")
			graphs[orgID] = g
		}
		g.EdgeFacts[e.Id] = e
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if count == 0 {
		t.Fatal("benchmark fixture contains no edges")
	}
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	for org, g := range graphs {
		ctx := app.WithTenantOrgID(context.Background(), org)
		if err := st.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error { return saveEdges(ctx, tx, g) }); err != nil {
			t.Fatal(err)
		}
		where, args := scope(ctx).clause("org_id")
		got, err := st.scanEdges(ctx, `SELECT `+edgeColumns+` FROM codeindex_edges WHERE 1=1`+where, args...)
		if err != nil || len(got) != len(g.EdgeFacts) {
			t.Fatalf("edge count changed: %d != %d, %v", len(got), len(g.EdgeFacts), err)
		}
		for _, e := range got {
			want := g.EdgeFacts[e.Id]
			if !proto.Equal(e, want) || (e.Evidence == nil) != (want.Evidence == nil) {
				t.Fatalf("edge %s failed lossless round trip", e.Id)
			}
		}
	}
	if _, err := st.bun.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
	pages := func(db *sql.DB) (table, indexes int64) {
		t.Helper()
		if err := db.QueryRow(`SELECT COALESCE(SUM(pgsize),0) FROM dbstat WHERE name='codeindex_edges'`).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT COALESCE(SUM(pgsize),0) FROM dbstat WHERE name IN
			(SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='codeindex_edges')`).Scan(&indexes); err != nil {
			t.Fatal(err)
		}
		return table, indexes
	}
	oldTable, oldIndexes := pages(source)
	newTable, newIndexes := pages(st.db)
	t.Logf("edges=%d payload=%d -> %d (%.1f%% reduction), table=%d -> %d, indexes=%d -> %d", count, oldPayload, newPayload, 100*(1-float64(newPayload)/float64(oldPayload)), oldTable, newTable, oldIndexes, newIndexes)
	if newPayload*100 > oldPayload*40 {
		t.Fatal("payload reduction below 60% target")
	}

	// Compare complete databases with all unrelated data retained. VACUUM both
	// sides so freelist/fragmentation savings aren't credited to the new format.
	wholePath := filepath.Join(t.TempDir(), "whole.db")
	if _, err := source.Exec(`VACUUM INTO ?`, wholePath); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(wholePath)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := sql.Open("sqlite", wholePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = whole.Close() }()
	whole.SetMaxOpenConns(1)
	var compactPath string
	if err := st.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&compactPath); err != nil {
		t.Fatal(err)
	}
	if _, err := whole.Exec(`ATTACH DATABASE ? AS compact`, compactPath); err != nil {
		t.Fatal(err)
	}
	schemaRows, err := st.db.Query(`SELECT sql FROM sqlite_master WHERE tbl_name='codeindex_edges' AND sql IS NOT NULL ORDER BY type DESC`)
	if err != nil {
		t.Fatal(err)
	}
	var schema []string
	for schemaRows.Next() {
		var statement string
		if err := schemaRows.Scan(&statement); err != nil {
			t.Fatal(err)
		}
		schema = append(schema, statement)
	}
	if err := schemaRows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = schemaRows.Close()
	tx, err := whole.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DROP TABLE codeindex_edges`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range schema {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO codeindex_edges SELECT * FROM compact.codeindex_edges`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := whole.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(wholePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("whole database (both vacuumed, unrelated data retained)=%d -> %d (%.1f%% reduction)", before.Size(), after.Size(), 100*(1-float64(after.Size())/float64(before.Size())))
}
