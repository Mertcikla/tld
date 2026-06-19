package dbrepo_test

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	sqlitevec "github.com/viant/sqlite-vec/vec"
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
}

func TestOpenSQLiteBootstrapsLegacyMigrationState(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tld.db")
	applySQLiteMigrationsWithoutTracking(t, dbPath)

	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{
		SQLitePath: dbPath,
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() { _ = handle.Close() }()

	var count int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bun_migrations`).Scan(&count); err != nil {
		t.Fatalf("query bun migrations table: %v", err)
	}
	expectedMigrations := countSQLiteMigrations(t)
	if count != expectedMigrations {
		t.Fatalf("bun_migrations count = %d, want %d", count, expectedMigrations)
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

func applySQLiteMigrationsWithoutTracking(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := sqlitevec.Register(db); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.ReadDir(assets.FS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		sqlBytes, err := assets.FS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", entry.Name(), err)
		}
	}
}
