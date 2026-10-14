//go:build integration

package materialize

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestPostgresIntegerIDUpgradePreservesSettingsAndDiagramOwnership(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = admin.Close() })
	for _, org := range []uuid.UUID{uuid.Nil, uuid.New()} {
		t.Run(org.String(), func(t *testing.T) {
			ctx := context.Background()
			schema := "integer_ids_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
					t.Error(err)
				}
			})
			db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
			db.SetMaxOpenConns(1)
			b := bun.NewDB(db, pgdialect.New())
			t.Cleanup(func() { _ = b.Close() })
			if _, err := db.ExecContext(ctx, `SET search_path TO "`+schema+`", public`); err != nil {
				t.Fatal(err)
			}
			testIntegerIDUpgrade(t, db, b, dbrepo.DialectPostgres, "migrations/postgres", org)
		})
	}
}
