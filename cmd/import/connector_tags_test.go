package importcmd_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

func TestConnectorTagsImportAndSync(t *testing.T) {
	for _, target := range []string{"local", "remote"} {
		for _, command := range []string{"import", "sync"} {
			t.Run(target+"/"+command, func(t *testing.T) {
				dataDir, dir := t.TempDir(), t.TempDir()
				t.Setenv("TLD_CONFIG_DIR", t.TempDir())
				t.Setenv("TLD_DATA_DIR", dataDir)
				cmd.MustInitWorkspace(t, dir)
				appStore, err := store.Open(filepath.Join(dataDir, "tld.db"), assets.FS)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = appStore.Close() })
				adapter := store.NewAPIAdapter(appStore)
				if target == "remote" {
					cmd.WriteConfig(t, dir, cmd.NewMockServer(t, &api.WorkspaceService{Store: adapter}), "test-key")
				}
				parts := strings.SplitN(strings.TrimSpace(basicImport), "\nconnectors:\n", 2)
				connectorYAML := parts[1] + "\n"
				writeYAML := func(name, body string) {
					t.Helper()
					if err := os.WriteFile(filepath.Join(workspace.ResolveDir(dir), name), []byte(body), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				snapshot := func() map[string]string {
					t.Helper()
					files := map[string]string{}
					for _, name := range []string{"elements.yaml", "connectors.yaml", ".tld.lock"} {
						data, err := os.ReadFile(filepath.Join(workspace.ResolveDir(dir), name))
						if os.IsNotExist(err) {
							continue
						}
						if err != nil {
							t.Fatal(err)
						}
						files[name] = string(data)
					}
					return files
				}
				args := []string{command}
				if command == "import" {
					args = append(args, writeImportFile(t, basicImport+"    tags: [feat]\n"))
				} else {
					writeYAML("elements.yaml", strings.TrimPrefix(parts[0], "elements:\n")+"\n")
					writeYAML("connectors.yaml", connectorYAML+"    tags: [feat]\n")
				}
				args = append(args, "--target", target, "--data-dir", dataDir)
				before := snapshot()
				cmd.MustRunCmd(t, dir, append(slices.Clone(args), "--dry-run")...)
				if !reflect.DeepEqual(before, snapshot()) {
					t.Fatal("dry-run changed workspace files")
				}
				connectors, err := adapter.ListAllConnectors(context.Background(), uuid.Nil)
				if err != nil || len(connectors) != 0 {
					t.Fatalf("dry-run created connectors: %v, %v", connectors, err)
				}
				cmd.MustRunCmd(t, dir, args...)
				var connectorID int32
				checkTags := func(want []string) {
					t.Helper()
					connectors, err := adapter.ListAllConnectors(context.Background(), uuid.Nil)
					if err != nil || len(connectors) != 1 {
						t.Fatalf("connectors = %v, %v; want one", connectors, err)
					}
					if !slices.Equal(connectors[0].GetTags(), want) {
						t.Fatalf("tags = %v, want %v", connectors[0].GetTags(), want)
					}
					if connectorID != 0 && connectors[0].GetId() != connectorID {
						t.Fatal("sync replaced the connector")
					}
					connectorID = connectors[0].GetId()
				}
				checkTags([]string{"feat"})
				syncArgs := []string{"sync", "--target", target, "--data-dir", dataDir, "--format", "json"}
				checkSummary := func(want int, dryRun bool) {
					t.Helper()
					args := slices.Clone(syncArgs)
					if dryRun {
						args = append(args, "--dry-run")
					}
					out, _ := cmd.MustRunCmd(t, dir, args...)
					var result cmdutil.JSONOutput
					if err := json.Unmarshal([]byte(out), &result); err != nil {
						t.Fatal(err)
					}
					if result.Summary["connectors_updated"] != want || result.Summary["connectors_created"] != 0 {
						t.Fatalf("unexpected summary: %s", out)
					}
				}
				checkSummary(0, false)
				previous := []string{"feat"}
				for _, step := range []struct {
					yaml string
					tags []string
				}{
					{"    tags: [critical]\n", []string{"critical"}},
					{"    description: changed without tags\n", []string{"critical"}},
					{"    tags: []\n", nil},
				} {
					writeYAML("connectors.yaml", connectorYAML+step.yaml)
					before := snapshot()
					checkSummary(1, true)
					checkTags(previous)
					if !reflect.DeepEqual(before, snapshot()) {
						t.Fatal("update dry-run changed workspace files")
					}
					checkSummary(1, false)
					checkTags(step.tags)
					checkSummary(0, false)
					previous = step.tags
				}
			})
		}
	}
}
