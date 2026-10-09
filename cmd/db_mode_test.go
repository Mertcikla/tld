package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertNoWorkspaceFiles fails when DB-only commands wrote YAML into the
// workspace directory.
func assertNoWorkspaceFiles(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"elements.yaml", "connectors.yaml", ".tld.yaml", ".tld.lock"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Fatalf("DB mode wrote %s", name)
		}
	}
}

func TestDBModeAddListRender(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "API Service", "--technology", "go")
	MustRunCmd(t, dir, "add", "Database", "--kind", "database")
	MustRunCmd(t, dir, "add", "Child", "--parent", "api-service")

	assertNoWorkspaceFiles(t, dir)

	out, _ := MustRunCmd(t, dir, "list", "elements")
	for _, want := range []string{"API Service", "Database", "Child"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list elements missing %q:\n%s", want, out)
		}
	}

	out, _ = MustRunCmd(t, dir, "render", "root")
	if !strings.Contains(out, "API Service") {
		t.Fatalf("render root missing API Service:\n%s", out)
	}

	out, _ = MustRunCmd(t, dir, "list", "views", "--tree")
	if !strings.Contains(out, "api-service") {
		t.Fatalf("list views missing child view:\n%s", out)
	}
}

func TestDBModeConnectListRemove(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "API")
	MustRunCmd(t, dir, "add", "DB", "--kind", "database")
	MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db")

	assertNoWorkspaceFiles(t, dir)

	out, _ := MustRunCmd(t, dir, "list", "connectors")
	if !strings.Contains(out, "api") || !strings.Contains(out, "db") {
		t.Fatalf("list connectors missing endpoints:\n%s", out)
	}

	MustRunCmd(t, dir, "remove", "connector", "--view", "root", "--from", "api", "--to", "db")
	out, _ = MustRunCmd(t, dir, "list", "connectors")
	if strings.Contains(out, "root\tapi") {
		t.Fatalf("connector still present:\n%s", out)
	}
}

func TestDBModeRename(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "Old Name")
	MustRunCmd(t, dir, "rename", "--from", "old-name", "--to", "New Name")

	out, _ := MustRunCmd(t, dir, "list", "elements")
	if !strings.Contains(out, "New Name") || strings.Contains(out, "Old Name") {
		t.Fatalf("rename did not apply:\n%s", out)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeUpdateFields(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "Svc")
	MustRunCmd(t, dir, "update", "element", "svc", "technology", "go")
	MustRunCmd(t, dir, "update", "element", "svc", "language", "go")
	MustRunCmd(t, dir, "update", "element", "svc", "tags", "backend", "--append")

	out, _ := MustRunCmd(t, dir, "inspect", "svc")
	if !strings.Contains(out, "go") || !strings.Contains(out, "backend") {
		t.Fatalf("update not visible in inspect:\n%s", out)
	}

	_, _, err := RunCmd(t, dir, "update", "element", "svc", "owner", "payments")
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("owner update err = %v, want unsupported-field error", err)
	}
	_, _, err = RunCmd(t, dir, "update", "element", "svc", "ref", "renamed")
	if err == nil || !strings.Contains(err.Error(), "tld rename") {
		t.Fatalf("ref update err = %v, want tld rename hint", err)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeInspectTargetSource(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "API Service")

	out, _ := MustRunCmd(t, dir, "inspect", "api-service")
	if !strings.Contains(out, "API Service") {
		t.Fatalf("inspect missing element:\n%s", out)
	}
	if !strings.Contains(out, "target") || strings.Contains(out, "local_db") {
		t.Fatalf("inspect should report the target source only:\n%s", out)
	}
}

func TestDBModeValidateRequiresWorkspace(t *testing.T) {
	dir := t.TempDir()
	_, _, err := RunCmd(t, dir, "validate")
	if err == nil || !strings.Contains(err.Error(), "requires a workspace") {
		t.Fatalf("validate err = %v, want workspace-required error", err)
	}
}

