package mcp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func TestMCPAddAutoAppliesLocalSQLite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_DATA_DIR", dataDir)

	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureGlobalConfig(); err != nil {
		t.Fatal(err)
	}

	wdir := dir
	format := "text"
	compact := false
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "tld-test", Version: "test"}, nil)
	registerTools(server, &cobra.Command{}, &wdir, &format, &compact, dataDir)
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "tld-test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()

	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "tld_add",
		Arguments: map[string]any{
			"name": "API",
			"ref":  "api",
			"kind": "service",
		},
	})
	if err != nil {
		t.Fatalf("call tld_add: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %+v", result.Content)
	}

	db := openMCPTestDB(t, dataDir)
	assertMCPCount(t, db, "SELECT COUNT(*) FROM elements", 1)
	// Bootstrap root view plus the element's owned canonical diagram.
	assertMCPCount(t, db, "SELECT COUNT(*) FROM views", 2)
	assertMCPCount(t, db, "SELECT COUNT(*) FROM views WHERE owner_element_id IS NOT NULL", 1)
}

func openMCPTestDB(t *testing.T, dataDir string) *sql.DB {
	t.Helper()
	sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Legacy().Close() })
	return sqliteStore.DB()
}

func assertMCPCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}
