package importcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

const sharedDiagram = `elements:
  custom-system:
    name: System
    kind: system
    has_view: true
    placements: [{parent: root}]
  custom-api:
    name: API
    kind: service
    placements: [{parent: custom-system}]
  custom-db:
    name: Database
    kind: database
    placements: [{parent: custom-system}]
connectors:
  - view: custom-system
    source: custom-api
    target: custom-db
    label: reads
`

func TestSyncCmd_SeparateLocalAndCloudIDs(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "scoped"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) { testSyncTargets(t, legacy) })
	}
}

func testSyncTargets(t *testing.T, legacy bool) {
	localData, cloudData := t.TempDir(), t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_DATA_DIR", localData)
	dir, cloudSeed := t.TempDir(), t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustInitWorkspace(t, cloudSeed)
	cmd.MustRunCmd(t, dir, "import", writeImportFile(t, sharedDiagram), "--target", "local")
	local := readSyncWorkspace(t, dir)
	if legacy {
		lock, err := workspace.LoadLockFile(local.Dir)
		if err != nil {
			t.Fatal(err)
		}
		lock.ActiveTarget, lock.Targets = "", nil
		if err := workspace.WriteLockFile(local.Dir, lock); err != nil {
			t.Fatal(err)
		}
	}
	// Both databases start allocating at the same IDs, but these are unrelated
	// resources, including an owned view and a connector in that view.
	other := "elements:\n  other:\n    name: Other\n    has_view: true\n    placements: [{parent: root}]\n  other-child:\n    name: Other child\n    placements: [{parent: other}]\nconnectors:\n  - view: other\n    source: other\n    target: other-child\n    label: unrelated\n"
	cmd.MustRunCmd(t, cloudSeed, "import", writeImportFile(t, other), "--target", "local", "--data-dir", cloudData)
	seed := readSyncWorkspace(t, cloudSeed)
	cloudStore, err := store.Open(filepath.Join(cloudData, "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cloudStore.Close() })
	serverURL := cmd.NewMockServer(t, &api.WorkspaceService{Store: store.NewAPIAdapter(cloudStore)})
	cmd.WriteConfig(t, dir, serverURL, "test-key")

	before := syncFiles(t, dir)
	cmd.MustRunCmd(t, dir, "sync", "--target", "cloud", "--dry-run")
	if !reflect.DeepEqual(before, syncFiles(t, dir)) {
		t.Fatal("dry run changed YAML or target metadata")
	}
	cmd.MustRunCmd(t, dir, "sync", "--target", "cloud")
	cloud := readSyncWorkspace(t, dir)
	for _, ref := range []string{"custom-system", "custom-api", "custom-db"} {
		if cloud.Meta.Elements[ref] == nil || cloud.Meta.Elements[ref].ID == local.Meta.Elements[ref].ID {
			t.Fatalf("%s reused local ID: local=%v cloud=%v", ref, local.Meta.Elements[ref], cloud.Meta.Elements[ref])
		}
	}
	if cloud.Meta.Views["custom-system"].ID == local.Meta.Views["custom-system"].ID {
		t.Fatal("reused local owned view ID")
	}
	connector := "custom-system/custom-api~custom-db/reads"
	if cloud.Meta.Connectors[connector].ID == local.Meta.Connectors[connector].ID {
		t.Fatal("reused local connector ID")
	}
	if cloud.Elements["other"].Name != "Other" || cloud.Meta.Elements["other"].ID != seed.Meta.Elements["other"].ID {
		t.Fatal("overwrote unrelated cloud resource")
	}
	if err := workspace.UpdateElementField(cloud.Dir, "custom-api", "description", "shared change"); err != nil {
		t.Fatal(err)
	}
	cmd.MustRunCmd(t, dir, "sync", "--target", "local")
	updatedLocal := readSyncWorkspace(t, dir)
	if updatedLocal.Meta.Elements["custom-api"].ID != local.Meta.Elements["custom-api"].ID {
		t.Fatal("lost local mapping on return from cloud")
	}
	cmd.MustRunCmd(t, dir, "sync", "--target", "cloud")
	cmd.MustRunCmd(t, dir, "sync", "--target", "cloud")
	updatedCloud := readSyncWorkspace(t, dir)
	if updatedCloud.Meta.Elements["custom-api"].ID != cloud.Meta.Elements["custom-api"].ID || len(updatedCloud.Elements) != 5 {
		t.Fatal("repeated target switching duplicated resources")
	}
	for _, target := range []string{"local", "cloud", "local"} {
		cmd.MustRunCmd(t, dir, "pull", "--target", target, "--force")
		ws := readSyncWorkspace(t, dir)
		if ws.Elements["custom-api"] == nil || ws.Elements["custom-api"].Description != "shared change" {
			t.Fatalf("pull %s lost shared ref/content", target)
		}
		want := local.Meta.Elements["custom-api"].ID
		if target == "cloud" {
			want = cloud.Meta.Elements["custom-api"].ID
		}
		if ws.Meta.Elements["custom-api"].ID != want {
			t.Fatalf("pull %s used another target's IDs", target)
		}
	}
	// A normal mutation must use the selected namespace too, even when the
	// previously active cache belongs to the other target.
	cmd.MustRunCmd(t, dir, "update", "element", "custom-api", "description", "cloud only", "--target", "cloud")
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := exec.NewRunner(*cfg, "local", localData, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	el, err := runner.GetElement(context.Background(), int32(local.Meta.Elements["custom-api"].ID))
	if err != nil || el.GetDescription() != "shared change" {
		t.Fatalf("cloud mutation affected local target: %v %v", el, err)
	}
	// Ref renames must update the inactive target's current mappings as well.
	cmd.MustRunCmd(t, dir, "update", "element", "custom-api", "ref", "renamed-api")
	if err := workspace.UpdateElementField(local.Dir, "renamed-api", "name", "Renamed API"); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"local", "cloud"} {
		cmd.MustRunCmd(t, dir, "sync", "--target", target)
		ws := readSyncWorkspace(t, dir)
		want := local.Meta.Elements["custom-api"].ID
		if target == "cloud" {
			want = cloud.Meta.Elements["custom-api"].ID
		}
		if ws.Meta.Elements["renamed-api"].ID != want || len(ws.Elements) != 5 {
			t.Fatalf("rename lost %s target mapping", target)
		}
	}
}

func readSyncWorkspace(t *testing.T, dir string) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func syncFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, name := range []string{"elements.yaml", "connectors.yaml", ".tld.lock"} {
		data, err := os.ReadFile(filepath.Join(workspace.ResolveDir(dir), name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	return files
}
