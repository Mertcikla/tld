//go:build integration

package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestPostgresConcurrentRepositoryIdentity(t *testing.T) {
	first := openPostgresTestStore(t)
	var schema string
	if err := first.db.QueryRowContext(context.Background(), `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithDSN(os.Getenv("TLD_TEST_POSTGRES_URL")),
		pgdriver.WithConnParams(map[string]any{"search_path": schema}),
	))
	secondDB := bun.NewDB(db, pgdialect.New())
	t.Cleanup(func() { _ = secondDB.Close() })
	second := NewStore(db, secondDB, dbrepo.DialectPostgres)
	testConcurrentRepositoryIdentity(t, first, second)
}
