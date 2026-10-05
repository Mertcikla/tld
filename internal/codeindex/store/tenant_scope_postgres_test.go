//go:build integration

package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

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
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	// Apply the production codeindex DDL without the retired watch pipeline's
	// pgvector dependency or unrelated workspace tables. Run the key migration
	// unchanged, in a transaction just as Bun runs the .tx.up.sql migration.
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(parsed.String())))
	defer func() { _ = db.Close() }()
	for _, migration := range []string{
		"20260930000100_codeindex_schema.up.sql",
		"20261005000100_org_scoping.up.sql",
		"20261005000200_codeindex_tenant_keys.tx.up.sql",
	} {
		raw, err := assets.FS.ReadFile("migrations/postgres/" + migration)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(migration, ".tx.up.sql") {
			if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
				_ = tx.Rollback()
				t.Fatalf("%s: %v", migration, err)
			}
		} else {
			ddl := regexp.MustCompile(`(?m)^--.*$`).ReplaceAllString(string(raw), "")
			for _, statement := range strings.Split(ddl, ";") {
				statement = strings.TrimSpace(statement)
				if !strings.HasPrefix(statement, "CREATE TABLE") && !strings.HasPrefix(statement, "ALTER TABLE") && !strings.HasPrefix(statement, "CREATE INDEX") && !strings.HasPrefix(statement, "CREATE UNIQUE INDEX") && !strings.HasPrefix(statement, "DROP INDEX") && !strings.HasPrefix(statement, "UPDATE") {
					continue
				}
				if !strings.Contains(statement, "TABLE codeindex_") && !strings.Contains(statement, "TABLE IF NOT EXISTS codeindex_") && !strings.Contains(statement, "ON codeindex_") && !strings.HasPrefix(statement, "UPDATE codeindex_") && !strings.Contains(statement, "DROP INDEX IF EXISTS idx_codeindex_") {
					continue
				}
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					_ = tx.Rollback()
					t.Fatalf("%s: %v", migration, err)
				}
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	st := NewStore(db, bun.NewDB(db, pgdialect.New()), dbrepo.DialectPostgres)
	t.Run("colliding keys", func(t *testing.T) { testTenantScopeCollidingKeys(t, st) })
	t.Run("lease and watch claims", func(t *testing.T) { testTenantScopeLeaseAndWatchClaims(t, st) })
}
