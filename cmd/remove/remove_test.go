package remove_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestRemoveElementCmd_LocalOnly(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "element", "api")
	if err != nil {
		t.Fatalf("remove element: %v", err)
	}
	if !strings.Contains(stdout, "del: api") {
		t.Errorf("stdout %q does not contain success message", stdout)
	}
}

func TestRemoveElementCmd_ReferencedElementFails(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")

	_, _, err := cmd.RunCmd(t, dir, "remove", "element", "platform")
	if err == nil {
		t.Fatal("expected referenced element removal to fail")
	}
	msg := err.Error()
	for _, want := range []string{
		`element "platform" is still referenced`,
		"elements.yaml[api].placements[0].parent",
		"connectors.yaml[platform:api:db:reads].view",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Elements["platform"] == nil {
		t.Fatal("platform should not have been deleted")
	}
}

func TestRemoveConnectorCmd(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace", "--with-view")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("remove connector: %v", err)
	}
	if !strings.Contains(stdout, "del: 1") {
		t.Errorf("stdout %q does not contain success message", stdout)
	}
}

func TestRemoveConnectorCmd_AmbiguousRequiresLabel(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "writes")

	_, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db")
	if err == nil {
		t.Fatal("expected ambiguous connector removal to fail")
	}
	if !strings.Contains(err.Error(), "multiple connectors match") || !strings.Contains(err.Error(), "platform:api:db:reads") || !strings.Contains(err.Error(), "platform:api:db:writes") {
		t.Fatalf("unexpected error: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")
	if err != nil {
		t.Fatalf("remove connector with label: %v", err)
	}
	if !strings.Contains(stdout, "del: 1") {
		t.Fatalf("missing delete count: %s", stdout)
	}
	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Connectors["platform:api:db:reads"] != nil || ws.Connectors["platform:api:db:writes"] == nil {
		t.Fatalf("unexpected connectors after delete: %+v", ws.Connectors)
	}
}
