package add_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// TestSyncCommands_RemoteRoundTrip exercises the synchronous server flow over
// ConnectRPC: every command applies immediately and refreshes the YAML cache.
func TestSyncCommands_RemoteRoundTrip(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	if _, _, err := cmd.RunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace"); err != nil {
		t.Fatalf("remote add platform: %v", err)
	}
	stdout, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	if err != nil {
		t.Fatalf("remote add api: %v", err)
	}
	if !strings.Contains(stdout, "add: api (id=") {
		t.Fatalf("expected immediate server feedback with id, got:\n%s", stdout)
	}
	if _, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "platform", "--label", "runs-on"); err != nil {
		t.Fatalf("remote connect: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "Handles traffic"); err != nil {
		t.Fatalf("remote update: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Elements["api"] == nil || ws.Elements["api"].Description != "Handles traffic" {
		t.Fatalf("YAML cache not refreshed: %+v", ws.Elements["api"])
	}
	if meta := ws.Meta.Elements["api"]; meta == nil || meta.ID == 0 {
		t.Fatalf("element meta missing server id: %+v", meta)
	}
	if len(ws.Connectors) != 1 {
		t.Fatalf("connectors = %d, want 1", len(ws.Connectors))
	}

	if _, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "platform"); err != nil {
		t.Fatalf("remote remove connector: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "remove", "element", "api"); err != nil {
		t.Fatalf("remote remove element: %v", err)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if _, ok := ws.Elements["api"]; ok {
		t.Fatalf("api still present after remove: %+v", ws.Elements)
	}
	if len(ws.Connectors) != 0 {
		t.Fatalf("connectors = %d, want 0", len(ws.Connectors))
	}
}

// TestSyncCommands_RemoteViewLabel verifies that updating an element's
// view_label is propagated to the owned view on the server.
func TestSyncCommands_RemoteViewLabel(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	if _, _, err := cmd.RunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace"); err != nil {
		t.Fatalf("add platform: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service"); err != nil {
		t.Fatalf("add api: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "update", "element", "platform", "view_label", "System"); err != nil {
		t.Fatalf("update view_label: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	meta := ws.Meta.Views["platform"]
	if meta == nil || meta.ID == 0 {
		t.Fatalf("platform view meta missing: %+v", ws.Meta.Views)
	}
	view := svc.View(int32(meta.ID))
	if view == nil {
		t.Fatalf("server view %d not found", meta.ID)
	}
	if view.GetLevelLabel() != "System" {
		t.Fatalf("server view label = %q, want %q", view.GetLevelLabel(), "System")
	}
}

// TestEnsureElementIDDoesNotMaskServerErrors ensures a transient server error is
// surfaced instead of being swallowed by an auto-create attempt.
func TestEnsureElementIDDoesNotMaskServerErrors(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	// Hand-written element with no cached server ID in the lockfile metadata.
	content := "api:\n  name: API\n  kind: service\n  placements:\n    - parent: root\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	svc.ListElementsFunc = func(*diagv1.ListElementsRequest) ([]*diagv1.Element, error) {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("server unavailable"))
	}

	if _, _, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "Handles traffic"); err == nil {
		t.Fatal("expected update to fail when list elements fails")
	}
	if got := svc.ElementCount(); got != 0 {
		t.Fatalf("server element count = %d, want 0 (must not auto-create on server error)", got)
	}
}

// TestSyncCommands_ParentGetsDiagramWhenChildAdded verifies that add by itself
// creates no diagram, and that placing another element under it elevates the
// parent: its diagram is created on the server (with the --diagram-label) and
// recorded in the YAML cache.
func TestSyncCommands_ParentGetsDiagramWhenChildAdded(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	if _, _, err := cmd.RunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace", "--diagram-label", "System"); err != nil {
		t.Fatalf("add platform: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Elements["platform"].HasView {
		t.Fatal("add must not create a diagram by default")
	}
	if ws.Meta.Views["platform"] != nil {
		t.Fatalf("unexpected view metadata after plain add: %+v", ws.Meta.Views)
	}

	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service"); err != nil {
		t.Fatalf("add api: %v", err)
	}

	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if ws.Elements["platform"] == nil || !ws.Elements["platform"].HasView {
		t.Fatalf("expected has_view in YAML: %+v", ws.Elements["platform"])
	}
	elementID := int32(ws.Meta.Elements["platform"].ID)
	viewMeta := ws.Meta.Views["platform"]
	if viewMeta == nil || viewMeta.ID == 0 {
		t.Fatalf("platform view meta missing: %+v", ws.Meta.Views)
	}
	view := svc.View(int32(viewMeta.ID))
	if view == nil {
		t.Fatalf("server view %d not found", viewMeta.ID)
	}
	if view.OwnerElementId == nil || *view.OwnerElementId != elementID {
		t.Fatalf("server view owner = %v, want element %d", view.OwnerElementId, elementID)
	}
	if view.GetLevelLabel() != "System" {
		t.Fatalf("server view label = %q, want %q", view.GetLevelLabel(), "System")
	}
}

// TestSyncCommands_AddDoesNotDuplicateWithoutMetadata verifies that add
// self-heals missing cache metadata by matching the server element by
// name+kind instead of creating a duplicate.
func TestSyncCommands_AddDoesNotDuplicateWithoutMetadata(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	if n := svc.ElementCount(); n != 1 {
		t.Fatalf("precondition: %d elements", n)
	}

	// Simulate hand-written YAML / lost cache metadata.
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	ws.Meta = nil
	if err := workspace.Save(ws); err != nil {
		t.Fatalf("save workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, ".tld.lock")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove lockfile: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	if err != nil {
		t.Fatalf("add after metadata loss: %v", err)
	}
	if n := svc.ElementCount(); n != 1 {
		t.Fatalf("server element count = %d, want 1 (duplicate created)", n)
	}
	if !strings.Contains(stdout, "updated: api") {
		t.Fatalf("expected update feedback, got:\n%s", stdout)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if meta := ws.Meta.Elements["api"]; meta == nil || meta.ID == 0 {
		t.Fatalf("metadata not restored: %+v", ws.Meta.Elements)
	}
}

// TestSyncCommands_AddRecreatesDeletedServerElement verifies that add
// recreates the server element when the cached ID points at a deleted row.
func TestSyncCommands_AddRecreatesDeletedServerElement(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	oldID := int32(ws.Meta.Elements["api"].ID)

	svc.RemoveElement(oldID)

	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service"); err != nil {
		t.Fatalf("add after server deletion: %v", err)
	}
	if n := svc.ElementCount(); n != 1 {
		t.Fatalf("server element count = %d, want 1", n)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if newID := int32(ws.Meta.Elements["api"].ID); newID == oldID {
		t.Fatalf("metadata still points at deleted element %d", oldID)
	}
}

// TestSyncCommands_ConnectPromotesViewInYaml verifies that connect records the
// implicitly created parent view in the YAML cache (has_view + view metadata).
func TestSyncCommands_ConnectPromotesViewInYaml(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "platform")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if !ws.Elements["platform"].HasView {
		t.Fatalf("has_view not promoted in YAML: %+v", ws.Elements["platform"])
	}
	meta := ws.Meta.Views["platform"]
	if meta == nil || meta.ID == 0 {
		t.Fatalf("view metadata missing: %+v", ws.Meta.Views)
	}
	if view := svc.View(int32(meta.ID)); view == nil {
		t.Fatalf("server view %d not found", meta.ID)
	}
}
