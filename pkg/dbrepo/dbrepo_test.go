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
	_ "modernc.org/sqlite"
)

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
