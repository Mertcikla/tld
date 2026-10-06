package dbrepo_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"
	_ "modernc.org/sqlite"
)

func TestOpenSQLiteSkipsLegacyVectorTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	// Older releases created this virtual table with the sqlite-vec module,
	// which is no longer linked. Seed the schema entry directly to mimic such a
	// database without depending on the module.
	if _, err := db.ExecContext(ctx, `PRAGMA writable_schema = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sqlite_master (type, name, tbl_name, rootpage, sql)
		VALUES ('table', 'watch_embedding_vec', 'watch_embedding_vec', 0, 'CREATE VIRTUAL TABLE watch_embedding_vec USING vec(id)')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA writable_schema = RESET`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{SQLitePath: path, Migrations: assets.FS})
	if err != nil {
		t.Fatalf("OpenSQLite with legacy vector table: %v", err)
	}
	defer func() { _ = handle.Close() }()

	// The real shadow table from the removed watch pipeline is dropped.
	var shadow int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = '_vec_watch_embedding_vec'`).Scan(&shadow); err != nil {
		t.Fatal(err)
	}
	if shadow != 0 {
		t.Fatalf("legacy shadow table remains: %d", shadow)
	}

	// The virtual table needs the missing module to drop, so it is left as is.
	var vec int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'watch_embedding_vec'`).Scan(&vec); err != nil {
		t.Fatal(err)
	}
	if vec != 1 {
		t.Fatalf("legacy virtual table entries = %d, want it left untouched", vec)
	}
}

func TestSQLiteUpgradeRollsBackSchemaAndHistoryOnFailure(t *testing.T) {
	for _, failure := range []string{"codeindex-sql", "org-scoping-sql", "history-write"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "upgrade.db"))
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			b := bun.NewDB(db, sqlitedialect.New())
			defer func() { _ = b.Close() }()
			seedPreCodeindexSQLite(t, ctx, b)
			if _, err := db.ExecContext(ctx, `INSERT INTO tags(name, color) VALUES ('preserved', '#123456')`); err != nil {
				t.Fatal(err)
			}
			var inject, repair string
			switch failure {
			case "codeindex-sql":
				// Fail after the destructive legacy cleanup, at the final ALTER.
				inject = `ALTER TABLE elements ADD COLUMN repository_id TEXT`
				repair = `ALTER TABLE elements DROP COLUMN repository_id`
			case "org-scoping-sql":
				inject = `CREATE TRIGGER fail_upgrade BEFORE UPDATE ON tags BEGIN SELECT RAISE(ABORT, 'injected failure'); END`
				repair = `DROP TRIGGER fail_upgrade`
			case "history-write":
				inject = `CREATE TRIGGER fail_upgrade BEFORE INSERT ON bun_migrations WHEN NEW.name = '20260930000100' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`
				repair = `DROP TRIGGER fail_upgrade`
			}
			if _, err := db.ExecContext(ctx, inject); err != nil {
				t.Fatal(err)
			}
			if err := dbrepo.ApplyEmbeddedMigrations(ctx, b, assets.FS, "migrations"); err == nil {
				t.Fatal("expected injected upgrade failure")
			}
			var legacyTables, newTables, history, orgColumns int
			for query, dest := range map[string]*int{
				`SELECT COUNT(*) FROM sqlite_master WHERE name = 'watch_repositories'`:                           &legacyTables,
				`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('codeindex_repositories', 'tags_org_scoped')`: &newTables,
				`SELECT COUNT(*) FROM bun_migrations WHERE name >= '20260930000100'`:                             &history,
				`SELECT COUNT(*) FROM pragma_table_info('placements') WHERE name = 'org_id'`:                     &orgColumns,
			} {
				if err := db.QueryRowContext(ctx, query).Scan(dest); err != nil {
					t.Fatal(err)
				}
			}
			if legacyTables != 1 || newTables != 0 || history != 0 || orgColumns != 0 {
				t.Fatalf("partial upgrade: legacy=%d new=%d history=%d org columns=%d", legacyTables, newTables, history, orgColumns)
			}
			var color string
			if err := db.QueryRowContext(ctx, `SELECT color FROM tags WHERE name = 'preserved'`).Scan(&color); err != nil {
				t.Fatal(err)
			}
			if color != "#123456" {
				t.Fatalf("legacy tag changed: %s", color)
			}
			if _, err := db.ExecContext(ctx, repair); err != nil {
				t.Fatal(err)
			}
			// The same database must upgrade successfully once the cause is
			// removed, and reopening must not repeat committed schema changes.
			for range 2 {
				if err := dbrepo.ApplyEmbeddedMigrations(ctx, b, assets.FS, "migrations"); err != nil {
					t.Fatalf("retry upgrade: %v", err)
				}
			}
			if err := db.QueryRowContext(ctx, `SELECT color FROM tags WHERE name = 'preserved'`).Scan(&color); err != nil {
				t.Fatal(err)
			}
			if color != "#123456" {
				t.Fatalf("tag changed after retry: %s", color)
			}
		})
	}
}

