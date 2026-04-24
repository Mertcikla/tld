package cmdutil

import (
	"testing"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func strPtr(s string) *string {
	return &s
}

func TestConvertExportResponsePreservesRefsAndInfersOwnedViews(t *testing.T) {
	updated := timestamppb.New(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	base := &workspace.Workspace{
		Dir:    "/tmp/project",
		Config: workspace.Config{WorkspaceID: "workspace-id"},
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{
				"api-service": {ID: 10},
			},
			Views: map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{
				"old-connector-ref": {ID: 50},
			},
		},
	}
	msg := &diagv1.ExportOrganizationResponse{
		Elements: []*diagv1.Element{
			{Id: 10, Name: "API Service", Kind: strPtr("container"), HasView: true, ViewLabel: strPtr("Runtime"), Technology: strPtr("Go"), UpdatedAt: updated},
			{Id: 20, Name: "Database", Kind: strPtr("container"), UpdatedAt: updated},
		},
		Views: []*diagv1.View{
			{Id: 100, Name: "API Service", LevelLabel: strPtr("Runtime"), UpdatedAt: updated},
			{Id: 101, Name: "Landscape", UpdatedAt: updated},
		},
		Placements: []*diagv1.ElementPlacement{
			{ViewId: 100, ElementId: 20, PositionX: 12.5, PositionY: 99},
			{ViewId: 999, ElementId: 10, PositionX: 1, PositionY: 2},
		},
		Connectors: []*diagv1.Connector{
			{Id: 50, ViewId: 100, SourceElementId: 10, TargetElementId: 20, Label: strPtr("reads"), Relationship: strPtr("dependency"), Direction: "forward", UpdatedAt: updated},
		},
	}

	got := ConvertExportResponse(base, msg)

	if got.Dir != base.Dir || got.Config.WorkspaceID != base.Config.WorkspaceID {
		t.Fatalf("workspace context was not preserved: %#v", got)
	}
	api := got.Elements["api-service"]
	if api == nil || !api.HasView || api.ViewLabel != "Runtime" || api.Technology != "Go" {
		t.Fatalf("existing element ref or owned view fields were not preserved: %#v", api)
	}
	if got.Meta.Views["api-service"].ID != 100 {
		t.Fatalf("owned view metadata should be keyed by owner ref: %#v", got.Meta.Views)
	}

	db := got.Elements["database"]
	if db == nil || len(db.Placements) != 1 {
		t.Fatalf("database placement was not imported: %#v", db)
	}
	if db.Placements[0].ParentRef != "api-service" || db.Placements[0].PositionX != 12.5 || db.Placements[0].PositionY != 99 {
		t.Fatalf("placement should target inferred owner view: %#v", db.Placements[0])
	}

	connector := got.Connectors["old-connector-ref"]
	if connector == nil {
		t.Fatalf("existing connector ref was not preserved: %#v", got.Connectors)
	}
	if connector.View != "api-service" || connector.Source != "api-service" || connector.Target != "database" || connector.Label != "reads" {
		t.Fatalf("connector refs were not converted through exported IDs: %#v", connector)
	}
	if got.Meta.Connectors["old-connector-ref"].ID != 50 {
		t.Fatalf("connector metadata was not preserved: %#v", got.Meta.Connectors)
	}
}

func TestConvertExportResponseUsesNavigationOwnershipBeforeNameInference(t *testing.T) {
	base := &workspace.Workspace{
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{
				"checkout-a": {ID: 1},
				"checkout-b": {ID: 2},
			},
		},
	}
	msg := &diagv1.ExportOrganizationResponse{
		Elements: []*diagv1.Element{
			{Id: 1, Name: "Checkout", HasView: true},
			{Id: 2, Name: "Checkout", HasView: true},
			{Id: 3, Name: "Worker"},
		},
		Views: []*diagv1.View{
			{Id: 10, Name: "Checkout"},
		},
		Navigations: []*diagv1.ElementNavigation{
			{ElementId: 2, ToViewId: 10},
		},
		Placements: []*diagv1.ElementPlacement{
			{ViewId: 10, ElementId: 3},
		},
	}

	got := ConvertExportResponse(base, msg)

	worker := got.Elements["worker"]
	if worker == nil || len(worker.Placements) != 1 {
		t.Fatalf("worker placement was not imported: %#v", worker)
	}
	if worker.Placements[0].ParentRef != "checkout-b" {
		t.Fatalf("navigation-owned view should be used as placement parent, got %#v", worker.Placements[0])
	}
	if got.Meta.Views["checkout-b"].ID != 10 {
		t.Fatalf("view metadata should belong to the navigation owner: %#v", got.Meta.Views)
	}
}
