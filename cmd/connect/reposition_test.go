package connect_test

import (
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func rootPlacement(t *testing.T, ws *workspace.Workspace, ref string) workspace.ViewPlacement {
	t.Helper()
	element := ws.Elements[ref]
	if element == nil {
		t.Fatalf("element %q not found", ref)
	}
	for _, placement := range element.Placements {
		if placement.ParentRef == "root" {
			return placement
		}
	}
	t.Fatalf("element %q has no root placement: %+v", ref, element.Placements)
	return workspace.ViewPlacement{}
}

func TestConnectCmd_RelocatesFreshTarget(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database", "--position-x", "2000", "--position-y", "2000")

	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	database := rootPlacement(t, ws, "db")
	if database.PositionX == 2000 && database.PositionY == 2000 {
		t.Fatalf("fresh target was not relocated: (%v, %v)", database.PositionX, database.PositionY)
	}
}

func TestConnectCmd_LeavesConnectedTargetAlone(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--position-x", "1000", "--position-y", "500")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database")

	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db")
	// api already has a connector now, so connecting back must not move it.
	cmd.MustRunCmd(t, dir, "connect", "--from", "db", "--to", "api")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	api := rootPlacement(t, ws, "api")
	if api.PositionX != 1000 || api.PositionY != 500 {
		t.Fatalf("connected target was moved: (%v, %v)", api.PositionX, api.PositionY)
	}
}

func TestConnectCmd_NoRepositionFlag(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database", "--position-x", "2000", "--position-y", "2000")

	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--no-reposition")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	database := rootPlacement(t, ws, "db")
	if database.PositionX != 2000 || database.PositionY != 2000 {
		t.Fatalf("--no-reposition still moved the target: (%v, %v)", database.PositionX, database.PositionY)
	}
}