func seedPreCodeindexSQLite(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(assets.FS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrator := migrate.NewMigrator(db, migrate.NewMigrations(), migrate.WithUpsert(true))
	if err := migrator.Init(ctx); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") || entry.Name() >= "20260930000100" {
			continue
		}
		raw, err := assets.FS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
		name, _, _ := strings.Cut(entry.Name(), "_")
		if err := migrator.MarkApplied(ctx, &migrate.Migration{Name: name, GroupID: 1}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenSQLiteAppliesLocalMigrations(t *testing.T) {
	ctx := context.Background()
	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{
		SQLitePath: filepath.Join(t.TempDir(), "tld.db"),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() { _ = handle.Close() }()

	if handle.Dialect != dbrepo.DialectSQLite {
		t.Fatalf("Dialect = %q, want %q", handle.Dialect, dbrepo.DialectSQLite)
	}
	var count int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags`).Scan(&count); err != nil {
		t.Fatalf("query migrated table: %v", err)
	}
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bun_migrations`).Scan(&count); err != nil {
		t.Fatalf("query bun migrations table: %v", err)
	}
	expectedMigrations := countSQLiteMigrations(t)
	if count != expectedMigrations {
		t.Fatalf("bun_migrations count = %d, want %d", count, expectedMigrations)
	}

	// Development databases may already record the former separate migration.
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO bun_migrations (name, group_id) VALUES ('20261003000200', 2)`); err != nil {
		t.Fatal(err)
	}
	if err := dbrepo.ApplyEmbeddedMigrations(ctx, handle.Bun, assets.FS, "migrations"); err != nil {
		t.Fatalf("reapply consolidated migrations with previous history: %v", err)
	}
}

func countSQLiteMigrations(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(assets.FS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			count++
		}
	}
	return count
}

func TestOpenSQLiteBootstrapsLegacyMigrationState(t *testing.T) {
	for _, last := range []int{1, 5} {
		t.Run(fmt.Sprintf("migrations-%d", last), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "legacy.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			names := []string{"20260524000100_init", "20260524000300_view_density_visibility_overrides", "20260524000400_missing_fk_indexes", "20260524000600_view_connector_tags", "20260530000100_element_noise_gate_bypass"}
			for _, name := range names[:last] {
				raw, err := assets.FS.ReadFile("migrations/" + name + ".up.sql")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, string(raw)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO tags (name, color) VALUES ('legacy-data', '#123456')`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{SQLitePath: path, Migrations: assets.FS})
				if err != nil {
					t.Fatal(err)
				}
				var name string
				if err := handle.DB.QueryRowContext(ctx, `SELECT name FROM tags WHERE name = 'legacy-data'`).Scan(&name); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bun_migrations`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != countSQLiteMigrations(t) {
					t.Fatalf("migration count = %d", count)
				}
				if err := handle.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
