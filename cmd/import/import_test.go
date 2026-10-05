package importcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestImportCmd_PreservesOmittedVisibility(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--bypass-noise-gate=false")
	file := writeImportFile(t, "elements:\n  api:\n    name: API\n    description: edited\n")
	cmd.MustRunCmd(t, dir, "import", file)
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := exec.NewRunner(ws.Config, "local", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	el, err := runner.GetElement(context.Background(), int32(ws.Meta.Elements["api"].ID))
	if err != nil {
		t.Fatal(err)
	}
	if el.GetBypassNoiseGate() || el.GetDescription() != "edited" {
		t.Fatalf("import changed omitted visibility or lost description: %v", el)
	}
	if ws.Elements["api"].BypassNoiseGate == nil || *ws.Elements["api"].BypassNoiseGate {
		t.Fatal("local cache lost visibility setting")
	}
}

func TestImportCmd_ConnectorOnlyPreservesViewName(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api")
	cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db")
	cmd.MustRunCmd(t, dir, "view", "create", "api", "--name", "Custom diagram")
	file := writeImportFile(t, "connectors:\n  - view: api\n    source: api\n    target: db\n")
	cmd.MustRunCmd(t, dir, "import", file)
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := exec.NewRunner(ws.Config, "local", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	views, err := runner.ListViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.GetId() == int32(ws.Meta.Views["api"].ID) {
			if view.GetName() != "Custom diagram" {
				t.Fatalf("view name = %q", view.GetName())
			}
			return
		}
	}
	t.Fatal("import lost existing view")
}

const basicImport = `
elements:
  api:
    name: API
    kind: service
    placements:
      - parent: root
  db:
    name: Database
    kind: database
    placements:
      - parent: root
connectors:
  - view: root
    source: api
    target: db
    label: reads
`

func writeImportFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "import.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write import file: %v", err)
	}
	return path
}

func TestImportCmd_CreatesElementsAndConnectors(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, basicImport)

	stdout, _, err := cmd.RunCmd(t, dir, "import", file)
	if err != nil {
		t.Fatalf("import: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stdout, "import:") {
		t.Fatalf("expected import summary, got:\n%s", stdout)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if len(ws.Elements) != 2 {
		t.Fatalf("elements = %d, want 2", len(ws.Elements))
	}
	if len(ws.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1", len(ws.Connectors))
	}
	if ws.Elements["api"] == nil || ws.Elements["db"] == nil {
		t.Fatalf("missing imported elements: %#v", ws.Elements)
	}
	if ws.Meta == nil || ws.Meta.Elements["api"].ID == 0 || ws.Meta.Elements["db"].ID == 0 {
		t.Fatalf("expected element IDs recorded in metadata, got %#v", ws.Meta)
	}
}

func TestImportCmd_IdempotentRerun(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, basicImport)

	if _, _, err := cmd.RunCmd(t, dir, "import", file); err != nil {
		t.Fatalf("first import: %v", err)
	}
	first, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load after first import: %v", err)
	}
	firstIDs := map[string]int32{
		"api": int32(first.Meta.Elements["api"].ID),
		"db":  int32(first.Meta.Elements["db"].ID),
	}

	stdout, _, err := cmd.RunCmd(t, dir, "import", file)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if !strings.Contains(stdout, "0 new") {
		t.Fatalf("expected re-run to create nothing new, got:\n%s", stdout)
	}

	second, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load after second import: %v", err)
	}
	if len(second.Elements) != 2 {
		t.Fatalf("elements after re-run = %d, want 2 (duplicates created)", len(second.Elements))
	}
	if len(second.Connectors) != 1 {
		t.Fatalf("connectors after re-run = %d, want 1 (duplicates created)", len(second.Connectors))
	}
	if got := int32(second.Meta.Elements["api"].ID); got != firstIDs["api"] {
		t.Fatalf("api id changed on re-run: %d -> %d", firstIDs["api"], got)
	}
	if got := int32(second.Meta.Elements["db"].ID); got != firstIDs["db"] {
		t.Fatalf("db id changed on re-run: %d -> %d", firstIDs["db"], got)
	}
}

