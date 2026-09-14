package localserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/store"
)

func TestSeedWorkspaceImportsElements(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, ".tld.yaml"), []byte("project_name: test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	elementsYAML := `api:
  name: API
  kind: component
  has_view: false
  placements:
    - parent: root
db:
  name: DB
  kind: database
  has_view: false
  placements:
    - parent: root
`
	if err := os.WriteFile(filepath.Join(wsDir, "elements.yaml"), []byte(elementsYAML), 0600); err != nil {
		t.Fatal(err)
	}

	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqliteStore.Close() }()
	adapter := store.NewAPIAdapter(sqliteStore)

	seeded, err := seedWorkspace(context.Background(), adapter, wsDir)
	if err != nil {
		t.Fatalf("seedWorkspace: %v", err)
	}
	if !seeded {
		t.Fatal("expected the workspace to be seeded")
	}
	views, elements, connectors, err := adapter.GetWorkspaceResourceCounts(context.Background(), localWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if elements != 2 {
		t.Fatalf("elements = %d, want 2 (views=%d connectors=%d)", elements, views, connectors)
	}
}
