package update_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestRefRenamePreservesCacheSyncState(t *testing.T) {
	for _, operation := range []string{"update", "rename"} {
		for _, dirty := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dirty=%t", operation, dirty), func(t *testing.T) {
				dir := t.TempDir()
				cmd.MustInitWorkspace(t, dir)
				cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api")
				wsDir := workspace.ResolveDir(dir)
				if dirty {
					if err := workspace.UpdateElementField(wsDir, "api", "description", "Pending edit"); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{"update", "element", "api", "ref", "backend"}
				if operation == "rename" {
					args = []string{"rename", "--from", "api", "--to", "backend"}
				}
				cmd.MustRunCmd(t, dir, args...)
				if err := cmdutil.EnsureCacheInSync(wsDir); (err != nil) != dirty {
					t.Fatalf("cache sync error = %v, want dirty=%t", err, dirty)
				}
				_, _, err := cmd.RunCmd(t, dir, "add", "DB", "--ref", "db")
				if (err != nil) != dirty {
					t.Fatalf("write after rename: %v, want dirty=%t", err, dirty)
				}
				ws, err := workspace.Load(dir)
				if err != nil {
					t.Fatal(err)
				}
				if ws.Elements["api"] != nil || ws.Elements["backend"] == nil || ws.Meta == nil || ws.Meta.Elements["backend"] == nil || ws.Meta.Elements["backend"].ID == 0 {
					t.Fatalf("renamed element or metadata lost: %+v", ws)
				}
				if dirty && ws.Elements["backend"].Description != "Pending edit" {
					t.Fatal("rename lost unapplied edits")
				}
			})
		}
	}
}

func TestUpdateElementRefCascadesReferencesAndPreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "connect", "--from", "platform", "--to", "api", "--view", "platform", "--label", "contains")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "platform", "--view", "platform", "--label", "reports")
	before, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A ref update must only change the local alias, even with a remote target
	// configured. Opening a local database here would create a new tld.db.
	dataDir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", dataDir)
	t.Setenv("TLD_APPLY_TARGET", "remote")
	t.Setenv("TLD_SERVER_URL", "http://127.0.0.1:1")
	stdout, _ := cmd.MustRunCmd(t, dir, "update", "element", "platform", "ref", "system", "--format", "json", "--compact")
	var payload cmdutil.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if payload.Command != "update element" || payload.Status != "ok" || len(payload.Items) != 1 || payload.Items[0].Ref != "system" || payload.Items[0].Action != "update" {
		t.Fatalf("unexpected mutation output: %+v", payload)
	}
	after, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.Elements["platform"] != nil || after.Elements["system"] == nil || after.Elements["system"].Name != before.Elements["platform"].Name {
		t.Fatalf("ref rename lost the element or changed its name: %+v", after.Elements)
	}
	for _, ref := range []string{"api", "db"} {
		if after.Elements[ref].Placements[0].ParentRef != "system" {
			t.Fatalf("placement for %s still uses the old parent", ref)
		}
	}
	if after.Meta.Elements["platform"] != nil || after.Meta.Views["platform"] != nil || after.Meta.Elements["system"] == nil || after.Meta.Views["system"] == nil {
		t.Fatalf("metadata refs did not cascade: %+v", after.Meta)
	}
	if after.Meta.Elements["system"].ID != before.Meta.Elements["platform"].ID || after.Meta.Views["system"].ID != before.Meta.Views["platform"].ID {
		t.Fatal("element or view database ID changed")
	}
	for oldKey, connector := range before.Connectors {
		want := *connector
		want.View = "system"
		if want.Source == "platform" {
			want.Source = "system"
		}
		if want.Target == "platform" {
			want.Target = "system"
		}
		newKey := workspace.ConnectorKey(&want)
		got := after.Connectors[newKey]
		meta := after.Meta.Connectors[newKey]
		if after.Connectors[oldKey] != nil || got == nil || got.View != want.View || got.Source != want.Source || got.Target != want.Target || meta == nil || meta.ID != before.Meta.Connectors[oldKey].ID {
			t.Fatalf("connector or metadata did not cascade: %s -> %s", oldKey, newKey)
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "tld.db")); !os.IsNotExist(err) {
		t.Fatalf("YAML ref update opened a target database: %v", err)
	}
}

func TestUpdateElementRefDryRunAndErrorsKeepWorkspaceUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name    string
		from    string
		to      string
		dryRun  bool
		wantErr bool
	}{
		{name: "dry-run", from: "platform", to: "system", dryRun: true},
		{name: "invalid ref", from: "platform", to: "Bad Ref", wantErr: true},
		{name: "existing ref", from: "platform", to: "api", wantErr: true},
		{name: "missing ref", from: "missing", to: "system", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)
			cmd.SeedElementWorkspace(t, dir)
			wsDir := workspace.ResolveDir(dir)
			before := map[string]string{}
			for _, file := range []string{"elements.yaml", "connectors.yaml", ".tld.lock"} {
				data, err := os.ReadFile(filepath.Join(wsDir, file))
				if err != nil {
					t.Fatal(err)
				}
				before[file] = string(data)
			}
			args := []string{"update", "element", tt.from, "ref", tt.to, "--format", "json"}
			if tt.dryRun {
				args = append(args, "--dry-run")
			}
			stdout, _, err := cmd.RunCmd(t, dir, args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error = %v\n%s", err, tt.wantErr, stdout)
			}
			var payload cmdutil.JSONOutput
			if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, stdout)
			}
			if tt.wantErr && (payload.Status != "error" || len(payload.Errors) != 1) {
				t.Fatalf("missing JSON error: %+v", payload)
			}
			if tt.dryRun && (payload.Status != "ok" || len(payload.Items) != 1 || payload.Items[0].Action != "dry-run") {
				t.Fatalf("missing dry-run result: %+v", payload)
			}
			for file, want := range before {
				data, err := os.ReadFile(filepath.Join(wsDir, file))
				if err != nil || string(data) != want {
					t.Fatalf("%s changed: %v", file, err)
				}
			}
		})
	}
}
