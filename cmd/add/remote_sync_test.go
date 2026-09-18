package add_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// TestSyncCommands_RemoteRoundTrip exercises the synchronous server flow over
// ConnectRPC: every command applies immediately and refreshes the YAML cache.
func TestSyncCommands_RemoteRoundTrip(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	if _, _, err := cmd.RunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace"); err != nil {
		t.Fatalf("remote add platform: %v", err)
	}
	stdout, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	if err != nil {
		t.Fatalf("remote add api: %v", err)
	}
	if !strings.Contains(stdout, "add: api (id=") {
		t.Fatalf("expected immediate server feedback with id, got:\n%s", stdout)
	}
	if _, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "platform", "--label", "runs-on"); err != nil {
		t.Fatalf("remote connect: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "Handles traffic"); err != nil {
		t.Fatalf("remote update: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Elements["api"] == nil || ws.Elements["api"].Description != "Handles traffic" {
		t.Fatalf("YAML cache not refreshed: %+v", ws.Elements["api"])
	}
	if meta := ws.Meta.Elements["api"]; meta == nil || meta.ID == 0 {
		t.Fatalf("element meta missing server id: %+v", meta)
	}
	if len(ws.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1", len(ws.Connectors))
	}

	if _, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "platform"); err != nil {
		t.Fatalf("remote remove connector: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "remove", "element", "api"); err != nil {
		t.Fatalf("remote remove element: %v", err)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if _, ok := ws.Elements["api"]; ok {
		t.Fatalf("api still present after remove: %+v", ws.Elements)
	}
	if len(ws.Connectors) != 0 {
		t.Fatalf("connectors = %d, want 0", len(ws.Connectors))
	}
}
