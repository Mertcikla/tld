package mcp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func TestMCPAddAutoAppliesLocalSQLite(t *testing.T) {
	dataDir, clientSession := setupMCPWorkspace(t)

	result := mustCallTool(t, clientSession, "tld_add", map[string]any{
		"name": "API",
		"ref":  "api",
		"kind": "service",
	})

	db := openMCPTestDB(t, dataDir)
	assertMCPCount(t, db, "SELECT COUNT(*) FROM elements", 1)
	// Only the bootstrap root view: an element added at root has no diagram
	// until another element is placed under it.
	assertMCPCount(t, db, "SELECT COUNT(*) FROM views", 1)
	assertMCPCount(t, db, "SELECT COUNT(*) FROM views WHERE owner_element_id IS NOT NULL", 0)
	_ = result
}

func TestMCPListElements(t *testing.T) {
	_, clientSession := setupMCPWorkspace(t)

	mustCallTool(t, clientSession, "tld_add", map[string]any{"name": "API", "ref": "api", "kind": "service"})
	mustCallTool(t, clientSession, "tld_add", map[string]any{"name": "DB", "ref": "db", "kind": "database"})

	text := toolText(mustCallTool(t, clientSession, "tld_list_elements", map[string]any{"kind": "service"}))
	if !strings.Contains(text, "api") || strings.Contains(text, "db") {
		t.Fatalf("list elements kind filter output = %q", text)
	}

	text = toolText(mustCallTool(t, clientSession, "tld_list_elements", map[string]any{"search": "database"}))
	if !strings.Contains(text, "db") || strings.Contains(text, "api") {
		t.Fatalf("list elements search output = %q", text)
	}
}

func TestMCPViewCreateAndRender(t *testing.T) {
	_, clientSession := setupMCPWorkspace(t)

	mustCallTool(t, clientSession, "tld_add", map[string]any{"name": "API", "ref": "api", "kind": "service"})
	mustCallTool(t, clientSession, "tld_view_create", map[string]any{"ref": "api", "name": "API Diagram"})

	text := toolText(mustCallTool(t, clientSession, "tld_list_views", nil))
	if !strings.Contains(text, "api") {
		t.Fatalf("list views output = %q", text)
	}

	rendered := toolText(mustCallTool(t, clientSession, "tld_render", map[string]any{"view": "root"}))
	if !strings.Contains(rendered, "flowchart") || !strings.Contains(rendered, "API") {
		t.Fatalf("render output = %q", rendered)
	}
}

// setupMCPWorkspace initializes an isolated config/data dir and workspace, then
// returns the data dir and a connected in-memory MCP client session with every
// tool registered.
func setupMCPWorkspace(t *testing.T) (string, *mcpsdk.ClientSession) {
	t.Helper()
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
	registerViewTools(server, &wdir, &format, &compact, dataDir)
	registerQueryTools(server, &wdir, &format, &compact, dataDir)

	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "tld-test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return dataDir, clientSession
}

func mustCallTool(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("tool %s returned error: %s", name, toolText(result))
	}
	return result
}

func toolText(result *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(*mcpsdk.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
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
