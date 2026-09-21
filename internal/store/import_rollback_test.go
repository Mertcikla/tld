package store

import (
	"context"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/api"
)

func rollbackStrPtr(s string) *string { return &s }

// TestImportServiceRollsBackPartialImports verifies that a failing import
// leaves no partial state behind when the store supports transactions.
func TestImportServiceRollsBackPartialImports(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	adapter := NewAPIAdapter(sqliteStore)
	ctx := context.Background()
	orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	svc := &api.ImportService{Store: adapter}
	_, err := svc.ImportResources(ctx, connect.NewRequest(&diagv1.ImportResourcesRequest{
		OrgId: orgID.String(),
		Elements: []*diagv1.PlanElement{
			{Ref: "platform", Name: "Platform", Kind: rollbackStrPtr("workspace"), HasView: true},
			{
				Ref:  "api",
				Name: "API",
				Kind: rollbackStrPtr("service"),
				Placements: []*diagv1.PlanViewPlacement{
					{ParentRef: "platform"},
				},
			},
		},
		Connectors: []*diagv1.PlanConnector{
			{Ref: "api:ghost", ViewRef: "platform", SourceElementRef: "api", TargetElementRef: "ghost", Label: rollbackStrPtr("reads")},
		},
	}))
	if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
		t.Fatalf("expected unknown target element error, got: %v", err)
	}

	elements, _, err := adapter.ListElements(ctx, uuid.Nil, 0, 0, "")
	if err != nil {
		t.Fatalf("list elements: %v", err)
	}
	if len(elements) != 0 {
		t.Fatalf("elements after rollback = %d, want 0 (partial import leaked)", len(elements))
	}
	views, err := adapter.ListViews(ctx, uuid.Nil)
	if err != nil {
		t.Fatalf("list views: %v", err)
	}
	// The store seeds a bootstrap root view on open; only the import's owned
	// view must be gone.
	for _, v := range views {
		if v.OwnerElementId != nil {
			t.Fatalf("owned view leaked after rollback: %+v", v)
		}
	}
}
