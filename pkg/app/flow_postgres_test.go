//go:build integration

package app

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

func flowPostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("TLD_TEST_POSTGRES_URL must be set for integration tests")
	}
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = admin.Close() })
	schema := "flow_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
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
	return parsed.String()
}

func TestFlowPostgresCRUD(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T, *Store)
	}{
		{"CRUD and cascades", runFlowNormalizedCRUD}, {"atomic rollback", runFlowSaveRollback}, {"concurrency and snapshots", runFlowConcurrentRevisionAndSnapshots}, {"bulk and invalid trees", runFlowBulkEmptyPathsAndInvalidTrees}, {"schema constraints", runFlowSchemaConstraints},
	} {
		t.Run(test.name, func(t *testing.T) {
			dsn := flowPostgresDSN(t)
			s, err := OpenStoreWithOptions(t.Context(), dbrepo.DBOptions{Dialect: dbrepo.DialectPostgres, DatabaseURL: dsn, Migrations: assets.FS})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			test.run(t, s)
		})
	}
}
