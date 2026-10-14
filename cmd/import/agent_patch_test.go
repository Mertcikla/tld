package importcmd_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

func TestImportPatchPreservesTargetAndCache(t *testing.T) {
	for _, target := range []string{"local", "remote"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("TLD_CONFIG_DIR", t.TempDir())
			t.Setenv("TLD_DATA_DIR", t.TempDir())
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)
			if target == "remote" {
				cmd.WriteConfig(t, dir, cmd.NewLocalAPIServer(t, t.TempDir()), "test-key")
				cfg, err := workspace.LoadGlobalConfig()
				if err != nil {
					t.Fatal(err)
				}
				bootstrap, err := exec.NewRunner(*cfg, "remote", "", false)
				if err != nil {
					t.Fatal(err)
				}
				_, err = bootstrap.CreateView(context.Background(), nil, "Workspace", nil)
				_ = bootstrap.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--tags", "backend", "--position-x", "200", "--position-y", "100")
			cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db")
			cmd.MustRunCmd(t, dir, "add", "Unrelated", "--ref", "unrelated")
			cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--view", "root", "--label", "reads", "--description", "Source evidence", "--direction", "both", "--style", "straight", "--tags", "storage")
			ws, err := workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			runner, err := exec.NewRunner(ws.Config, target, "", false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = runner.Close() }()
			id := int32(ws.Meta.Elements["api"].ID)
			views, err := runner.ListViews(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var rootID int32
			for _, view := range views {
				if view.OwnerElementId == nil {
					rootID = view.GetId()
					break
				}
			}
			// Simulate a UI edit after the CLI cache was last refreshed.
			if err := runner.UpdatePlacement(context.Background(), rootID, id, 450, 275); err != nil {
				t.Fatal(err)
			}
			file := writeImportFile(t, "elements:\n  api:\n    name: API\n    description: Patched\n    placements:\n      - parent: root\nconnectors:\n  - view: root\n    source: api\n    target: db\n    label: reads\n")
			cmd.MustRunCmd(t, dir, "import", file)
			assertState := func(x, y float64) {
				t.Helper()
				cached, err := workspace.Load(dir)
				if err != nil {
					t.Fatal(err)
				}
				if cached.Elements["unrelated"] == nil || !slices.Equal(cached.Elements["api"].Tags, []string{"backend"}) {
					t.Fatalf("lost unrelated resource or omitted tags: %+v", cached.Elements)
				}
				placement := cached.Elements["api"].Placements[0]
				if placement.PositionX != x || placement.PositionY != y {
					t.Fatalf("cache coordinates = (%v,%v), want (%v,%v)", placement.PositionX, placement.PositionY, x, y)
				}
				connector := cached.Connectors["root/api~db/reads"]
				if connector.Direction != "both" || connector.Style != "straight" || connector.Description != "Source evidence" || !slices.Equal(connector.Tags, []string{"storage"}) {
					t.Fatalf("lost omitted connector metadata: %+v", connector)
				}
				exported, err := runner.ExportWorkspace(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range exported.GetPlacements() {
					if p.GetElementId() == id && p.GetViewId() == rootID && (p.GetPositionX() != x || p.GetPositionY() != y) {
						t.Fatalf("target coordinates differ: %v", p)
					}
				}
			}
			assertState(450, 275)
			// An explicit zero is a coordinate update; the other axis is preserved.
			cmd.MustRunCmd(t, dir, "import", writeImportFile(t, "elements:\n  api:\n    name: API\n    placements:\n      - parent: root\n        position_x: 0\n"))
			assertState(0, 275)
			// Connector-only edits must not replay stale endpoint metadata/layout.
			description := "Changed on target"
			if _, err := runner.UpdateElement(context.Background(), id, api.ElementInput{Name: "API", Description: &description, Tags: []string{"backend"}}); err != nil {
				t.Fatal(err)
			}
			if err := runner.UpdatePlacement(context.Background(), rootID, id, 600, 300); err != nil {
				t.Fatal(err)
			}
			cmd.MustRunCmd(t, dir, "import", writeImportFile(t, "connectors:\n  - view: root\n    source: api\n    target: db\n    label: reads\n"))
			assertState(600, 300)
			cached, err := workspace.Load(dir)
			if err != nil || cached.Elements["api"].Description != description {
				t.Fatalf("replayed stale endpoint description: %v, %v", cached, err)
			}
		})
	}
}

func TestImportUnsupportedFieldsAbortBatch(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	for _, body := range []string{
		"elements:\n  api:\n    name: API\n    symbol: Handler\n",
		"elements:\n  api:\n    name: API\n    <<: {symbol: Handler}\n",
		"elements:\n  <<: {api: {name: API, symbol: Handler}}\n",
		"elements:\n  api:\n    name: API\n    view_name: Details\n",
		"elements:\n  api:\n    name: API\n  db:\n    name: DB\nconnectors:\n  - source: api\n    target: db\n    tags: [new-tag]\n",
	} {
		out, _, err := cmd.RunCmd(t, dir, "import", writeImportFile(t, body), "--format", "json")
		var result cmdutil.JSONOutput
		if err == nil || json.Unmarshal([]byte(out), &result) != nil || result.Error == nil || result.Error.Code != "unsupported_field" {
			t.Fatalf("unsupported field was not rejected: %v\n%s", err, out)
		}
	}
	out, _ := cmd.MustRunCmd(t, dir, "list", "elements", "--target", "local", "--format", "json")
	var result cmdutil.JSONOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Items) != 0 {
		t.Fatalf("rejected batch wrote target resources: %v\n%s", err, out)
	}
}

func TestImportResultsExposeSelectorsAndOfflineSchema(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := writeImportFile(t, basicImport)
	for _, preview := range []bool{true, false} {
		args := []string{"import", file, "--format", "json"}
		if preview {
			args = append(args, "--dry-run")
		}
		out, _ := cmd.MustRunCmd(t, dir, args...)
		var result cmdutil.JSONOutput
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "ok" || result.DryRun != preview || len(result.Items) != 3 {
			t.Fatalf("incomplete result: %s", out)
		}
		for _, item := range result.Items {
			if item.Ref == "" || (!preview && item.ID == 0) {
				t.Fatalf("unusable selector: %+v", item)
			}
		}
	}
	out, _ := cmd.MustRunCmd(t, dir, "import", "--schema")
	var schema map[string]any
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Fatal(err)
	}
	definitions := schema["$defs"].(map[string]any)
	connectorFields := definitions["connector"].(map[string]any)["properties"].(map[string]any)
	if _, advertised := connectorFields["tags"]; advertised {
		t.Fatal("schema advertises unsupported tags")
	}
	// A malformed file's hint points at an executable discovery command.
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("elements: ["), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := cmd.RunCmd(t, dir, "import", bad)
	if err == nil || !strings.Contains(err.Error(), "tld import --schema") {
		t.Fatalf("missing schema recovery: %v", err)
	}
}
