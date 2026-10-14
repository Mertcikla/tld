package materialize

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/community"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"
	"google.golang.org/protobuf/proto"
)

func TestIntegerIDUpgradePreservesSettingsAndDiagramOwnership(t *testing.T) {
	for _, cutoff := range []string{"20261014000100", "20261014000200"} {
		for _, org := range []uuid.UUID{uuid.Nil, uuid.New()} {
			t.Run(cutoff+"/"+org.String(), func(t *testing.T) {
				db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "upgrade.db"))
				if err != nil {
					t.Fatal(err)
				}
				db.SetMaxOpenConns(1)
				b := bun.NewDB(db, sqlitedialect.New())
				t.Cleanup(func() { _ = b.Close() })
				testIndexFormatUpgrade(t, db, b, dbrepo.DialectSQLite, "migrations", org, cutoff)
			})
		}
	}
}

func testIndexFormatUpgrade(t *testing.T, db *sql.DB, b *bun.DB, dialect dbrepo.Dialect, migrationsDir string, org uuid.UUID, cutoff string) {
	t.Helper()
	ctx := app.WithTenantOrgID(context.Background(), org)
	history := migrate.NewMigrator(b, migrate.NewMigrations(), migrate.WithUpsert(true))
	if err := history.Init(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(assets.FS, migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") || entry.Name() >= cutoff {
			continue
		}
		raw, err := assets.FS.ReadFile(migrationsDir + "/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
		name, _, _ := strings.Cut(entry.Name(), "_")
		if err := history.MarkApplied(ctx, &migrate.Migration{Name: name, GroupID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	ws := app.NewStore(db, b, dialect)
	idx := cstore.NewStore(db, b, dialect)
	root := t.TempDir()
	oldID := graph.RepositoryID(root)
	oldSnapshot := "old-snapshot"
	if cutoff == "20261014000200" {
		oldID, oldSnapshot = "1", "2"
	}
	if err := idx.EnsureRepositoryIdentity(ctx, oldID, root, "https://github.com/owner/repo", "github.com/owner/repo", true); err != nil {
		t.Fatal(err)
	}
	overrides := &pb.RepositoryMapConfiguration{MaxLeafFiles: proto.Uint32(7), IncludeExternalImports: proto.Bool(true)}
	if err := idx.SaveRepositoryMapOverrides(ctx, oldID, overrides); err != nil {
		t.Fatal(err)
	}
	files := groupMapFiles()
	for i := range files {
		files[i].ID = graph.LogicalFactKey(pb.FactKind_FACT_KIND_FILE, files[i].Path, files[i].Path)
		path := filepath.Join(root, files[i].Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package example\nfunc F() {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	input := GroupMapInput{RepositoryID: oldID, RepositoryName: "saved-repository", RepositoryRoot: root, SnapshotID: oldSnapshot, Files: files,
		Groups: []*community.Group{{Key: "alpha", Name: "alpha", Files: 2, Members: []int{0, 1}}, {Key: "beta", Name: "beta", Files: 2, Members: []int{2, 3}}},
		Edges:  []MapEdge{{FromFactID: files[0].ID, ToFactID: files[2].ID, Weight: 1, Kind: "calls"}}}
	opts := MapOptions{GroupLayers: true, AnnotateTags: true}
	before, err := ApplyGroupMap(ctx, ws, idx, input, opts)
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := idx.MappingsByRepository(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := b.NewRaw(`SELECT id FROM elements WHERE name=? ORDER BY id LIMIT 1`, "a.go").Scan(ctx, &fileID); err != nil {
		t.Fatal(err)
	}
	customName := "My edited file"
	if _, err := ws.UpdateElement(ctx, fileID, core.LibraryElement{Name: customName}); err != nil {
		t.Fatal(err)
	}
	var groupView int64
	if err := b.NewRaw(`SELECT v.id FROM views v JOIN elements e ON e.id=v.owner_element_id WHERE e.name=? ORDER BY v.id LIMIT 1`, "alpha").Scan(ctx, &groupView); err != nil {
		t.Fatal(err)
	}
	editedView := "My edited view"
	if _, err := ws.UpdateView(ctx, groupView, &editedView, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewRaw(`INSERT INTO codeindex_snapshots(id,repository_id,org_id) VALUES (?,?,?)`, oldSnapshot, oldID, org.String()).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewRaw(`INSERT INTO codeindex_blobs(hash,language,syntax_cache,org_id) VALUES ('old-cache','go','obsolete IDs',?)`, org.String()).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewRaw(`INSERT INTO codeindex_edges(id,repository_id,snapshot_id,logical_key,anchor_json,evidence_json,org_id)
		VALUES (?,?,?,?,?,?,?)`, "3", oldID, oldSnapshot, "edge|legacy", `{"path":"a.go"}`, `[{"producer":"scip"}]`, org.String()).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewRaw(`INSERT INTO codeindex_snapshot_edges(snapshot_id,edge_id,org_id) VALUES (?,?,?)`, oldSnapshot, "3", org.String()).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if dialect == dbrepo.DialectSQLite {
		if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_integer_ids BEFORE INSERT ON bun_migrations WHEN NEW.name='`+cutoff+`' BEGIN SELECT RAISE(ABORT,'injected history failure'); END`); err != nil {
			t.Fatal(err)
		}
		if err := dbrepo.ApplyEmbeddedMigrations(ctx, b, assets.FS, migrationsDir); err == nil {
			t.Fatal("expected upgrade rollback")
		}
		if registered, err := idx.RepositoryExists(ctx, oldID); err != nil || !registered {
			t.Fatalf("failed migration changed repository: %v", err)
		}
		if _, err := db.ExecContext(ctx, `DROP TRIGGER fail_integer_ids`); err != nil {
			t.Fatal(err)
		}
	}

	if err := dbrepo.ApplyEmbeddedMigrations(ctx, b, assets.FS, migrationsDir); err != nil {
		t.Fatal(err)
	}
	id, found, err := idx.RepositoryByRoot(ctx, root)
	if err != nil || !found {
		t.Fatalf("repository lost: %v", err)
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		t.Fatal(err)
	}
	if cutoff == "20261014000200" && id != oldID {
		t.Fatalf("edge format migration changed repository ID: %s -> %s", oldID, id)
	}
	var remoteURL string
	var managed bool
	if err := b.NewRaw(`SELECT remote_url, managed FROM codeindex_repositories WHERE id=? AND org_id=?`, id, org.String()).Scan(ctx, &remoteURL, &managed); err != nil || remoteURL != "https://github.com/owner/repo" || !managed {
		t.Fatalf("repository configuration lost: %s %v %v", remoteURL, managed, err)
	}
	namespace, err := idx.MappingNamespace(ctx, id)
	if err != nil || namespace != oldID {
		t.Fatalf("namespace=%q err=%v", namespace, err)
	}
	saved, err := idx.RepositoryMapOverrides(ctx, id)
	if err != nil || saved.GetMaxLeafFiles() != 7 || !saved.GetIncludeExternalImports() {
		t.Fatalf("settings lost: %v %v", saved, err)
	}
	for _, table := range []string{"codeindex_snapshots", "codeindex_facts", "codeindex_edges", "codeindex_blobs", "codeindex_artifacts", "codeindex_completed_maps"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	if latest, err := idx.Latest(ctx, id); err != nil || latest != "" {
		t.Fatalf("latest=%q err=%v", latest, err)
	}
	for _, mapping := range ownership {
		saved, found, err := idx.MappingByLogicalKey(ctx, mapping.LogicalKey)
		if err != nil || !found || saved.SnapshotID != "" || saved.ResourceID != mapping.ResourceID {
			t.Fatalf("ownership or stale snapshot reference after migration: %v %v", saved, err)
		}
	}
	linked, err := ws.ElementByID(ctx, fileID)
	if err != nil || linked.RepositoryID == nil || *linked.RepositoryID != id {
		t.Fatalf("element link lost: %v %v", linked, err)
	}
	snap, g, err := (indexer.Pipeline{Config: config.Default(), RepositoryID: id}).Build(ctx, &pb.IndexRequest{Directory: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Publish(ctx, root, snap, g); err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.ParseInt(snap.Id, 10, 64); err != nil {
		t.Fatal(err)
	}
	for _, fact := range g.Facts {
		if _, err := strconv.ParseInt(fact.Id, 10, 64); err != nil {
			t.Fatal(err)
		}
	}
	input.RepositoryID, input.MappingNamespace, input.SnapshotID = id, namespace, snap.Id
	after, err := ApplyGroupMap(ctx, ws, idx, input, opts)
	if err != nil {
		t.Fatal(err)
	}
	if before.ViewID != after.ViewID {
		t.Fatalf("map view replaced: %d -> %d", before.ViewID, after.ViewID)
	}
	for _, mapping := range ownership {
		current, found, err := idx.MappingByLogicalKey(ctx, mapping.LogicalKey)
		if err != nil || !found || current.ResourceID != mapping.ResourceID || current.RepositoryID != id {
			t.Fatalf("ownership changed: %v -> %v err=%v", mapping, current, err)
		}
	}
	edited, err := ws.ElementByID(ctx, fileID)
	if err != nil || edited.Name != customName {
		t.Fatalf("user name lost: %v %v", edited, err)
	}
	view, err := ws.ViewByID(ctx, groupView)
	if err != nil || view.Name != editedView {
		t.Fatalf("user view lost: %v %v", view, err)
	}
	if dialect == dbrepo.DialectPostgres {
		if _, err := b.NewRaw(`DELETE FROM bun_migrations WHERE name=?`, cutoff).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	if err := dbrepo.ApplyEmbeddedMigrations(ctx, b, assets.FS, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Snapshot(ctx, snap.Id); err != nil {
		t.Fatalf("reopen cleared the new graph: %v", err)
	}
}
