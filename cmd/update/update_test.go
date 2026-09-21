package update_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestUpdateElementCmdUpdatesScalarField(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "Handles traffic")
	if err != nil {
		t.Fatalf("update element: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "updated \"api\": description=\"Handles traffic\"") {
		t.Fatalf("stdout = %q, want update confirmation", stdout)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ws.Elements["api"].Description; got != "Handles traffic" {
		t.Fatalf("description = %q, want Handles traffic", got)
	}
}

func TestUpdateConnectorCmdUpdatesDirection(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "update", "connector", "platform:api:db:reads", "direction", "bidirectional")
	if err != nil {
		t.Fatalf("update connector: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "updated \"platform:api:db:reads\": direction=\"bidirectional\"") {
		t.Fatalf("stdout = %q, want update confirmation", stdout)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ws.Connectors["platform:api:db:reads"].Direction; got != "bidirectional" {
		t.Fatalf("direction = %q, want bidirectional", got)
	}
}

func TestUpdateConnectorCmdRenamesKeyOnSourceChange(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Cache", "--ref", "cache", "--parent", "platform", "--kind", "database")

	stdout, stderr, err := cmd.RunCmd(t, dir, "update", "connector", "platform:api:db:reads", "source", "cache")
	if err != nil {
		t.Fatalf("update connector source: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "updated \"platform:api:db:reads\": source=\"cache\"") {
		t.Fatalf("stdout = %q, want update confirmation", stdout)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	renamed := ws.Connectors["platform:cache:db:reads"]
	if renamed == nil || renamed.Source != "cache" {
		t.Fatalf("renamed connector missing: %+v", ws.Connectors)
	}
	if _, ok := ws.Connectors["platform:api:db:reads"]; ok {
		t.Fatalf("old connector key still present: %+v", ws.Connectors)
	}
}

func TestUpdateCmdShowsHelpWithNoSubcommand(t *testing.T) {
	stdout, stderr, err := cmd.RunCmd(t, t.TempDir(), "update")
	if err != nil {
		t.Fatalf("update help: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Update a resource field") {
		t.Fatalf("stdout = %q, want update help", stdout)
	}
}
