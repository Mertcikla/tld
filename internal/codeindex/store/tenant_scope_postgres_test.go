//go:build integration

package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// TestPostgresTenantScopeCollidingKeys verifies tenant isolation against the
// real Postgres schema produced by the production migration set. The migrator
// runs every migration exactly as the server does, so the test fails if a
// migration drifts from what the store expects. A dedicated schema isolates
// this test from others sharing the same DSN.
func TestPostgresTenantScopeCollidingKeys(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	ctx := context.Background()
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = admin.Close() }()
	schema := "tenant_keys_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()

	// Point the session at the isolated schema. This must run on the same
	// connection the migrator uses, so the pool is capped at one connection.
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, `SET search_path TO "`+schema+`", public`); err != nil {
		t.Fatalf("set search path: %v", err)
	}

	bunDB := bun.NewDB(db, pgdialect.New())
	if err := dbrepo.ApplyEmbeddedMigrations(ctx, bunDB, assets.FS, "migrations/postgres"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	st := NewStore(db, bunDB, dbrepo.DialectPostgres)
	t.Run("colliding keys", func(t *testing.T) { testTenantScopeCollidingKeys(t, st) })
	t.Run("lease and watch claims", func(t *testing.T) { testTenantScopeLeaseAndWatchClaims(t, st) })
}
