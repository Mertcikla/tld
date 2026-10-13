package add_test

import (
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func placementFor(t *testing.T, ws *workspace.Workspace, ref, parent string) workspace.ViewPlacement {
	t.Helper()
	element := ws.Elements[ref]
	if element == nil {
		t.Fatalf("element %q not found", ref)
	}
	for _, placement := range element.Placements {
		if placement.ParentRef == parent {
			return placement
		}
	}
	t.Fatalf("element %q has no placement in %q: %+v", ref, parent, element.Placements)
	return workspace.ViewPlacement{}
}

func loadWorkspace(t *testing.T, dir string) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	return ws
}

func TestAddCmd_AutoPlacesWithoutOverlap(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database")

	ws := loadWorkspace(t, dir)
	api := placementFor(t, ws, "api", "root")
	database := placementFor(t, ws, "db", "root")
	if api.PositionX == database.PositionX && api.PositionY == database.PositionY {
		t.Fatalf("auto-placed elements overlap at (%v, %v)", api.PositionX, api.PositionY)
	}
}

func TestAddCmd_ExplicitPositionWins(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")

	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database", "--position-x", "1000", "--position-y", "500")

	ws := loadWorkspace(t, dir)
	database := placementFor(t, ws, "db", "root")
	if database.PositionX != 1000 || database.PositionY != 500 {
		t.Fatalf("explicit position ignored: got (%v, %v)", database.PositionX, database.PositionY)
	}
}

func TestAddCmd_ReAddPreservesPosition(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--position-x", "1000", "--position-y", "500")

	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")

	ws := loadWorkspace(t, dir)
	api := placementFor(t, ws, "api", "root")
	if api.PositionX != 1000 || api.PositionY != 500 {
		t.Fatalf("re-add moved the element: got (%v, %v)", api.PositionX, api.PositionY)
	}
}
