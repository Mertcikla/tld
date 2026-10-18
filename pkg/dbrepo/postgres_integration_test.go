//go:build integration

package dbrepo_test

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/migrate"
)

func TestOpenPostgresAppliesLocalMigrationsWhenConfigured(t *testing.T) {
	dsn, _ := postgresMigrationTestDB(t)
	ctx := context.Background()
	for range 2 {
		handle, err := dbrepo.OpenPostgres(ctx, dbrepo.DBOptions{
			DatabaseURL: dsn,
			Migrations:  assets.FS,
		})
		if err != nil {
			t.Fatalf("OpenPostgres: %v", err)
		}
		defer func() { _ = handle.Close() }()

		if handle.Dialect != dbrepo.DialectPostgres {
			t.Fatalf("Dialect = %q, want %q", handle.Dialect, dbrepo.DialectPostgres)
		}
		assertPostgresMigrationState(t, handle.DB)
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenPostgresBootstrapsLegacyMigrationState(t *testing.T) {
	dsn, legacy := postgresMigrationTestDB(t)
	raw, err := assets.FS.ReadFile("migrations/postgres/20260524000100_local_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(t.Context(), string(raw)); err != nil {
		t.Fatal(err)
	}
	seedLegacyPostgresDiagram(t, legacy)
	assertPostgresUpgrade(t, dsn)
}

func TestOpenPostgresUpgradesRecordedMigrations(t *testing.T) {
	files := postgresMigrationFiles(t)
	for last, filename := range files {
		t.Run(filename, func(t *testing.T) {
			dsn, legacy := postgresMigrationTestDB(t)
			migrator := migrate.NewMigrator(bun.NewDB(legacy, pgdialect.New()), migrate.NewMigrations())
			if err := migrator.Init(t.Context()); err != nil {
				t.Fatal(err)
			}
			// Exercise every historical migration boundary with real diagram data.
			for i, name := range files[:last+1] {
				raw, err := assets.FS.ReadFile("migrations/postgres/" + name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := legacy.ExecContext(t.Context(), string(raw)); err != nil {
					t.Fatalf("seed migration %s: %v", name, err)
				}
				migration := &migrate.Migration{Name: strings.SplitN(name, "_", 2)[0], GroupID: 1}
				if err := migrator.MarkApplied(t.Context(), migration); err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					seedLegacyPostgresDiagram(t, legacy)
				}
			}
			assertPostgresUpgrade(t, dsn)
		})
	}
}

func postgresMigrationTestDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	ctx := context.Background()
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = admin.Close() })
	schema := "migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	scopedDSN := parsed.String()
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(scopedDSN)))
	t.Cleanup(func() { _ = db.Close() })
	return scopedDSN, db
}

func seedLegacyPostgresDiagram(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO elements (id, name, kind, created_at, updated_at)
		VALUES (1, 'API', 'service', '2026-05-24', '2026-05-24'),
		       (2, 'Database', 'database', '2026-05-24', '2026-05-24');
		INSERT INTO views (id, name, created_at, updated_at)
		VALUES (1, 'Architecture', '2026-05-24', '2026-05-24');
		INSERT INTO placements (view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (1, 1, 10, 20, '2026-05-24', '2026-05-24'),
		       (1, 2, 30, 40, '2026-05-24', '2026-05-24');
		INSERT INTO connectors (id, view_id, source_element_id, target_element_id, label, created_at, updated_at)
		VALUES (1, 1, 1, 2, 'reads', '2026-05-24', '2026-05-24');
		INSERT INTO tags (name, color) VALUES ('legacy-data', '#123456');
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func assertPostgresUpgrade(t *testing.T, dsn string) {
	t.Helper()
	ctx := t.Context()
	for range 2 {
		handle, err := dbrepo.OpenPostgres(ctx, dbrepo.DBOptions{DatabaseURL: dsn, Migrations: assets.FS})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = handle.Close() }()
		assertPostgresMigrationState(t, handle.DB)
		var source, target, label, view, color string
		var x, y float64
		err = handle.DB.QueryRowContext(ctx, `
			SELECT s.name, d.name, c.label, v.name, p.position_x, p.position_y
			FROM connectors c
			JOIN elements s ON s.id = c.source_element_id
			JOIN elements d ON d.id = c.target_element_id
			JOIN views v ON v.id = c.view_id
			JOIN placements p ON p.view_id = v.id AND p.element_id = s.id
			WHERE c.id = 1
		`).Scan(&source, &target, &label, &view, &x, &y)
		if err != nil {
			t.Fatal(err)
		}
		if source != "API" || target != "Database" || label != "reads" || view != "Architecture" || x != 10 || y != 20 {
			t.Fatalf("diagram changed after upgrade: %q -> %q, label=%q, view=%q, position=(%v, %v)", source, target, label, view, x, y)
		}
		if err := handle.DB.QueryRowContext(ctx, `SELECT color FROM tags WHERE name = 'legacy-data'`).Scan(&color); err != nil {
			t.Fatal(err)
		}
		if color != "#123456" {
			t.Fatalf("tag color = %q, want #123456", color)
		}
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func assertPostgresMigrationState(t *testing.T, db *sql.DB) {
	t.Helper()
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM codeindex_facts`: 0,
		`SELECT COUNT(*) FROM bun_migrations`:  len(postgresMigrationFiles(t)),
		`SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = current_schema() AND table_name IN ('watch_embeddings', 'watch_embedding_models')`: 0,
	} {
		var got int
		if err := db.QueryRowContext(t.Context(), query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %d, want %d", query, got, want)
		}
	}
}

func postgresMigrationFiles(t *testing.T) []string {
	t.Helper()
	entries, err := assets.FS.ReadDir("migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			files = append(files, entry.Name())
		}
	}
	return files
}
