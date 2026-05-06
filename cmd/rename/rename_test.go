package rename_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestRenameCmdCascadesElementReferences(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "rename", "--from", "api", "--to", "service-api")
	if err != nil {
		t.Fatalf("rename: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Renamed element") {
		t.Fatalf("stdout = %q, want rename confirmation", stdout)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Elements["api"]; ok {
		t.Fatalf("old api ref still exists: %+v", ws.Elements)
	}
	if ws.Elements["service-api"] == nil {
		t.Fatalf("new service-api ref missing: %+v", ws.Elements)
	}
	connector := ws.Connectors["platform:service-api:db:reads"]
	if connector == nil {
		t.Fatalf("renamed connector key missing: %+v", ws.Connectors)
	}
	if connector.Source != "service-api" || connector.Target != "db" {
		t.Fatalf("connector endpoints = %s -> %s, want service-api -> db", connector.Source, connector.Target)
	}
}

func TestRenameCmdRequiresBothRefs(t *testing.T) {
	_, _, err := cmd.RunCmd(t, t.TempDir(), "rename", "--from", "api")
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "to" not set`) {
		t.Fatalf("err = %v, want cobra required flag validation", err)
	}
}
