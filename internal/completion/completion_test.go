package completion_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mertcikla/tld/internal/completion"
)

func setupConfig(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "tld.yaml"), []byte("server_url: https://tldiagram.com\napi_key: \"\"\norg_id: \"\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func writeWS(t *testing.T, dir, elements, connectors string) {
	t.Helper()
	if elements != "" {
		if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(elements), 0o600); err != nil {
			t.Fatalf("write elements: %v", err)
		}
	}
	if connectors != "" {
		if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(connectors), 0o600); err != nil {
			t.Fatalf("write connectors: %v", err)
		}
	}
}

func TestElementRefs_SortedAndUnique(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	writeWS(t, dir,
		"zeta:\n  name: Zeta\n  kind: service\nalpha:\n  name: Alpha\n  kind: database\nmid:\n  name: Mid\n  kind: service\n",
		"")
	refs, dir2 := completion.ElementRefs(&dir)
	_ = dir2
	want := []string{"alpha", "mid", "zeta"}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
}

func TestElementRefs_MissingFileEmpty(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	refs, _ := completion.ElementRefs(&dir)
	if len(refs) != 0 {
		t.Fatalf("expected empty, got %v", refs)
	}
}

func TestElementRefs_MalformedSilent(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	writeWS(t, dir, "not: valid: yaml: here\n  bad\n", "")
	refs, _ := completion.ElementRefs(&dir)
	if len(refs) != 0 {
		t.Fatalf("expected empty on bad yaml, got %v", refs)
	}
}

func TestElementRefsWithNames_FormatsTabDescription(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	writeWS(t, dir,
		"api:\n  name: API Service\n  kind: service\ndb:\n  name: Postgres\n  kind: database\n",
		"")
	refs, _ := completion.ElementRefsWithNames(&dir)
	want := []string{"api\tAPI Service", "db\tPostgres"}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
}

func TestConnectorKeys(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	writeWS(t, dir,
		"api:\n  name: API\n  kind: service\ndb:\n  name: DB\n  kind: database\n",
		"connectors:\n  - view: root\n    source: api\n    target: db\n")
	keys, _ := completion.ConnectorKeys(&dir)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %v", keys)
	}
}

func TestParentRefs_IncludesRoot(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	writeWS(t, dir, "api:\n  name: API\n  kind: service\n", "")
	refs, _ := completion.ParentRefs(&dir)
	found := false
	for _, r := range refs {
		if r == "root" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ParentRefs missing 'root': %v", refs)
	}
}

func TestViewRefs_OnlyHasViewElements(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	writeWS(t, dir,
		"api:\n  name: API\n  kind: service\n  has_view: true\ndb:\n  name: DB\n  kind: database\n",
		"")
	views, _ := completion.ViewRefs(&dir)
	want := []string{"api", "root"}
	if !reflect.DeepEqual(views, want) {
		t.Fatalf("views = %v, want %v", views, want)
	}
}

func TestStaticSetsNonEmpty(t *testing.T) {
	if len(completion.ElementFields()) == 0 {
		t.Fatal("ElementFields empty")
	}
	if len(completion.ConnectorFields()) == 0 {
		t.Fatal("ConnectorFields empty")
	}
	if len(completion.ElementKinds()) == 0 {
		t.Fatal("ElementKinds empty")
	}
	if len(completion.ConnectorDirections()) == 0 {
		t.Fatal("ConnectorDirections empty")
	}
}

func TestNilWdirNoPanic(t *testing.T) {
	setupConfig(t)
	refs, _ := completion.ElementRefs(nil)
	if len(refs) != 0 {
		t.Fatalf("expected empty, got %v", refs)
	}
}
