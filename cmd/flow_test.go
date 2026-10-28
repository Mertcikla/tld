package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func flowJSON(t *testing.T, dir string, args ...string) *diagv1.Flow {
	t.Helper()
	args = append(args, "--format", "json")
	out, _ := MustRunCmd(t, dir, args...)
	var envelope struct {
		Extra struct {
			Flow json.RawMessage `json:"flow"`
		} `json:"extra"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	f := &diagv1.Flow{}
	if err := protojson.Unmarshal(envelope.Extra.Flow, f); err != nil {
		t.Fatalf("invalid flow: %v\n%s", err, out)
	}
	return f
}
func setupFlowDiagram(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_DATA_DIR", t.TempDir())
	MustRunCmd(t, dir, "add", "API", "--kind", "service")
	MustRunCmd(t, dir, "add", "Database", "--kind", "database")
	MustRunCmd(t, dir, "connect", "--from", "api", "--to", "database", "--label", "writes")
	return dir
}
func TestFlowAuthoringAndWorkspaceIndependence(t *testing.T) {
	dir := setupFlowDiagram(t)
	for _, name := range []string{".tld.yaml", "elements.yaml", "connectors.yaml", ".tld.lock"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("invalid: ["), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f := flowJSON(t, dir, "flow", "create", "Checkout", "--view", "root", "--description", "Journey")
	if f.Id == "" || f.Revision != 1 {
		t.Fatalf("unexpected created flow: %v", f)
	}
	flowJSON(t, dir, "flow", "step", "add", f.Id, "--id", "request", "--title", "Receive", "--element", "api")
	flowJSON(t, dir, "flow", "step", "add", f.Id, "--id", "persist", "--connector", "root/api~database/writes")
	branch := flowJSON(t, dir, "flow", "branch", "create", f.Id, "persist", "--mode", "alternate", "--path", "Approved", "--path", "Declined")
	paths := branch.Content.Steps[1].GetBranches().Paths
	flowJSON(t, dir, "flow", "step", "add", f.Id, "--path", paths[0].Id, "--id", "nested", "--title", "Nested")
	moved := flowJSON(t, dir, "flow", "step", "move", f.Id, "request", "--path", paths[1].Id)
	if len(moved.Content.Steps) != 1 || len(moved.Content.Steps[0].GetBranches().Paths[1].Steps) != 1 {
		t.Fatalf("move lost steps: %v", moved)
	}
	before := flowJSON(t, dir, "flow", "inspect", f.Id)
	preview := flowJSON(t, dir, "flow", "step", "delete", f.Id, "persist", "--dry-run")
	after := flowJSON(t, dir, "flow", "inspect", f.Id)
	if len(preview.Content.Steps) != 0 || before.Revision != after.Revision || len(after.Content.Steps) != 1 {
		t.Fatal("dry run changed target or preview")
	}
	for _, args := range [][]string{
		{"flow", "step", "move", f.Id, "persist", "--path", paths[0].Id},
		{"flow", "step", "add", f.Id, "--id", "nested"},
		{"flow", "step", "update", f.Id, "persist", "--go-to", f.Id},
		{"flow", "branch", "path", "delete", f.Id, paths[0].Id},
		{"flow", "update", f.Id, "--name", "No", "--expected-revision", "1"},
		{"flow", "step", "update", f.Id, "persist", "--element", "api", "--clear-elements"},
	} {
		if _, _, err := RunCmd(t, dir, args...); err == nil {
			t.Fatalf("expected rejection: %v", args)
		}
	}
	f = flowJSON(t, dir, "flow", "branch", "path", "add", f.Id, "persist", "Retry", "--id", "retry")
	f = flowJSON(t, dir, "flow", "branch", "path", "move", f.Id, "retry", "--before", paths[0].Id)
	if f.Content.Steps[0].GetBranches().Paths[0].Id != "retry" {
		t.Fatal("path move failed")
	}
	flowJSON(t, dir, "flow", "branch", "path", "update", f.Id, "retry", "--name", "Try again")
	flowJSON(t, dir, "flow", "branch", "update", f.Id, "persist", "--mode", "parallel")
	flowJSON(t, dir, "flow", "branch", "path", "delete", f.Id, "retry")
	flowJSON(t, dir, "flow", "step", "update", f.Id, "request", "--clear-elements", "--title", "Changed")
	f = flowJSON(t, dir, "flow", "update", f.Id, "--name", "New Checkout", "--description", "")
	if f.Content.Description != "" {
		t.Fatal("description did not clear")
	}
	flowJSON(t, dir, "flow", "create", "New Checkout", "--view", "root")
	if _, _, err := RunCmd(t, dir, "flow", "inspect", "New Checkout"); err == nil {
		t.Fatal("ambiguous flow name accepted")
	}
	for _, name := range []string{".tld.yaml", "elements.yaml", "connectors.yaml", ".tld.lock"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != "invalid: [" {
			t.Fatal("workspace file changed")
		}
	}
	flowJSON(t, dir, "flow", "delete", f.Id)
	if _, _, err := RunCmd(t, dir, "flow", "inspect", f.Id); err == nil {
		t.Fatal("deleted flow still exists")
	}
}
func TestFlowCompleteImportExport(t *testing.T) {
	dir := setupFlowDiagram(t)
	fixture := filepath.Join("..", "internal", "flowdoc", "fixtures", "checkout.flow.yaml")
	// A preview must not create the owner, resources or flow.
	flowJSON(t, dir, "flow", "import", fixture, "--new-view", "Preview", "--dry-run")
	out, _ := MustRunCmd(t, dir, "flow", "list", "--format", "json")
	if strings.Contains(out, "Checkout") {
		t.Fatal("preview created flow")
	}
	f := flowJSON(t, dir, "flow", "import", fixture, "--new-view", "Checkout demo")
	exported := filepath.Join(t.TempDir(), "roundtrip.flow.yaml")
	MustRunCmd(t, dir, "flow", "export", f.Id, "--complete", "-o", exported)
	data, err := os.ReadFile(exported)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"format: tld-flow", "branches:", "is_primary_icon: true", "storage", "position_x: 120"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("export missing %q:\n%s", expected, data)
		}
	}
	second := flowJSON(t, dir, "flow", "import", exported, "--new-view", "Another demo")
	if second.Id == f.Id || second.ViewId == f.ViewId {
		t.Fatal("complete import reused resources")
	}
	replaced := flowJSON(t, dir, "flow", "import", exported, "--view", stringID(f.ViewId), "--replace", f.Id)
	if replaced.Id != f.Id || replaced.Revision != f.Revision+1 {
		t.Fatal("replacement did not retain identity")
	}
	out, _, err = RunCmdWithStdin(t, dir, strings.NewReader("format: tld-flow\nversion: 1\nname: Empty\nsteps: []\n"), "flow", "import", "-", "--view", "root")
	if err != nil {
		t.Fatalf("stdin import failed: %v %s", err, out)
	}
}
func stringID(id int32) string { return strconv.Itoa(int(id)) }
func TestFlowSchemaAndErrorOutput(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	out, _ := MustRunCmd(t, dir, "flow", "import", "--schema")
	if !strings.Contains(out, "Portable tlDiagram flow v1") {
		t.Fatal("schema missing")
	}
	out, _, err := RunCmd(t, dir, "flow", "update", "missing", "--name", "No", "--format", "json")
	if err == nil || !json.Valid([]byte(out)) || !strings.Contains(out, "not_found") {
		t.Fatalf("expected structured error: %v %s", err, out)
	}
}

func TestFlowLinksAndSoftReferences(t *testing.T) {
	dir := setupFlowDiagram(t)
	source := flowJSON(t, dir, "flow", "create", "Source", "--view", "root")
	target := flowJSON(t, dir, "flow", "create", "Target", "--view", "root")
	flowJSON(t, dir, "flow", "step", "add", source.Id, "--id", "link", "--go-to", target.Id, "--element", "api")
	deleted, _ := MustRunCmd(t, dir, "flow", "delete", target.Id, "--format", "json")
	if !strings.Contains(deleted, "will become unavailable") {
		t.Fatal("missing incoming-link warning")
	}
	MustRunCmd(t, dir, "remove", "connector", "root/api~database/writes")
	MustRunCmd(t, dir, "remove", "element", "api")
	out, _ := MustRunCmd(t, dir, "flow", "update", source.Id, "--description", "Keep references", "--format", "json")
	if !strings.Contains(out, "is unavailable") {
		t.Fatal("missing stale reference warning")
	}
	preserved := flowJSON(t, dir, "flow", "inspect", source.Id)
	if preserved.Content.Steps[0].GetGoToFlowId() != target.Id || len(preserved.Content.Steps[0].ElementIds) != 1 {
		t.Fatal("unrelated update lost soft references")
	}
	if _, _, err := RunCmd(t, dir, "flow", "export", source.Id); err == nil {
		t.Fatal("export accepted unavailable references")
	}
	cleared := flowJSON(t, dir, "flow", "step", "update", source.Id, "link", "--clear-go-to", "--clear-elements")
	if cleared.Content.Steps[0].Control != nil || len(cleared.Content.Steps[0].ElementIds) != 0 {
		t.Fatal("clear flags failed")
	}
	flowJSON(t, dir, "flow", "branch", "create", source.Id, "link", "--mode", "parallel", "--path", "A", "--path", "B")
	deletedBranch := flowJSON(t, dir, "flow", "branch", "delete", source.Id, "link")
	if len(deletedBranch.Content.Steps) != 1 || deletedBranch.Content.Steps[0].Control != nil {
		t.Fatal("branch delete did not preserve owning step")
	}
}
