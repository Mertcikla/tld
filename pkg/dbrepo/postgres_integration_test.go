//go:build integration

package dbrepo_test

import (
	"context"
	"os"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
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
