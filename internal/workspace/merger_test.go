package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/tld/v2/internal/workspace"
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
			"api/api~db/reads": {View: "api", Source: "api", Target: "db", Label: "reads"},
		},
		Meta: &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: time.Now()}},
			Views:      map[string]*workspace.ResourceMetadata{"api": {ID: 2, UpdatedAt: time.Now()}},
			Connectors: map[string]*workspace.ResourceMetadata{"api/api~db/reads": {ID: 3, UpdatedAt: time.Now()}},
		},
	}

	if _, err := workspace.MergeWorkspace(dir, newWS, &workspace.Meta{}, &workspace.Meta{}); err != nil {
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

func TestMergeWorkspace_MigratesLegacyConnectorKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(`'platform:api:db:reads':
  view: platform
  source: api
  target: db
  label: reads
`), 0600); err != nil {
		t.Fatal(err)
	}
	newWS := &workspace.Workspace{
		Dir: dir,
		Connectors: map[string]*workspace.Connector{
			"platform/api~db/reads": {View: "platform", Source: "api", Target: "db", Label: "reads"},
		},
		Meta: &workspace.Meta{
			Connectors: map[string]*workspace.ResourceMetadata{"platform/api~db/reads": {ID: 7, UpdatedAt: time.Now()}},
		},
	}

	if _, err := workspace.MergeWorkspace(dir, newWS, &workspace.Meta{}, &workspace.Meta{}); err != nil {
		t.Fatalf("MergeWorkspace: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "platform:api:db:reads") {
		t.Fatalf("legacy connector key was not migrated:\n%s", text)
	}
	if !strings.Contains(text, "platform/api~db/reads") {
		t.Fatalf("canonical connector key missing:\n%s", text)
	}
}

func TestMergeWorkspace_PreservesListFormConnectors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(`- view: platform
  source: api
  target: db
  label: reads
- view: platform
  source: api
  target: cache
  label: writes
`), 0600); err != nil {
		t.Fatal(err)
	}
	newWS := &workspace.Workspace{
		Dir: dir,
		Connectors: map[string]*workspace.Connector{
			"platform/api~cache/writes":    {View: "platform", Source: "api", Target: "cache", Label: "writes"},
			"platform/api~queue/publishes": {View: "platform", Source: "api", Target: "queue", Label: "publishes"},
		},
		Meta: &workspace.Meta{
			Connectors: map[string]*workspace.ResourceMetadata{
				"platform/api~cache/writes":    {ID: 1, UpdatedAt: time.Now()},
				"platform/api~queue/publishes": {ID: 2, UpdatedAt: time.Now()},
			},
		},
	}

	if _, err := workspace.MergeWorkspace(dir, newWS, &workspace.Meta{}, &workspace.Meta{}); err != nil {
		t.Fatalf("MergeWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "_meta_connectors:") {
		t.Fatalf("connectors.yaml should stay a list, got map metadata section:\n%s", text)
	}
	if !strings.Contains(text, "- view: platform") {
		t.Fatalf("connectors.yaml should stay in list form:\n%s", text)
	}
	for _, want := range []string{"target: db", "target: cache", "target: queue"} {
		if !strings.Contains(text, want) {
			t.Fatalf("connectors.yaml missing %q:\n%s", want, text)
		}
	}
}

func TestPlanMergeWorkspaceReportsDeletionsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	elements := `api:
  name: API
  kind: service
db:
  name: DB
  kind: database
`
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(elements), 0600); err != nil {
		t.Fatal(err)
	}
	lastSyncTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	lastSyncMeta := &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{
			"api": {ID: 1, UpdatedAt: lastSyncTime},
			"db":  {ID: 2, UpdatedAt: lastSyncTime},
		},
	}
	currentMeta := &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{
			"api": {ID: 1, UpdatedAt: lastSyncTime},
			"db":  {ID: 2, UpdatedAt: lastSyncTime},
		},
	}
	emptyTarget := &workspace.Workspace{
		Dir:        dir,
		Elements:   map[string]*workspace.Element{},
		Connectors: map[string]*workspace.Connector{},
		Meta: &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{},
			Views:      map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{},
		},
	}

	result, err := workspace.PlanMergeWorkspace(dir, emptyTarget, lastSyncMeta, currentMeta)
	if err != nil {
		t.Fatalf("PlanMergeWorkspace: %v", err)
	}
	if result.TrackedElements != 2 || len(result.DeletedElements) != 2 {
		t.Fatalf("plan = %+v, want 2 tracked and 2 deleted elements", result)
	}

	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != elements {
		t.Fatalf("PlanMergeWorkspace modified elements.yaml:\n%s", data)
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

	if _, err := workspace.MergeWorkspace(dir, newWS, lastSyncMeta, currentMeta); err != nil {
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

func TestMergeWorkspace_LocalLinkConflictsWithConcurrentServerEdit(t *testing.T) {
	dir := t.TempDir()
	lastSync := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`api:
  name: API
  kind: service
  file_path: internal/api.go
`), 0600); err != nil {
		t.Fatal(err)
	}
	lastSyncMeta := &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: lastSync}},
	}
	// The local link advanced the current watermark past the last sync.
	currentMeta := &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: lastSync.Add(time.Minute)}},
	}
	// A concurrent server edit renamed the element and dropped its link.
	newWS := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"api": {Name: "API v2", Kind: "service"},
		},
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 1, UpdatedAt: lastSync.Add(2 * time.Minute)}},
		},
	}

	if _, err := workspace.MergeWorkspace(dir, newWS, lastSyncMeta, currentMeta); err != nil {
		t.Fatalf("MergeWorkspace: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "file_path: internal/api.go") {
		t.Fatalf("local link was silently dropped:\n%s", text)
	}
	if !strings.Contains(text, "CONFLICT") {
		t.Fatalf("concurrent server edit did not raise a conflict:\n%s", text)
	}
}
