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

func TestPostgresFactsLiteralPathPrefix(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	ctx := context.Background()
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	schema := "facts_prefix_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	if _, err := db.ExecContext(ctx, `SET search_path TO "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	// Apply the production codeindex schema without unrelated workspace tables
	// and their optional extensions. Only the elements table is needed for the
	// migration's repository link; this test does not read or write elements.
	if _, err := db.ExecContext(ctx, `CREATE TABLE elements (id BIGINT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	migration, err := assets.FS.ReadFile("migrations/postgres/20260930000100_codeindex_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	st := NewStore(db, bun.NewDB(db, pgdialect.New()), dbrepo.DialectPostgres)
	testFactsLiteralPathPrefix(t, st)
}
