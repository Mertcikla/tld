package update_test

import (
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// TestUpdateConnectorSourceChangeSyncsServer verifies that updating a
// key-changing connector field (source/target) syncs the server and lands on
// the renamed YAML key.
func TestUpdateConnectorSourceChangeSyncsServer(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "add", "Cache", "--ref", "cache", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads")

	if _, _, err := cmd.RunCmd(t, dir, "update", "connector", "platform:api:db:reads", "source", "cache"); err != nil {
		t.Fatalf("update connector source: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Connectors["platform:cache:db:reads"] == nil || ws.Connectors["platform:cache:db:reads"].Source != "cache" {
		t.Fatalf("renamed connector missing: %+v", ws.Connectors)
	}
	cacheID := int32(ws.Meta.Elements["cache"].ID)
	connID := int32(ws.Meta.Connectors["platform:cache:db:reads"].ID)
	serverConn := svc.Connector(connID)
	if serverConn == nil {
		t.Fatalf("server connector %d not found", connID)
	}
	if serverConn.GetSourceElementId() != cacheID {
		t.Fatalf("server connector source = %d, want element %d", serverConn.GetSourceElementId(), cacheID)
	}
}

// TestUpdateElementViewLabelPromotesYamlHasView verifies that setting
// view_label on a viewless element creates the owned view server-side and
// promotes has_view in the YAML cache (instead of leaving them out of sync).
func TestUpdateElementViewLabelPromotesYamlHasView(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service"); err != nil {
		t.Fatalf("add api: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Elements["api"].HasView {
		t.Fatal("precondition: api should start without a view")
	}

	if _, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "view_label", "Backoffice"); err != nil {
		t.Fatalf("update view_label: %v", err)
	}

	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if !ws.Elements["api"].HasView {
		t.Fatalf("expected has_view to be promoted in YAML: %+v", ws.Elements["api"])
	}
	viewMeta := ws.Meta.Views["api"]
	if viewMeta == nil || viewMeta.ID == 0 {
		t.Fatalf("view meta missing: %+v", ws.Meta.Views)
	}
	view := svc.View(int32(viewMeta.ID))
	if view == nil {
		t.Fatalf("server view %d not found", viewMeta.ID)
	}
	if view.GetLevelLabel() != "Backoffice" {
		t.Fatalf("server view label = %q, want %q", view.GetLevelLabel(), "Backoffice")
	}
	elementID := int32(ws.Meta.Elements["api"].ID)
	if view.OwnerElementId == nil || *view.OwnerElementId != elementID {
		t.Fatalf("server view owner = %v, want element %d", view.OwnerElementId, elementID)
	}
}
