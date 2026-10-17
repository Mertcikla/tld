package importcmd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// TestSyncCmd_PushesYAMLLinkToStore mirrors the tld link workflow: the source
// link is written to elements.yaml first, then tld sync pushes it to the store.
func TestSyncCmd_PushesYAMLLinkToStore(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Data", "--ref", "data")

	wsDir := workspace.ResolveDir(dir)
	if err := workspace.UpdateElementField(wsDir, "data", "file_path", "internal/workspace/"); err != nil {
		t.Fatalf("write yaml link: %v", err)
	}
	if err := workspace.UpdateElementField(wsDir, "data", "repository_id", "repo-1"); err != nil {
		t.Fatalf("write yaml repository_id: %v", err)
	}

	cmd.MustRunCmd(t, dir, "sync")

	el := loadElement(t, dir, "data")
	if el.GetFilePath() != "internal/workspace/" || el.GetRepositoryId() != "repo-1" {
		t.Fatalf("store element not synced: file_path=%q repository_id=%q", el.GetFilePath(), el.GetRepositoryId())
	}
}

func TestSyncCmd_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Data", "--ref", "data")

	if err := workspace.UpdateElementField(workspace.ResolveDir(dir), "data", "file_path", "internal/workspace/"); err != nil {
		t.Fatalf("write yaml link: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "sync", "--dry-run")
	if err != nil {
		t.Fatalf("sync dry-run: %v", err)
	}
	if !strings.Contains(stdout, "would sync") {
		t.Fatalf("unexpected dry-run output: %s", stdout)
	}
	if el := loadElement(t, dir, "data"); el.GetFilePath() != "" {
		t.Fatalf("dry-run wrote to the store: file_path=%q", el.GetFilePath())
	}
}

func TestSyncCmd_JSONCommandLabel(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Data", "--ref", "data")

	stdout, _, err := cmd.RunCmd(t, dir, "--format", "json", "sync")
	if err != nil {
		t.Fatalf("sync json: %v", err)
	}
	if !strings.Contains(stdout, `"command": "sync"`) {
		t.Fatalf("expected command=sync in JSON, got: %s", stdout)
	}
}

func TestSyncCmd_ReportsChangesAndSkipsUnchangedWrites(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "import", writeImportFile(t, basicImport))
	cmd.MustRunCmd(t, dir, "view", "create", "api")
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := exec.NewRunner(ws.Config, "local", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	db, err := store.Open(runner.TargetLabel(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// Fail even when a redundant UPDATE would leave the same timestamp.
	for table, condition := range map[string]string{
		"elements":   "WHEN OLD.description IS NEW.description",
		"connectors": "WHEN OLD.description IS NEW.description",
		"views":      "", "placements": "",
	} {
		sql := fmt.Sprintf("CREATE TRIGGER reject_%s_update BEFORE UPDATE ON %s %s BEGIN SELECT RAISE(ABORT, 'unchanged write'); END", table, table, condition)
		if _, err := db.DB().ExecContext(context.Background(), sql); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want int, args ...string) {
		t.Helper()
		out, _ := cmd.MustRunCmd(t, dir, append([]string{"sync", "--format", "json"}, args...)...)
		var result cmdutil.JSONOutput
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		if result.Summary["elements_updated"] != want || result.Summary["connectors_updated"] != want {
			t.Fatalf("unexpected summary: %s", out)
		}
		for _, item := range result.Items {
			expected := "unchanged"
			if want == 1 && (item.ResourceType == "connector" || (item.ResourceType == "element" && item.Ref == "api")) {
				expected = "update"
			}
			if item.Action != expected {
				t.Errorf("%s %s: action %s, want %s", item.ResourceType, item.Ref, item.Action, expected)
			}
		}
	}
	check(0)
	ws.Elements["api"].Description = "changed element"
	for _, c := range ws.Connectors {
		c.Description = "changed connector"
	}
	if err := workspace.Save(ws); err != nil {
		t.Fatal(err)
	}
	check(1, "--dry-run")
	if got := loadElement(t, dir, "api").GetDescription(); got != "" {
		t.Fatalf("dry-run wrote description: %q", got)
	}
	check(1)
	check(0)
}

func loadElement(t *testing.T, dir, ref string) *diagv1.Element {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := exec.NewRunner(ws.Config, "local", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	meta := ws.Meta.Elements[ref]
	if meta == nil {
		t.Fatalf("no metadata for element %q", ref)
	}
	el, err := runner.GetElement(context.Background(), int32(meta.ID))
	if err != nil {
		t.Fatal(err)
	}
	return el
}
