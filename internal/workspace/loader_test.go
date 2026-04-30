package workspace_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hashidlib "github.com/mertcikla/tld/internal/hashids"
	"github.com/mertcikla/tld/internal/workspace"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func setupConfig(t *testing.T) string {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	return filepath.Join(configDir, "tld.yaml")
}

func minimalConfig() string {
	return "server_url: https://tldiagram.com\napi_key: \"\"\norg_id: \"\"\n"
}

func TestLoad_MinimalWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Elements) != 0 || len(ws.Connectors) != 0 {
		t.Fatalf("expected empty workspace, got %d elements and %d connectors", len(ws.Elements), len(ws.Connectors))
	}
}

func TestLoad_MissingConfigFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	setupConfig(t) // config file doesn't exist yet

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.ServerURL != "https://tldiagram.com" {
		t.Fatalf("expected default ServerURL, got %q", ws.Config.ServerURL)
	}
}

func TestLoad_MalformedConfigYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), ":\t:\n")

	_, err := workspace.Load(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse global config") {
		t.Fatalf("error %q does not contain 'parse global config'", err.Error())
	}
}

func TestLoad_APIKeyFromEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	t.Setenv("TLD_API_KEY", "env-test-key")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.APIKey != "env-test-key" {
		t.Fatalf("APIKey = %q, want env-test-key", ws.Config.APIKey)
	}
}

func TestLoad_EnvOverridesAPIKeyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), "server_url: http://localhost\napi_key: file-key\norg_id: \"\"\n")
	t.Setenv("TLD_API_KEY", "env-key")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.APIKey != "env-key" {
		t.Fatalf("APIKey = %q, want env-key", ws.Config.APIKey)
	}
}

func TestLoad_WorkspaceConfigLoaded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, ".tld.yaml"), "project_name: Demo\nexclude:\n  - vendor/\n  - \"**/*.pb.go\"\nrepositories:\n  frontend:\n    url: github.com/example/frontend\n    localDir: frontend\n    root: root-ref\n    config:\n      mode: auto\n    exclude:\n      - \"**/*_test.go\"\n      - init*\n")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.WorkspaceConfig == nil {
		t.Fatal("expected workspace config to be loaded")
	}
	repo := ws.WorkspaceConfig.Repositories["frontend"]
	if ws.WorkspaceConfig.ProjectName != "Demo" || repo.URL != "github.com/example/frontend" || repo.LocalDir != "frontend" || repo.Root != "root-ref" {
		t.Fatalf("unexpected workspace config: %+v / %+v", ws.WorkspaceConfig, repo)
	}
	if repo.Config == nil || repo.Config.Mode != "auto" {
		t.Fatalf("unexpected repository mode: %+v", repo.Config)
	}
	rules := ws.IgnoreRulesForRepository("frontend")
	if rules == nil || !rules.ShouldIgnorePath("pkg/example.pb.go") || !rules.ShouldIgnoreSymbol("initHelper") {
		t.Fatalf("expected merged ignore rules, got %+v", rules)
	}
}

func TestLoad_DefaultsRepositoryModeToUpsert(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, ".tld.yaml"), "repositories:\n  frontend:\n    url: github.com/example/frontend\n    localDir: frontend\n")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	repo := ws.WorkspaceConfig.Repositories["frontend"]
	if repo.Config == nil || repo.Config.Mode != "upsert" {
		t.Fatalf("repository mode = %+v, want upsert", repo.Config)
	}
}

func TestLoad_ElementsLoadedAndMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "elements.yaml"), `api:
  name: API
  kind: service
  description: Handles traffic
  has_view: true
  view_label: Container
  placements:
    - parent: root
      position_x: 100
      position_y: 50
_meta_elements:
  api:
    id: 101
    updated_at: 2024-03-24T10:00:00Z
_meta_views:
  api:
    id: 202
    updated_at: 2024-03-24T11:00:00Z
`)

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Elements) != 1 {
		t.Fatalf("len(Elements) = %d, want 1", len(ws.Elements))
	}
	api := ws.Elements["api"]
	if api == nil || api.Kind != "service" || !api.HasView || len(api.Placements) != 1 {
		t.Fatalf("unexpected element: %+v", api)
	}
	if ws.Meta.Elements["api"].ID != 101 || ws.Meta.Views["api"].ID != 202 {
		t.Fatalf("unexpected metadata: elements=%+v views=%+v", ws.Meta.Elements["api"], ws.Meta.Views["api"])
	}
}

func TestLoad_ElementsMetadataFromLockFileCurrentEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "elements.yaml"), `api:
  name: API
  kind: service
  has_view: true
  placements:
    - parent: root
_meta_views:
  api:
    id: 202
    updated_at: 2024-03-24T11:00:00Z
`)
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{
		Version: "v1",
		Metadata: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 999, UpdatedAt: time.Date(2024, 3, 24, 10, 0, 0, 0, time.UTC)}},
			Views:    map[string]*workspace.ResourceMetadata{"api": {ID: 888, UpdatedAt: time.Date(2024, 3, 24, 10, 30, 0, 0, time.UTC)}},
		},
		CurrentElements: map[string]*workspace.ResourceMetadata{"api": {ID: 101, UpdatedAt: time.Date(2024, 3, 24, 12, 0, 0, 0, time.UTC)}},
		CurrentViews:    map[string]*workspace.ResourceMetadata{"api": {ID: 303, UpdatedAt: time.Date(2024, 3, 24, 12, 30, 0, 0, time.UTC)}},
	}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Meta.Elements["api"].ID != 101 {
		t.Fatalf("expected current lockfile element metadata to win, got %+v", ws.Meta.Elements["api"])
	}
	if ws.Meta.Views["api"].ID != 303 {
		t.Fatalf("expected current lockfile view metadata to win, got %+v", ws.Meta.Views["api"])
	}
}

