package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hashidlib "github.com/mertcikla/tld/internal/hashids"
	"github.com/mertcikla/tld/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"API Service", "api-service"},
		{"My DB!", "my-db"},
		{"  leading spaces  ", "leading-spaces"},
		{"multiple---hyphens", "multiple-hyphens"},
	}
	for _, tc := range cases {
		if got := workspace.Slugify(tc.in); got != tc.want {
			t.Fatalf("Slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUpsertElement_CreatesAndMergesPlacements(t *testing.T) {
	dir := t.TempDir()
	if err := workspace.UpsertElement(dir, "api", &workspace.Element{
		Name:       "API",
		Kind:       "service",
		Placements: []workspace.ViewPlacement{{ParentRef: "root", PositionX: 10, PositionY: 20}},
	}); err != nil {
		t.Fatalf("first UpsertElement: %v", err)
	}
	if err := workspace.UpsertElement(dir, "api", &workspace.Element{
		Name:        "API",
		Kind:        "service",
		Description: "Handles traffic",
		HasView:     true,
		ViewLabel:   "Container",
		Placements: []workspace.ViewPlacement{
			{ParentRef: "root", PositionX: 30, PositionY: 40},
			{ParentRef: "platform", PositionX: 50, PositionY: 60},
		},
	}); err != nil {
		t.Fatalf("second UpsertElement: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]workspace.Element
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	element := got["api"]
	if element.Description != "Handles traffic" || !element.HasView || element.ViewLabel != "Container" {
		t.Fatalf("unexpected merged element: %+v", element)
	}
	if len(element.Placements) != 2 {
		t.Fatalf("expected 2 placements, got %d", len(element.Placements))
	}
	if element.Placements[0].ParentRef != "root" || element.Placements[0].PositionX != 30 || element.Placements[0].PositionY != 40 {
		t.Fatalf("root placement not updated: %+v", element.Placements[0])
	}
}

func TestUpsertElement_ErrorsOnKindMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := workspace.UpsertElement(dir, "shared", &workspace.Element{Name: "Shared", Kind: "service"}); err != nil {
		t.Fatalf("first UpsertElement: %v", err)
	}
	err := workspace.UpsertElement(dir, "shared", &workspace.Element{Name: "Shared", Kind: "database"})
	if err == nil {
		t.Fatal("expected kind mismatch error")
	}
	if !strings.Contains(err.Error(), "already exists with kind") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpsertElement_PersistsElementAndViewMetadataToLockFile(t *testing.T) {
	dir := t.TempDir()
	content := "api:\n" +
		"  name: API\n" +
		"  kind: service\n" +
		"  placements:\n" +
		"    - parent: root\n" +
		"      position_x: 10\n" +
		"      position_y: 20\n" +
		"_meta_elements:\n" +
		"  api:\n" +
		"    id: 11\n" +
		"    updated_at: 2024-01-01T00:00:00Z\n" +
		"_meta_views:\n" +
		"  api:\n" +
		"    id: 21\n" +
		"    updated_at: 2024-01-02T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{Version: "v1"}); err != nil {
		t.Fatal(err)
	}

	if err := workspace.UpsertElement(dir, "api", &workspace.Element{
		Name:        "API",
		Kind:        "service",
		Description: "Updated",
		Placements:  []workspace.ViewPlacement{{ParentRef: "root", PositionX: 50, PositionY: 60}},
	}); err != nil {
		t.Fatalf("UpsertElement: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "_meta_elements:") || strings.Contains(text, "_meta_views:") {
		t.Fatalf("elements.yaml should not keep migrated metadata sections:\n%s", text)
	}
	lockFile, err := workspace.LoadLockFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lockFile.CurrentElements["api"] == nil || lockFile.CurrentElements["api"].ID != 11 {
		t.Fatalf("lockfile current element metadata missing: %+v", lockFile.CurrentElements)
	}
	if lockFile.CurrentViews["api"] == nil || lockFile.CurrentViews["api"].ID != 21 {
		t.Fatalf("lockfile current view metadata missing: %+v", lockFile.CurrentViews)
	}
}

func TestAppendConnector_WritesFlatList(t *testing.T) {
	dir := t.TempDir()
	encodedID := hashidlib.Encode(1)
	content := "- view: system\n" +
		"  source: api\n" +
		"  target: db\n" +
		"  label: reads\n" +
		"  id: " + encodedID + "\n" +
		"  updated_at: 2024-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.AppendConnector(dir, &workspace.Connector{
		View:   "system",
		Source: "api",
		Target: "queue",
		Label:  "publishes",
	}); err != nil {
		t.Fatalf("AppendConnector: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "_meta_connectors:") {
		t.Fatalf("connectors.yaml should not have _meta_connectors:\n%s", text)
	}
	if !strings.Contains(text, "- view: system") {
		t.Fatalf("connectors.yaml should use list format, got:\n%s", text)
	}
	if !strings.Contains(text, "label: publishes") || !strings.Contains(text, "target: queue") {
		t.Fatalf("connectors.yaml missing appended connector:\n%s", text)
	}
	if !strings.Contains(text, "id: "+encodedID) {
		t.Fatalf("connectors.yaml lost connector metadata:\n%s", text)
	}
}

func TestSave_WritesElementsAndConnectorsAndRemovesLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	for _, legacyFile := range []string{"diagrams.yaml", "objects.yaml", "edges.yaml", "links.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, legacyFile), []byte("legacy: true\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{Version: "v1"}); err != nil {
		t.Fatal(err)
	}

	ws := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"api": {
				Name:       "API",
				Kind:       "service",
				HasView:    true,
				Placements: []workspace.ViewPlacement{{ParentRef: "root"}},
			},
			"db": {
				Name:       "DB",
				Kind:       "database",
				Placements: []workspace.ViewPlacement{{ParentRef: "api"}},
			},
		},
		Connectors: map[string]*workspace.Connector{
			"api:api:db:reads": {View: "api", Source: "api", Target: "db", Label: "reads"},
		},
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{
				"api": {ID: 1, UpdatedAt: time.Now()},
			},
			Views: map[string]*workspace.ResourceMetadata{
				"api": {ID: 2, UpdatedAt: time.Now()},
			},
			Connectors: map[string]*workspace.ResourceMetadata{
				"api:api:db:reads": {ID: 3, UpdatedAt: time.Now()},
			},
		},
	}

	if err := workspace.Save(ws); err != nil {
		t.Fatalf("Save: %v", err)
	}

	elementsData, _ := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if strings.Contains(string(elementsData), "_meta_elements:") || strings.Contains(string(elementsData), "_meta_views:") {
		t.Fatalf("elements.yaml should not contain current metadata sections:\n%s", elementsData)
	}
	lockFile, err := workspace.LoadLockFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lockFile.CurrentElements["api"] == nil || lockFile.CurrentElements["api"].ID != 1 {
		t.Fatalf("lockfile current element metadata missing: %+v", lockFile.CurrentElements)
	}
	if lockFile.CurrentViews["api"] == nil || lockFile.CurrentViews["api"].ID != 2 {
		t.Fatalf("lockfile current view metadata missing: %+v", lockFile.CurrentViews)
	}
	if lockFile.CurrentConnectors["api:api:db:reads"] == nil || lockFile.CurrentConnectors["api:api:db:reads"].ID != 3 {
		t.Fatalf("lockfile current connector metadata missing: %+v", lockFile.CurrentConnectors)
	}
	connectorsData, _ := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if strings.Contains(string(connectorsData), "_meta_connectors:") || !strings.Contains(string(connectorsData), "label: reads") {
		t.Fatalf("connectors.yaml unexpected content:\n%s", connectorsData)
	}
	if !strings.Contains(string(connectorsData), "id: ") {
		t.Fatalf("connectors.yaml missing inline metadata:\n%s", connectorsData)
	}
	if strings.Contains(string(connectorsData), "updated_at:") {
		t.Fatalf("connectors.yaml should not keep inline updated_at when lockfile exists:\n%s", connectorsData)
	}
	for _, legacyFile := range []string{"diagrams.yaml", "objects.yaml", "edges.yaml", "links.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, legacyFile)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", legacyFile, err)
		}
	}
}
