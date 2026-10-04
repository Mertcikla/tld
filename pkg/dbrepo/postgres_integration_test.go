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
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestOpenPostgresAppliesLocalMigrationsWhenConfigured(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	ctx := context.Background()
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
	var count int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_facts`).Scan(&count); err != nil {
		t.Fatalf("query migrated table: %v", err)
	}
}

func TestOpenPostgresBootstrapsLegacyMigrationState(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	ctx := context.Background()
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = admin.Close() }()
	schema := "legacy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	legacyDSN := parsed.String()
	legacy := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(legacyDSN)))
	defer func() { _ = legacy.Close() }()
	raw, err := assets.FS.ReadFile("migrations/postgres/20260524000100_local_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Keep the legacy tables isolated while resolving pgvector in its shared schema.
	if _, err := admin.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	legacySQL := strings.ReplaceAll(string(raw), "embedding vector,", "embedding public.vector,")
	if _, err := legacy.ExecContext(ctx, legacySQL); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `INSERT INTO tags (name,color) VALUES ('legacy-data','#123456')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		handle, err := dbrepo.OpenPostgres(ctx, dbrepo.DBOptions{DatabaseURL: legacyDSN, Migrations: assets.FS})
		if err != nil {
			t.Fatal(err)
		}
		var name string
		if err := handle.DB.QueryRowContext(ctx, `SELECT name FROM tags WHERE name = 'legacy-data'`).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
