package export_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestExportCmd(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ExportFunc: func(_ *diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			resp := &diagv1.ExportOrganizationResponse{
				Views: []*diagv1.View{
					{Id: 1, Name: "D1", UpdatedAt: timestamppb.Now()},
				},
				Elements: []*diagv1.Element{
					{Id: 2, Name: "O1", Kind: new("service"), UpdatedAt: timestamppb.Now()},
				},
				Placements: []*diagv1.ElementPlacement{
					{ViewId: 1, ElementId: 2, PositionX: 10, PositionY: 20},
				},
			}
			return resp, nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	stdout, _, err := cmd.RunCmd(t, dir, "export")
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if !strings.Contains(stdout, "Exported 1 elements, 0 diagrams, 0 connectors") {
		t.Errorf("stdout %q does not contain success message", stdout)
	}

	// Verify files
	if _, err := os.Stat(filepath.Join(dir, "elements.yaml")); os.IsNotExist(err) {
		t.Error("elements.yaml not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "connectors.yaml")); os.IsNotExist(err) {
		t.Error("connectors.yaml not created")
	}
}

func TestExportCmd_MapsOwnedViewsToElements(t *testing.T) {
	now := timestamppb.Now()
	rootID := int32(1)
	apiViewID := int32(2)

	svc := &cmd.MockDiagramService{
		ExportFunc: func(_ *diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			resp := &diagv1.ExportOrganizationResponse{
				Views: []*diagv1.View{
					{Id: rootID, Name: "Workspace Root", UpdatedAt: now},
					{Id: apiViewID, Name: "API Service", LevelLabel: new("Container"), ParentViewId: &rootID, UpdatedAt: now},
				},
				Elements: []*diagv1.Element{
					{Id: 10, Name: "API Service", Kind: new("service"), HasView: true, ViewLabel: new("Container"), UpdatedAt: now},
					{Id: 11, Name: "Worker", Kind: new("service"), UpdatedAt: now},
				},
				Navigations: []*diagv1.ElementNavigation{
					{Id: 20, ElementId: 10, FromViewId: rootID, ToViewId: apiViewID},
				},
				Placements: []*diagv1.ElementPlacement{
					{ViewId: rootID, ElementId: 10, PositionX: 10, PositionY: 20},
					{ViewId: apiViewID, ElementId: 11, PositionX: 30, PositionY: 40},
				},
			}
			return resp, nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	stdout, _, err := cmd.RunCmd(t, dir, "export")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(stdout, "Exported 2 elements, 1 diagrams, 0 connectors") {
		t.Fatalf("stdout %q does not contain owned-view count summary", stdout)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	api := ws.Elements["api-service"]
	if api == nil {
		t.Fatal("api-service element missing from exported workspace")
	}
	if !api.HasView || api.ViewLabel != "Container" {
		t.Fatalf("api-service view metadata = %+v, want has_view true and label Container", api)
	}
	if len(api.Placements) != 1 || api.Placements[0].ParentRef != "root" {
		t.Fatalf("api-service placements = %+v, want root placement", api.Placements)
	}

	worker := ws.Elements["worker"]
	if worker == nil {
		t.Fatal("worker element missing from exported workspace")
	}
	if len(worker.Placements) != 1 || worker.Placements[0].ParentRef != "api-service" {
		t.Fatalf("worker placements = %+v, want parent api-service", worker.Placements)
	}
	if ws.Meta.Views["api-service"] == nil {
		t.Fatal("api-service view metadata missing")
	}
}
