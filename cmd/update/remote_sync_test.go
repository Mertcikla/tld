package update_test

import (
	"fmt"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
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

// TestSyncCommands_UpdateClearsElementField verifies that clearing a field in
// YAML clears it on the server instead of being treated as "leave unchanged".
func TestSyncCommands_UpdateClearsElementField(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--description", "hello")
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	elementID := int32(ws.Meta.Elements["api"].ID)
	if got := svc.Element(elementID).GetDescription(); got != "hello" {
		t.Fatalf("precondition: server description = %q", got)
	}

	cmd.MustRunCmd(t, dir, "update", "element", "api", "description", "")

	if got := svc.Element(elementID).GetDescription(); got != "" {
		t.Fatalf("server description = %q, want cleared", got)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if ws.Elements["api"].Description != "" {
		t.Fatalf("YAML description = %q, want cleared", ws.Elements["api"].Description)
	}
}

// TestSyncCommands_UpdateClearsConnectorField verifies that clearing a
// connector field survives on the server and the renamed YAML key is kept.
func TestSyncCommands_UpdateClearsConnectorField(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads", "--description", "read path")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	connID := int32(ws.Meta.Connectors["platform:api:db:reads"].ID)
	if got := svc.Connector(connID).GetDescription(); got != "read path" {
		t.Fatalf("precondition: server description = %q", got)
	}

	cmd.MustRunCmd(t, dir, "update", "connector", "platform:api:db:reads", "description", "")

	if got := svc.Connector(connID).GetDescription(); got != "" {
		t.Fatalf("server description = %q, want cleared", got)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if c := ws.Connectors["platform:api:db:reads"]; c == nil || c.Description != "" {
		t.Fatalf("YAML connector = %+v, want cleared description", c)
	}
}

// TestSyncCommands_UpdateDoesNotTouchYamlOnServerFailure verifies the server
// runs first: a failed server write leaves the YAML cache untouched.
func TestSyncCommands_UpdateDoesNotTouchYamlOnServerFailure(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--description", "old")
	svc.UpdateElementFunc = func(*diagv1.UpdateElementRequest) (*diagv1.UpdateElementResponse, error) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("server exploded"))
	}

	if _, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "new"); err == nil {
		t.Fatal("expected update to fail")
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if got := ws.Elements["api"].Description; got != "old" {
		t.Fatalf("YAML description = %q, want unchanged %q", got, "old")
	}

	svc.UpdateConnectorFunc = func(*diagv1.UpdateConnectorRequest) (*diagv1.UpdateConnectorResponse, error) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("server exploded"))
	}
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "root")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads")

	if _, _, err := cmd.RunCmd(t, dir, "update", "connector", "root:api:db:reads", "label", "writes"); err == nil {
		t.Fatal("expected connector update to fail")
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if ws.Connectors["root:api:db:reads"] == nil {
		t.Fatalf("connector key changed despite server failure: %+v", ws.Connectors)
	}
}

// TestSyncCommands_UpdateConnectorRejectsDuplicateKey verifies that a rename
// collision is refused before the server is touched.
func TestSyncCommands_UpdateConnectorRejectsDuplicateKey(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "writes")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	readsID := int32(ws.Meta.Connectors["platform:api:db:reads"].ID)

	if _, _, err := cmd.RunCmd(t, dir, "update", "connector", "platform:api:db:reads", "label", "writes"); err == nil {
		t.Fatal("expected duplicate-key update to fail")
	}
	if got := svc.Connector(readsID).GetLabel(); got != "reads" {
		t.Fatalf("server label = %q, want unchanged %q", got, "reads")
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if ws.Connectors["platform:api:db:reads"] == nil {
		t.Fatalf("YAML key changed despite validation failure: %+v", ws.Connectors)
	}
}
