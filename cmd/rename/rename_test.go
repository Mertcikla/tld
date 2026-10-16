package rename_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestRenameCmdCascadesElementReferences(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "rename", "--from", "api", "--to", "service-api")
	if err != nil {
		t.Fatalf("rename: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "renamed api") && !strings.Contains(stdout, "service-api") {
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
	connector := ws.Connectors["platform/service-api~db/reads"]
	if connector == nil {
		t.Fatalf("renamed connector key missing: %+v", ws.Connectors)
		return
	}
	if connector.Source != "service-api" || connector.Target != "db" {
		t.Fatalf("connector endpoints = %s -> %s, want service-api -> db", connector.Source, connector.Target)
	}
}

func TestRenameCmdDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	path := filepath.Join(dir, ".tld", "elements.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, _, err := cmd.RunCmd(t, dir, "rename", "--from", "api", "--to", "renamed", "--dry-run")
	if err != nil {
		t.Fatalf("rename dry-run: %v", err)
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

func TestRenameCmdRequiresBothRefs(t *testing.T) {
	_, _, err := cmd.RunCmd(t, t.TempDir(), "rename", "--from", "api")
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "to" not set`) {
		t.Fatalf("err = %v, want cobra required flag validation", err)
	}
}

func TestRenameDeprecatedAliasKeepsJSONStdoutParseable(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)
	stdout, stderr, err := cmd.RunCmd(t, dir, "rename", "--from", "api", "--to", "service-api", "--format", "json")
	if err != nil {
		t.Fatalf("rename alias: %v\n%s\n%s", err, stdout, stderr)
	}
	var payload struct {
		Command string `json:"command"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil || payload.Command != "rename" || payload.Status != "ok" {
		t.Fatalf("invalid legacy JSON result: %s (%v)", stdout, err)
	}
	if !strings.Contains(stderr, "deprecated") || !strings.Contains(stderr, "tld update element <ref> ref <new-ref>") {
		t.Fatalf("missing actionable deprecation notice: %q", stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil || ws.Elements["service-api"] == nil || ws.Connectors["platform/service-api~db/reads"] == nil {
		t.Fatalf("deprecated alias did not cascade: %+v (%v)", ws, err)
	}
}
