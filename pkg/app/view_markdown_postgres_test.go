//go:build integration

package app

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestEnsureViewMarkdownTablePostgresRepeated(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	// Roll back an isolated schema so existing application data is untouched.
	if _, err := db.ExecContext(ctx, `BEGIN`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.ExecContext(ctx, `ROLLBACK`) }()
	schema := "markdown_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	for _, query := range []string{
		`CREATE SCHEMA ` + schema,
		`SET LOCAL search_path TO ` + schema,
		`CREATE TABLE views (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE view_markdown_documents (view_id INTEGER PRIMARY KEY REFERENCES views(id), path TEXT NOT NULL, is_managed INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	store := &Store{db: db, dialect: dbrepo.DialectPostgres}
	for range 3 {
		if err := store.ensureViewMarkdownTable(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// A swallowed duplicate-column error would leave this transaction aborted.
	if _, err := db.ExecContext(ctx, `SELECT org_id, source_kind FROM view_markdown_documents`); err != nil {
		t.Fatal(err)
	}
}
