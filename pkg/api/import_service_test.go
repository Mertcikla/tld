package api

import (
	"context"
	"fmt"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
)

func TestImportResourcesDefaultsElementBypassNoiseGateFalse(t *testing.T) {
	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	var createdInput ElementInput
	created := false
	store := &contractStore{
		createElement: func(_ context.Context, id uuid.UUID, input ElementInput) (*diagv1.Element, error) {
			if id != workspaceID {
				t.Fatalf("workspace id = %s, want %s", id, workspaceID)
			}
			created = true
			createdInput = input
			return &diagv1.Element{Id: 1, Name: input.Name}, nil
		},
	}
	service := &ImportService{Store: store}
	requestElement := &diagv1.PlanElement{Ref: "api", Name: "API"}

	_, err := service.ImportResources(context.Background(), connect.NewRequest(&diagv1.ImportResourcesRequest{
		OrgId:    workspaceID.String(),
		Elements: []*diagv1.PlanElement{requestElement},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected element to be created via the sync store method")
	}
	if createdInput.BypassNoiseGate == nil || *createdInput.BypassNoiseGate {
		t.Fatalf("imported bypass_noise_gate = %v, want explicit false", createdInput.BypassNoiseGate)
	}
	if requestElement.BypassNoiseGate != nil {
		t.Fatal("ImportResources should not mutate caller-owned plan elements")
	}
}

// TestImportResourcesSyncWithAutoLayout exercises the granular sync import path
// end-to-end against SQLite, including server-side auto-layout for placements
// that arrive without coordinates.
func TestImportResourcesSyncWithAutoLayout(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	apiStore := NewAPIStore(store)

	isRoot := true
	roots, _, err := apiStore.GetViews(ctx, orgID, nil, &isRoot, "", 1, 0)
	if err != nil {
		t.Fatalf("list root views: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("no root view found")
	}
	root := roots[0]

	svc := &ImportService{Store: apiStore}
	const label = "reads"
	resp, err := svc.ImportResources(ctx, connect.NewRequest(&diagv1.ImportResourcesRequest{
		OrgId: orgID.String(),
		Elements: []*diagv1.PlanElement{
			{Ref: "api", Name: "API", Kind: ptr("service"), Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
			{Ref: "db", Name: "DB", Kind: ptr("database"), Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
		},
		Connectors: []*diagv1.PlanConnector{
			{Ref: "api:db", ViewRef: "root", SourceElementRef: "api", TargetElementRef: "db", Label: ptr(label)},
		},
	}))
	if err != nil {
		t.Fatalf("ImportResources: %v", err)
	}
	if resp.Msg.GetViewId() != root.GetId() {
		t.Fatalf("view id = %d, want root %d", resp.Msg.GetViewId(), root.GetId())
	}

	content, err := apiStore.GetProjectedViewContent(ctx, root.GetId(), orgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(content.GetPlacements()) != 2 {
		t.Fatalf("placements = %d, want 2", len(content.GetPlacements()))
	}
	if len(content.GetConnectors()) != 1 {
		t.Fatalf("connectors = %d, want 1", len(content.GetConnectors()))
	}
	positions := map[string]bool{}
	for _, placement := range content.GetPlacements() {
		positions[fmt.Sprintf("%.0f:%.0f", placement.GetPositionX(), placement.GetPositionY())] = true
	}
	if len(positions) != 2 {
		t.Fatalf("auto-layout positions not distinct: %v", positions)
	}
}

func TestParseStructurizrUsesRootViewForConnectors(t *testing.T) {
	service := &ImportService{}

	resp, err := service.ParseStructurizr(context.Background(), connect.NewRequest(&diagv1.ParseStructurizrRequest{
		Code: `workspace {
  model {
    user = person "User"
    app = softwareSystem "App"
    user -> app "Uses"
  }
}`,
	}))
	if err != nil {
		t.Fatal(err)
	}

	elements := resp.Msg.GetElements()
	if len(elements) != 2 {
		t.Fatalf("elements = %d, want 2", len(elements))
	}
	for _, element := range elements {
		placements := element.GetPlacements()
		if len(placements) != 1 || placements[0].GetParentRef() != "root" {
			t.Fatalf("element %q placements = %+v, want one root placement", element.GetRef(), placements)
		}
	}

	connectors := resp.Msg.GetConnectors()
	if len(connectors) != 1 {
		t.Fatalf("connectors = %d, want 1", len(connectors))
	}
	if got := connectors[0].GetViewRef(); got != "root" {
		t.Fatalf("connector view ref = %q, want root", got)
	}
}
