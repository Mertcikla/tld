package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/diag/tld/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestMergeWorkspace_WritesElementWorkspaceAndCleansLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "diagrams.yaml"), []byte("legacy: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	newWS := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"api": {Name: "API", Kind: "service", HasView: true, Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
			"db":  {Name: "DB", Kind: "database", Placements: []workspace.ViewPlacement{{ParentRef: "api"}}},
		},
		Connectors: map[string]*workspace.Connector{
			"api:api:db:reads": {View: "api", Source: "api", Target: "db", Label: "reads"},
		},
		Meta: &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: time.Now()}},
			Views:      map[string]*workspace.ResourceMetadata{"api": {ID: 2, UpdatedAt: time.Now()}},
			Connectors: map[string]*workspace.ResourceMetadata{"api:api:db:reads": {ID: 3, UpdatedAt: time.Now()}},
		},
	}

	if err := workspace.MergeWorkspace(dir, newWS, &workspace.Meta{}, &workspace.Meta{}); err != nil {
		t.Fatalf("MergeWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "diagrams.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy diagrams.yaml to be removed, err=%v", err)
	}
	elementsData, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(elementsData), "_meta_elements:") || !strings.Contains(string(elementsData), "_meta_views:") {
		t.Fatalf("elements metadata missing:\n%s", elementsData)
	}
	connectorsData, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(connectorsData), "_meta_connectors:") {
		t.Fatalf("connector metadata missing:\n%s", connectorsData)
	}
}

func TestMergeWorkspace_ServerWinsOnElementPlacementPositions(t *testing.T) {
	dir := t.TempDir()
	lastSyncTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`api:
  name: API
  kind: service
  placements:
    - parent: root
      position_x: 11
      position_y: 22
`), 0600); err != nil {
		t.Fatal(err)
	}
	lastSyncMeta := &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: lastSyncTime}},
		Views:    map[string]*workspace.ResourceMetadata{"api": {ID: 2, UpdatedAt: lastSyncTime}},
	}
	currentMeta := &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: lastSyncTime.Add(time.Minute)}},
		Views:    map[string]*workspace.ResourceMetadata{"api": {ID: 2, UpdatedAt: lastSyncTime.Add(time.Minute)}},
	}
	newWS := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"api": {Name: "API", Kind: "service", Placements: []workspace.ViewPlacement{{ParentRef: "root", PositionX: 55, PositionY: 66}}},
		},
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: lastSyncTime.Add(2 * time.Minute)}},
			Views:    map[string]*workspace.ResourceMetadata{"api": {ID: 2, UpdatedAt: lastSyncTime.Add(2 * time.Minute)}},
		},
	}

	if err := workspace.MergeWorkspace(dir, newWS, lastSyncMeta, currentMeta); err != nil {
		t.Fatalf("MergeWorkspace: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]workspace.Element
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["api"].Placements[0].PositionX != 55 || got["api"].Placements[0].PositionY != 66 {
		t.Fatalf("server placement should win, got %+v", got["api"].Placements[0])
	}
}
