package update_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestUpdateElementCmdSetsAppendsAndRemovesTags(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "update", "element", "api", "tags", "edge, core")
	assertElementTags(t, dir, []string{"edge", "core"})

	cmd.MustRunCmd(t, dir, "update", "element", "api", "tags", "core, public", "--append")
	assertElementTags(t, dir, []string{"edge", "core", "public"})

	cmd.MustRunCmd(t, dir, "update", "element", "api", "tags", "edge", "--remove")
	assertElementTags(t, dir, []string{"core", "public"})
}

func TestUpdateElementCmdTagsDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	path := filepath.Join(dir, ".tld", "elements.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "tags", "edge", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run update: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Fatalf("stdout = %q, want dry-run confirmation", stdout)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("elements.yaml changed during dry-run")
	}
}

func TestUpdateConnectorCmdSetsAndAppendsTags(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	key := "platform/api~db/reads"
	cmd.MustRunCmd(t, dir, "update", "connector", key, "tags", "critical")
	assertConnectorTags(t, dir, key, []string{"critical"})

	cmd.MustRunCmd(t, dir, "update", "connector", key, "tags", "hot", "--append")
	assertConnectorTags(t, dir, key, []string{"critical", "hot"})
}

func TestUpdateCmdRejectsTagFlagsOnOtherFields(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "x", "--append")
	if err == nil || !strings.Contains(err.Error(), "only valid with the tags field") {
		t.Fatalf("err = %v, want tag-flag validation error", err)
	}
}

func assertElementTags(t *testing.T, dir string, want []string) {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ws.Elements["api"].Tags; !reflect.DeepEqual(got, want) {
		t.Fatalf("element tags = %v, want %v", got, want)
	}
}

func assertConnectorTags(t *testing.T, dir, key string, want []string) {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	connector := ws.Connectors[key]
	if connector == nil {
		t.Fatalf("connector %q missing", key)
	}
	if got := connector.Tags; !reflect.DeepEqual(got, want) {
		t.Fatalf("connector tags = %v, want %v", got, want)
	}
}
