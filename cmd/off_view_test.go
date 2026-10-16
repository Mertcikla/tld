package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestOffViewRelationshipsPreservePlacementsAndReportVisibility(t *testing.T) {
	for _, operation := range []string{"connect", "import", "update"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)
			cmd.MustRunCmd(t, dir, "add", "Runtime", "--ref", "runtime")
			cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "runtime", "--position-x", "101", "--position-y", "203")
			cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db", "--position-x", "401", "--position-y", "203")
			before, err := workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "connect":
				out, _ := cmd.MustRunCmd(t, dir, "connect", "--view", "runtime", "--from", "api", "--to", "db", "--label", "reads", "--dry-run", "--format", "json")
				if !strings.Contains(out, `"off-view"`) {
					t.Fatalf("preview missing visibility: %s", out)
				}
				out, _ = cmd.MustRunCmd(t, dir, "connect", "--view", "runtime", "--from", "api", "--to", "db", "--label", "reads")
				if !strings.Contains(out, "off-view (db in root)") {
					t.Fatalf("creation missing visibility: %s", out)
				}
			case "import":
				file := filepath.Join(t.TempDir(), "diagram.yaml")
				if err := os.WriteFile(file, []byte("connectors:\n  - view: runtime\n    source: api\n    target: db\n    label: reads\n"), 0600); err != nil {
					t.Fatal(err)
				}
				cmd.MustRunCmd(t, dir, "import", file, "--dry-run")
				cmd.MustRunCmd(t, dir, "import", file)
			case "update":
				cmd.MustRunCmd(t, dir, "connect", "--view", "root", "--from", "api", "--to", "db", "--label", "reads", "--no-reposition")
				cmd.MustRunCmd(t, dir, "update", "connector", "root/api~db/reads", "view", "runtime")
			}
			after, err := workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			for ref, old := range before.Elements {
				if !reflect.DeepEqual(old.Placements, after.Elements[ref].Placements) {
					t.Fatalf("%s placements changed: %+v -> %+v", ref, old.Placements, after.Elements[ref].Placements)
				}
			}
			key := "runtime/api~db/reads"
			if after.Connectors[key] == nil {
				t.Fatal("off-view relationship was lost")
			}
			cmd.MustRunCmd(t, dir, "validate", "ARC008", "--strict", "--source", "live")
			for _, source := range []string{"cache", "live"} {
				viewOut, _ := cmd.MustRunCmd(t, dir, "inspect", "runtime", "--type", "view", "--source", source, "--format", "json")
				var viewReport struct {
					Diagnostics workspace.ViewDiagnostics `json:"view_diagnostics"`
				}
				if err := json.Unmarshal([]byte(viewOut), &viewReport); err != nil {
					t.Fatal(err)
				}
				if len(viewReport.Diagnostics.OffViewConnectors) != 1 {
					t.Fatalf("view inspect: %s", viewOut)
				}
				out, _ := cmd.MustRunCmd(t, dir, "inspect", key, "--type", "connector", "--source", source, "--format", "json")
				var report struct {
					Connector struct {
						Visibility workspace.ConnectorVisibility `json:"visibility"`
					} `json:"connector"`
				}
				if err := json.Unmarshal([]byte(out), &report); err != nil {
					t.Fatal(err)
				}
				if report.Connector.Visibility.Mode != "off-view" || strings.Join(report.Connector.Visibility.EndpointViews["db"], ",") != "root" {
					t.Fatalf("inspect: %s", out)
				}
				out, _ = cmd.MustRunCmd(t, dir, "list", "connectors", "--view", "runtime", "--source", source)
				if !strings.Contains(out, "off-view (db in root)") {
					t.Fatalf("list: %s", out)
				}
				out, _ = cmd.MustRunCmd(t, dir, "list", "connectors", "--view", "runtime", "--source", source, "--format", "json")
				if !strings.Contains(out, `"off-view"`) {
					t.Fatalf("JSON list: %s", out)
				}
				out, stderr := cmd.MustRunCmd(t, dir, "render", "runtime", "--source", source)
				if strings.Contains(out, "reads") || !strings.Contains(stderr, "off-view") || strings.Contains(stderr, "Warning") || strings.Contains(stderr, "Place missing") {
					t.Fatalf("render: %s\n%s", out, stderr)
				}
				out, _ = cmd.MustRunCmd(t, dir, "render", "runtime", "--source", source, "--format", "json")
				var rendered struct {
					Summary map[string]int `json:"summary"`
					Extra   struct {
						Visibility map[string]workspace.ConnectorVisibility `json:"connector_visibility"`
					} `json:"extra"`
				}
				if err := json.Unmarshal([]byte(out), &rendered); err != nil {
					t.Fatal(err)
				}
				if rendered.Summary["off_view_connectors"] != 1 || rendered.Summary["unresolved_connectors"] != 0 || rendered.Summary["rendered_connectors"] != 0 || rendered.Extra.Visibility[key].Mode != "off-view" {
					t.Fatalf("JSON render: %s", out)
				}
			}
			// The view owner is a valid external endpoint, not its own child placement.
			cmd.MustRunCmd(t, dir, "connect", "--view", "runtime", "--from", "api", "--to", "runtime", "--label", "reports")
			cmd.MustRunCmd(t, dir, "validate", "ARC008", "--strict", "--source", "live")
		})
	}
}
