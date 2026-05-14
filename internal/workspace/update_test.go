package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestUpdateElementField_UpdatesScalarField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`platform:
  name: Platform
  kind: workspace
`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.UpdateElementField(dir, "platform", "name", "Platform Core"); err != nil {
		t.Fatalf("UpdateElementField failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "name: Platform Core") {
		t.Fatalf("element name was not updated:\n%s", text)
	}
}

func TestUpdateElementField_UnknownFieldFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`platform:
  name: Platform
  kind: workspace
`), 0600); err != nil {
		t.Fatal(err)
	}

	err := workspace.UpdateElementField(dir, "platform", "bogus", "value")
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown element field") || !strings.Contains(err.Error(), "known fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateElementField_RefCascadesLikeRename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`platform:
  name: Platform
  kind: workspace
  has_view: true
api:
  name: API
  kind: service
  placements:
    - parent: platform
_meta_elements:
  platform:
    id: 11
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
`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.UpdateElementField(dir, "platform", "ref", "platform-core"); err != nil {
		t.Fatalf("UpdateElementField ref failed: %v", err)
	}

	elementsData, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(elementsData), "platform-core:") || strings.Contains(string(elementsData), "\nplatform:\n") {
		t.Fatalf("element ref was not renamed:\n%s", string(elementsData))
	}

	connectorsData, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	connectorsText := string(connectorsData)
	if !strings.Contains(connectorsText, "view: platform-core") || !strings.Contains(connectorsText, "source: platform-core") {
		t.Fatalf("connector fields were not cascaded:\n%s", connectorsText)
	}
}

func TestUpdateElementField_RefRejectsCollision(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`platform:
  name: Platform
  kind: workspace
api:
  name: API
  kind: service
`), 0600); err != nil {
		t.Fatal(err)
	}

	err := workspace.UpdateElementField(dir, "platform", "ref", "api")
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), `element "api" already exists`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateConnectorField_LabelRekeysConnector(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(`system:web:api:calls:
  view: system
  source: web
  target: api
  label: calls
_meta_connectors:
  system:web:api:calls:
    id: c1
    updated_at: 2024-01-01T00:00:00Z
`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.UpdateConnectorField(dir, "system:web:api:calls", "label", "reads"); err != nil {
		t.Fatalf("UpdateConnectorField failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "label: calls") {
		t.Fatalf("old connector label still exists:\n%s", text)
	}
	if !strings.Contains(text, "label: reads") {
		t.Fatalf("connector label was not updated:\n%s", text)
	}
}

func TestUpdateConnectorField_RekeyCollisionFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(`system:web:api:calls:
  view: system
  source: web
  target: api
  label: calls
system:web:api:reads:
  view: system
  source: web
  target: api
  label: reads
`), 0600); err != nil {
		t.Fatal(err)
	}

	err := workspace.UpdateConnectorField(dir, "system:web:api:calls", "label", "reads")
	if err == nil {
		t.Fatal("expected connector collision error")
	}
	if !strings.Contains(err.Error(), `connector "system:web:api:reads" already exists`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateConnectorField_UnknownFieldFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(`system:web:api:calls:
  view: system
  source: web
  target: api
  label: calls
`), 0600); err != nil {
		t.Fatal(err)
	}

	err := workspace.UpdateConnectorField(dir, "system:web:api:calls", "bogus", "value")
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown connector field") || !strings.Contains(err.Error(), "known fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}