func TestLoad_ElementsMetadataAllowsRefsNamedLikeElementFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "elements.yaml"), `language:
  name: Language
  kind: service
  placements:
    - parent: root
symbol:
  name: Symbol
  kind: service
  placements:
    - parent: root
_meta_elements:
  language:
    id: 101
    updated_at: 2024-03-24T10:00:00Z
  symbol:
    id: 202
    updated_at: 2024-03-24T11:00:00Z
`)

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Elements) != 2 {
		t.Fatalf("len(Elements) = %d, want 2", len(ws.Elements))
	}
	if ws.Elements["language"] == nil || ws.Elements["symbol"] == nil {
		t.Fatalf("expected language and symbol elements, got %+v", ws.Elements)
	}
	if ws.Meta.Elements["language"].ID != 101 || ws.Meta.Elements["symbol"].ID != 202 {
		t.Fatalf("unexpected metadata: %+v", ws.Meta.Elements)
	}
}

func TestLoad_MalformedElementsYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "elements.yaml"), ":\t:\n")

	_, err := workspace.Load(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse elements.yaml") {
		t.Fatalf("error %q does not contain 'parse elements.yaml'", err.Error())
	}
}

func TestLoad_ConnectorsLoadedAndMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "connectors.yaml"), `system:api:db:reads:
  view: system
  source: api
  target: db
  label: reads
_meta_connectors:
  system:api:db:reads:
    id: 303
    updated_at: 2024-03-24T12:00:00Z
`)

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Connectors) != 1 {
		t.Fatalf("len(Connectors) = %d, want 1", len(ws.Connectors))
	}
	conn := ws.Connectors["system:api:db:reads"]
	if conn == nil || conn.Source != "api" || conn.Target != "db" {
		t.Fatalf("unexpected connector: %+v", conn)
	}
	if ws.Meta.Connectors["system:api:db:reads"].ID != 303 {
		t.Fatalf("unexpected connector metadata: %+v", ws.Meta.Connectors["system:api:db:reads"])
	}
}

func TestLoad_ConnectorsListFormatLoadsInlineMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	encodedID := hashidlib.Encode(303)
	writeFile(t, filepath.Join(dir, "connectors.yaml"), fmt.Sprintf("- view: system\n  source: api\n  target: db\n  label: reads\n  id: %s\n  updated_at: 2024-03-24T12:00:00Z\n", encodedID))

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Connectors) != 1 {
		t.Fatalf("len(Connectors) = %d, want 1", len(ws.Connectors))
	}
	if ws.Meta.Connectors["system:api:db:reads"].ID != 303 {
		t.Fatalf("unexpected connector metadata: %+v", ws.Meta.Connectors["system:api:db:reads"])
	}
	if got := ws.Meta.Connectors["system:api:db:reads"].UpdatedAt.Format("2006-01-02T15:04:05Z07:00"); got != "2024-03-24T12:00:00Z" {
		t.Fatalf("unexpected connector updated_at: %s", got)
	}
}

func TestLoad_ConnectorMetadataFromLockFileCurrentConnectors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	encodedID := hashidlib.Encode(303)
	writeFile(t, filepath.Join(dir, "connectors.yaml"), fmt.Sprintf("- view: system\n  source: api\n  target: db\n  label: reads\n  id: %s\n", encodedID))
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{
		Version: "v1",
		Metadata: &workspace.Meta{
			Connectors: map[string]*workspace.ResourceMetadata{"system:api:db:reads": {ID: 404, UpdatedAt: time.Date(2024, 3, 24, 10, 0, 0, 0, time.UTC)}},
		},
		CurrentConnectors: map[string]*workspace.ResourceMetadata{"system:api:db:reads": {ID: 505, UpdatedAt: time.Date(2024, 3, 24, 12, 0, 0, 0, time.UTC)}},
	}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Meta.Connectors["system:api:db:reads"].ID != 505 {
		t.Fatalf("expected current lockfile connector metadata to win, got %+v", ws.Meta.Connectors["system:api:db:reads"])
	}
	if got := ws.Meta.Connectors["system:api:db:reads"].UpdatedAt.Format("2006-01-02T15:04:05Z07:00"); got != "2024-03-24T12:00:00Z" {
		t.Fatalf("unexpected connector updated_at from current lockfile metadata: %s", got)
	}
}

func TestLoad_EmptyConnectorList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "connectors.yaml"), "[]\n")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Connectors) != 0 {
		t.Fatalf("len(Connectors) = %d, want 0", len(ws.Connectors))
	}
}

func TestLoad_MalformedConnectorsYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, setupConfig(t), minimalConfig())
	writeFile(t, filepath.Join(dir, "connectors.yaml"), ":\t:\n")

	_, err := workspace.Load(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse connectors.yaml") {
		t.Fatalf("error %q does not contain 'parse connectors.yaml'", err.Error())
	}
}