func TestYamlFlagForcesWorkspaceMode(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "YAML Only", "--yaml")

	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read elements.yaml: %v", err)
	}
	if !strings.Contains(string(data), "YAML Only") {
		t.Fatalf("elements.yaml missing element:\n%s", data)
	}

	out, _ := MustRunCmd(t, dir, "list", "elements", "--yaml")
	if !strings.Contains(out, "YAML Only") {
		t.Fatalf("list --yaml missing element:\n%s", out)
	}
}

func TestDBModeRemoveElement(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "Gone")
	MustRunCmd(t, dir, "remove", "element", "gone")

	out, _ := MustRunCmd(t, dir, "list", "elements")
	if strings.Contains(out, "Gone") {
		t.Fatalf("element still present:\n%s", out)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeViewCreateAndDelete(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "Platform", "--kind", "workspace")
	MustRunCmd(t, dir, "view", "create", "platform", "--name", "Platform View")
	MustRunCmd(t, dir, "view", "set-level", "platform", "Container")

	out, _ := MustRunCmd(t, dir, "list", "views")
	if !strings.Contains(out, "Platform View") {
		t.Fatalf("view missing after create:\n%s", out)
	}

	MustRunCmd(t, dir, "view", "rename", "platform", "Renamed View")
	out, _ = MustRunCmd(t, dir, "list", "views")
	if !strings.Contains(out, "Renamed View") {
		t.Fatalf("view rename not applied:\n%s", out)
	}

	MustRunCmd(t, dir, "view", "delete", "platform")
	out, _ = MustRunCmd(t, dir, "list", "views")
	if !strings.Contains(out, "no") && strings.Contains(out, "Renamed View") {
		t.Fatalf("view still present after delete:\n%s", out)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeLinkExternal(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "Status Page")
	MustRunCmd(t, dir, "link", "status-page", "--external", "https://status.example.com")

	out, _ := MustRunCmd(t, dir, "inspect", "status-page")
	if !strings.Contains(out, "https://status.example.com") {
		t.Fatalf("link url missing from inspect:\n%s", out)
	}
	if !strings.Contains(out, "external") {
		t.Fatalf("external tag missing from inspect:\n%s", out)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeLinkUnlink(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "API")
	MustRunCmd(t, dir, "link", "api", "internal/api.go")

	out, _ := MustRunCmd(t, dir, "inspect", "api")
	if !strings.Contains(out, "internal/api.go") {
		t.Fatalf("file_path missing from inspect:\n%s", out)
	}

	MustRunCmd(t, dir, "link", "api", "--unlink")
	out, _ = MustRunCmd(t, dir, "inspect", "api")
	if strings.Contains(out, "internal/api.go") {
		t.Fatalf("link not cleared:\n%s", out)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeAddDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out, _ := MustRunCmd(t, dir, "add", "Preview", "--dry-run")
	if !strings.Contains(out, "dry-run") {
		t.Fatalf("dry-run output missing:\n%s", out)
	}
	listOut, _ := MustRunCmd(t, dir, "list", "elements")
	if strings.Contains(listOut, "Preview") {
		t.Fatalf("dry-run wrote to the database:\n%s", listOut)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModeImport(t *testing.T) {
	dir := t.TempDir()
	importFile := filepath.Join(t.TempDir(), "import.yaml")
	if err := os.WriteFile(importFile, []byte("elements:\n  api:\n    name: API\n    kind: service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	MustRunCmd(t, dir, "import", importFile)

	out, _ := MustRunCmd(t, dir, "list", "elements")
	if !strings.Contains(out, "API") {
		t.Fatalf("imported element missing:\n%s", out)
	}
	assertNoWorkspaceFiles(t, dir)
}

func TestDBModePullExportsYAML(t *testing.T) {
	dbDir := t.TempDir()
	MustRunCmd(t, dbDir, "add", "Exported")

	exportDir := t.TempDir()
	MustRunCmd(t, dbDir, "pull", "--workspace", exportDir)

	data, err := os.ReadFile(filepath.Join(exportDir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read exported elements.yaml: %v", err)
	}
	if !strings.Contains(string(data), "Exported") {
		t.Fatalf("exported elements.yaml missing element:\n%s", data)
	}
}
