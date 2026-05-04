package update_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestUpdateElementCmdUpdatesScalarField(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "Handles traffic")
	if err != nil {
		t.Fatalf("update element: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Updated element") {
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
	if !strings.Contains(stdout, "Updated connector") {
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

func TestUpdateCmdShowsHelpWithNoSubcommand(t *testing.T) {
	stdout, stderr, err := cmd.RunCmd(t, t.TempDir(), "update")
	if err != nil {
		t.Fatalf("update help: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Update a resource field") {
		t.Fatalf("stdout = %q, want update help", stdout)
	}
}
