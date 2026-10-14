package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func diagramFile(t *testing.T, body string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "view.yaml")
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}
func loadPlacementWorkspace(t *testing.T, dir string) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}
func seedPlacementWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Runtime", "--ref", "runtime")
	cmd.MustRunCmd(t, dir, "view", "create", "runtime")
	cmd.MustRunCmd(t, dir, "add", "Other", "--ref", "other")
	cmd.MustRunCmd(t, dir, "view", "create", "other")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--position-x", "101", "--position-y", "203")
	cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db", "--position-x", "401", "--position-y", "203")
	return dir
}
func TestConnectorPlacementConnectAndImportMakeEndpointsVisible(t *testing.T) {
	for _, operation := range []string{"connect", "import"} {
		t.Run(operation, func(t *testing.T) {
			dir := seedPlacementWorkspace(t)
			before := loadPlacementWorkspace(t, dir)
			if operation == "connect" {
				cmd.MustRunCmd(t, dir, "connect", "--view", "runtime", "--from", "api", "--to", "db", "--label", "reads")
			} else {
				file := diagramFile(t, "connectors:\n  - view: runtime\n    source: api\n    target: db\n    label: reads\n")
				cmd.MustRunCmd(t, dir, "import", file)
			}
			ws := loadPlacementWorkspace(t, dir)
			for _, ref := range []string{"api", "db"} {
				if !ws.ElementPlacedInView(ref, "runtime") || !ws.ElementPlacedInView(ref, "root") {
					t.Fatalf("endpoint placements missing: %+v", ws.Elements[ref])
				}
				if ws.Meta.Elements[ref].ID != before.Meta.Elements[ref].ID {
					t.Fatal("duplicated reusable element")
				}
				found := false
				for _, p := range ws.Elements[ref].Placements {
					if p.ParentRef == "root" {
						for _, old := range before.Elements[ref].Placements {
							if old.ParentRef == "root" && p.PositionX == old.PositionX && p.PositionY == old.PositionY {
								found = true
							}
						}
					}
				}
				if !found {
					t.Fatal("existing coordinates changed")
				}
			}
			out, _ := cmd.MustRunCmd(t, dir, "render", "runtime", "--source", "live", "--format", "json")
			var result struct {
				Summary map[string]int `json:"summary"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result.Summary["rendered_connectors"] != 1 || result.Summary["omitted_connectors"] != 0 {
				t.Fatalf("relationship invisible: %s", out)
			}
		})
	}
}

func TestConnectorPlacementConnectDryRunReportsPlacementsWithoutWriting(t *testing.T) {
	dir := seedPlacementWorkspace(t)
	out, _ := cmd.MustRunCmd(t, dir, "connect", "--view", "runtime", "--from", "api", "--to", "db", "--dry-run", "--format", "json")
	var result struct {
		Extra struct {
			Placements []string `json:"placements_added"`
		} `json:"extra"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Extra.Placements, ",") != "api,db" {
		t.Fatalf("missing placement preview: %s", out)
	}
	ws := loadPlacementWorkspace(t, dir)
	if len(ws.Connectors) != 0 || ws.ElementPlacedInView("api", "runtime") || ws.ElementPlacedInView("db", "runtime") {
		t.Fatal("dry-run wrote diagram contents")
	}
}
func TestConnectorPlacementRetainsExistingSharingBehavior(t *testing.T) {
	dir := seedPlacementWorkspace(t)
	cmd.MustRunCmd(t, dir, "connect", "--view", "root", "--from", "api", "--to", "db", "--label", "reads")
	for _, ref := range []string{"api", "db"} {
		cmd.MustRunCmd(t, dir, "view", "place", "runtime", ref)
	}
	ws := loadPlacementWorkspace(t, dir)
	if ws.Connectors["root/api~db/reads"] == nil || ws.Connectors["runtime/api~db/reads"] == nil {
		t.Fatalf("sharing lost existing relationships: %v", ws.Connectors)
	}
	for range 2 {
		cmd.MustRunCmd(t, dir, "connect", "--view", "runtime", "--from", "api", "--to", "db", "--label", "reads")
	}
	ws = loadPlacementWorkspace(t, dir)
	if len(ws.Connectors) != 2 {
		t.Fatalf("explicit relationship did not converge: %v", ws.Connectors)
	}
	cmd.MustRunCmd(t, dir, "update", "connector", "runtime/api~db/reads", "view", "other")
	ws = loadPlacementWorkspace(t, dir)
	if !ws.ElementPlacedInView("api", "other") || !ws.ElementPlacedInView("db", "other") {
		t.Fatal("moving a connector hid its endpoints")
	}
	cmd.MustRunCmd(t, dir, "validate", "ARC008", "--strict", "--source", "live")
}
func TestConnectorPlacementOwnerErrorsPrecedeAllWrites(t *testing.T) {
	dir := seedPlacementWorkspace(t)
	before := loadPlacementWorkspace(t, dir)
	commands := [][]string{
		{"connect", "--view", "runtime", "--from", "runtime", "--to", "api"},
		{"import", diagramFile(t, "elements:\n  new:\n    name: Must not create\nconnectors:\n  - view: runtime\n    source: runtime\n    target: new\n")},
	}
	for _, args := range commands {
		if _, _, err := cmd.RunCmd(t, dir, args...); err == nil || !strings.Contains(err.Error(), "containing view") {
			t.Fatalf("missing actionable owner error: %v", err)
		}
	}
	after := loadPlacementWorkspace(t, dir)
	if len(after.Elements) != len(before.Elements) || len(after.Connectors) != 0 {
		t.Fatal("refused request wrote resources")
	}
}
