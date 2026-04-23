package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestRenameElement_CascadesPlacementsConnectorsAndMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`platform:
  name: Platform
  kind: workspace
  has_view: true
  placements:
    - parent: root
api:
  name: API
  kind: service
  placements:
    - parent: platform
_meta_elements:
  platform:
    id: 11
    updated_at: 2024-01-01T00:00:00Z
  api:
    id: 12
    updated_at: 2024-01-01T00:00:00Z
_meta_views:
  platform:
    id: 21
    updated_at: 2024-01-01T00:00:00Z
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(`platform:platform:api:contains:
  view: platform
  source: platform
  target: api
  label: contains
system:web:platform:calls:
  view: system
  source: web
  target: platform
  label: calls
_meta_connectors:
  platform:platform:api:contains:
    id: 31
    updated_at: 2024-01-01T00:00:00Z
  system:web:platform:calls:
    id: 32
    updated_at: 2024-01-01T00:00:00Z
`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.RenameElement(dir, "platform", "platform-core"); err != nil {
		t.Fatalf("RenameElement failed: %v", err)
	}

	elementsData, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	elementsText := string(elementsData)
	if strings.Contains(elementsText, "\nplatform:\n") || strings.HasPrefix(elementsText, "platform:\n") || !strings.Contains(elementsText, "platform-core:\n") {
		t.Fatalf("elements key was not renamed:\n%s", elementsText)
	}
	if !strings.Contains(elementsText, "parent: platform-core") {
		t.Fatalf("placement parent was not updated:\n%s", elementsText)
	}
	if strings.Contains(elementsText, "_meta_elements:") || strings.Contains(elementsText, "_meta_views:") || !strings.Contains(elementsText, "platform-core:") {
		t.Fatalf("element metadata was not updated:\n%s", elementsText)
	}
	lockFile, err := workspace.LoadLockFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lockFile.CurrentElements["platform-core"] == nil || lockFile.CurrentElements["platform"] != nil {
		t.Fatalf("lockfile current element metadata was not renamed: %+v", lockFile.CurrentElements)
	}
	if lockFile.CurrentViews["platform-core"] == nil || lockFile.CurrentViews["platform"] != nil {
		t.Fatalf("lockfile current view metadata was not renamed: %+v", lockFile.CurrentViews)
	}

	connectorsData, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	connectorsText := string(connectorsData)
	if !strings.Contains(connectorsText, "view: platform-core") || !strings.Contains(connectorsText, "source: platform-core") || !strings.Contains(connectorsText, "target: platform-core") {
		t.Fatalf("connector fields were not updated:\n%s", connectorsText)
	}
	if strings.Contains(connectorsText, "updated_at:") {
		t.Fatalf("connectors.yaml should not keep inline updated_at after rename with lockfile:\n%s", connectorsText)
	}
}

func TestRenameConnector(t *testing.T) {
	dir := t.TempDir()
	content := `system:api-handler:db:reads:
  view: system
  source: api-handler
  target: db
  label: reads
system:web:api-handler:calls:
  view: system
  source: web
  target: api-handler
  label: calls
_meta_connectors:
  system:api-handler:db:reads:
    id: c1
    updated_at: 2024-01-01T00:00:00Z
  system:web:api-handler:calls:
    id: c2
    updated_at: 2024-01-01T00:00:00Z
`
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.RenameConnector(dir, "api-handler", "api-handler-2"); err != nil {
		t.Fatalf("RenameConnector failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var got []workspace.Connector
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 connectors, got %d", len(got))
	}
	foundSourceRename := false
	foundTargetRename := false
	for _, conn := range got {
		if conn.Source == "api-handler-2" && conn.Target == "db" && conn.Label == "reads" {
			foundSourceRename = true
		}
		if conn.Source == "web" && conn.Target == "api-handler-2" && conn.Label == "calls" {
			foundTargetRename = true
		}
	}
	if !foundSourceRename {
		t.Fatalf("renamed source connector missing: %+v", got)
	}
	if !foundTargetRename {
		t.Fatalf("renamed target connector missing: %+v", got)
	}
}
