package remove_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"
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
	if !strings.Contains(stdout, "Removed api from elements.yaml") {
		t.Errorf("stdout %q does not contain success message", stdout)
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
	if !strings.Contains(stdout, "Removed 1 connector(s)") {
		t.Errorf("stdout %q does not contain success message", stdout)
	}
}
