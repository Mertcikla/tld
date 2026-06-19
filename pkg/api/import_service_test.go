package api

import (
	"context"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
)

func TestImportResourcesDefaultsElementBypassNoiseGateFalse(t *testing.T) {
	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	var applied *diagv1.ApplyPlanRequest
	store := &contractStore{
		applyPlan: func(_ context.Context, id uuid.UUID, req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if id != workspaceID {
				t.Fatalf("workspace id = %s, want %s", id, workspaceID)
			}
			applied = req
			return &diagv1.ApplyPlanResponse{CreatedPlacements: []*diagv1.ElementPlacement{{ViewId: 7}}}, nil
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
	if applied == nil || len(applied.GetElements()) != 1 {
		t.Fatalf("applied request = %+v, want one element", applied)
	}
	if applied.GetElements()[0].BypassNoiseGate == nil || applied.GetElements()[0].GetBypassNoiseGate() {
		t.Fatalf("imported bypass_noise_gate = %v, want explicit false", applied.GetElements()[0].BypassNoiseGate)
	}
	if requestElement.BypassNoiseGate != nil {
		t.Fatal("ImportResources should not mutate caller-owned plan elements")
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
