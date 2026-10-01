package connect_test

import (
	"reflect"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestConnectCmdPersistsTags(t *testing.T) {
	dir := t.TempDir()
	setupWorkspaceForLinks(t, dir)

	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads", "--tags", "critical, api")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	connector := ws.Connectors["platform/api~db/reads"]
	if connector == nil {
		t.Fatalf("connector missing: %+v", ws.Connectors)
	}
	if !reflect.DeepEqual(connector.Tags, []string{"critical", "api"}) {
		t.Fatalf("connector tags = %v, want [critical api]", connector.Tags)
	}
}