func TestImportCmd_DryRunDoesNotMutateWorkspace(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, basicImport)

	elementsPath := filepath.Join(dir, ".tld", "elements.yaml")
	before, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatalf("read elements before: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "import", file, "--dry-run")
	if err != nil {
		t.Fatalf("import --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Fatalf("expected dry-run output, got:\n%s", stdout)
	}

	after, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatalf("read elements after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("elements.yaml changed during dry-run")
	}
}

func TestImportCmd_ConnectorsOnlyUsesWorkspaceElements(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service"); err != nil {
		t.Fatalf("add api: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "add", "Database", "--ref", "db", "--kind", "database"); err != nil {
		t.Fatalf("add db: %v", err)
	}

	file := writeImportFile(t, `
connectors:
  - view: root
    source: api
    target: db
    label: reads
`)
	if _, _, err := cmd.RunCmd(t, dir, "import", file); err != nil {
		t.Fatalf("connector-only import: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if len(ws.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1", len(ws.Connectors))
	}
}

func TestImportCmd_MissingReferenceExplainsReason(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, `
connectors:
  - view: root
    source: api
    target: ghost
`)

	_, _, err := cmd.RunCmd(t, dir, "import", file)
	if err == nil {
		t.Fatal("expected error for missing reference")
	}
	msg := err.Error()
	if !strings.Contains(msg, "neither defined in the file nor present in the workspace") {
		t.Fatalf("error should explain the missing ref, got: %s", msg)
	}
	if !strings.Contains(msg, "ghost") {
		t.Fatalf("error should name the missing ref, got: %s", msg)
	}
}

func TestImportCmd_UnknownPlacementParentExplainsReason(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, `
elements:
  api:
    name: API
    kind: service
    placements:
      - parent: missing-parent
`)

	_, _, err := cmd.RunCmd(t, dir, "import", file)
	if err == nil {
		t.Fatal("expected error for unknown placement parent")
	}
	if !strings.Contains(err.Error(), "missing-parent") {
		t.Fatalf("error should name the missing parent, got: %s", err.Error())
	}
}

func TestImportCmd_DuplicateConnectorFails(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, `
elements:
  api:
    name: API
    kind: service
  db:
    name: Database
    kind: database
connectors:
  - view: root
    source: api
    target: db
    label: reads
  - view: root
    source: api
    target: db
    label: reads
`)

	_, _, err := cmd.RunCmd(t, dir, "import", file)
	if err == nil {
		t.Fatal("expected duplicate connector error")
	}
	if !strings.Contains(err.Error(), "defined more than once") {
		t.Fatalf("error should explain the duplicate, got: %s", err.Error())
	}
}

func TestImportCmd_EmptyFileFails(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, "   \n")

	_, _, err := cmd.RunCmd(t, dir, "import", file)
	if err == nil {
		t.Fatal("expected empty file error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error should explain the file is empty, got: %s", err.Error())
	}
}

func TestImportCmd_MalformedYamlShowsSchemaHint(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, "this: [is: not: valid yaml\n")

	_, _, err := cmd.RunCmd(t, dir, "import", file)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "elements.schema.json") || !strings.Contains(err.Error(), "connectors.schema.json") {
		t.Fatalf("error should link the schemas, got: %s", err.Error())
	}
}

func TestImportCmd_InvalidRefFails(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, `
elements:
  "Bad Ref":
    name: Bad
    kind: service
`)

	_, _, err := cmd.RunCmd(t, dir, "import", file)
	if err == nil {
		t.Fatal("expected invalid ref error")
	}
	if !strings.Contains(err.Error(), "invalid ref") {
		t.Fatalf("error should explain the invalid ref, got: %s", err.Error())
	}
}

func TestImportCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, basicImport)

	stdout, _, err := cmd.RunCmd(t, dir, "import", file, "--format", "json")
	if err != nil {
		t.Fatalf("import --format json: %v", err)
	}
	if !strings.Contains(stdout, `"command": "import"`) || !strings.Contains(stdout, `"elements_created": 2`) {
		t.Fatalf("unexpected JSON output:\n%s", stdout)
	}
}
