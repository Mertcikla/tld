package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestWorkspaceServiceSQLiteCriticalPathRoundTrip(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	client := newSQLiteWorkspaceClient(t)

	root, err := client.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		OrgId:      orgID.String(),
		Name:       "Critical Path",
		LevelLabel: ptr("System"),
	}))
	if err != nil {
		t.Fatal(err)
	}

	apiElement, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name:        "API",
		Kind:        ptr("service"),
		Description: ptr("Handles requests"),
		TechnologyLinks: []*diagv1.TechnologyLink{{
			Type:          "catalog",
			Slug:          ptr("go"),
			Label:         "Go",
			IsPrimaryIcon: true,
		}},
		Tags: []string{"critical", "backend"},
		Repo: ptr("mertcikla/tld"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	dbElement, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name: "Database",
		Kind: ptr("database"),
		Tags: []string{"critical", "storage"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	childView, err := client.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		OrgId:          orgID.String(),
		Name:           "API Internals",
		OwnerElementId: ptr(apiElement.Msg.GetElement().GetId()),
		LevelLabel:     ptr("Component"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if childView.Msg.GetView().GetOwnerElementId() != apiElement.Msg.GetElement().GetId() {
		t.Fatalf("child owner = %d, want API element", childView.Msg.GetView().GetOwnerElementId())
	}

	if _, err := client.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: apiElement.Msg.GetElement().GetId(),
		PositionX: 120,
		PositionY: 140,
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: dbElement.Msg.GetElement().GetId(),
		PositionX: 460,
		PositionY: 140,
	})); err != nil {
		t.Fatal(err)
	}

	connector, err := client.CreateConnector(ctx, connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId:          root.Msg.GetView().GetId(),
		SourceElementId: apiElement.Msg.GetElement().GetId(),
		TargetElementId: dbElement.Msg.GetElement().GetId(),
		Label:           ptr("reads"),
		Direction:       "forward",
		Style:           "bezier",
		Tags:            []string{"runtime"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	updatedConnector, err := client.UpdateConnector(ctx, connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId:  connector.Msg.GetConnector().GetId(),
		Label:        ptr("writes"),
		Relationship: ptr("SQL"),
		Style:        "smoothstep",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updatedConnector.Msg.GetConnector().GetLabel() != "writes" || updatedConnector.Msg.GetConnector().GetStyle() != "smoothstep" {
		t.Fatalf("updated connector = %+v, want label writes and smoothstep style", updatedConnector.Msg.GetConnector())
	}

	updatedElement, err := client.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId:   apiElement.Msg.GetElement().GetId(),
		Name:        "API Gateway",
		Kind:        ptr("service"),
		Description: ptr("Updated through the shared API"),
		TechnologyLinks: []*diagv1.TechnologyLink{{
			Type:          "catalog",
			Slug:          ptr("go"),
			Label:         "Go",
			IsPrimaryIcon: true,
		}},
		Tags: []string{"critical", "edge"},
		Url:  ptr("https://example.com/runbook"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updatedElement.Msg.GetElement().GetName() != "API Gateway" || updatedElement.Msg.GetElement().GetTags()[1] != "edge" {
		t.Fatalf("updated element = %+v, want renamed element with tags", updatedElement.Msg.GetElement())
	}

	search, err := client.ListElements(ctx, connect.NewRequest(&diagv1.ListElementsRequest{Search: "gateway"}))
	if err != nil {
		t.Fatal(err)
	}
	if search.Msg.GetPagination().GetTotalCount() != 1 || search.Msg.GetElements()[0].GetId() != apiElement.Msg.GetElement().GetId() {
		t.Fatalf("search response = %+v, want only updated API element", search.Msg)
	}

	reloaded, err := client.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
		ViewId:         root.Msg.GetView().GetId(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Msg.GetContent().GetPlacements()) != 2 || len(reloaded.Msg.GetContent().GetConnectors()) != 1 {
		t.Fatalf("reloaded content = %+v, want two placements and one connector", reloaded.Msg.GetContent())
	}

	workspace, err := client.GetWorkspace(ctx, connect.NewRequest(&diagv1.GetWorkspaceRequest{
		OrgId:          orgID.String(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Msg.GetTotalCount() < 2 || !workspaceHasView(workspace.Msg.GetViews(), root.Msg.GetView().GetId()) || !workspaceHasView(workspace.Msg.GetViews(), childView.Msg.GetView().GetId()) {
		t.Fatalf("workspace response = %+v, want created root and child views", workspace.Msg)
	}

	if _, err := client.DeleteConnector(ctx, connect.NewRequest(&diagv1.DeleteConnectorRequest{
		OrgId:       orgID.String(),
		ConnectorId: connector.Msg.GetConnector().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := client.ListConnectors(ctx, connect.NewRequest(&diagv1.ListConnectorsRequest{
		ViewId: root.Msg.GetView().GetId(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(afterDelete.Msg.GetConnectors()) != 0 {
		t.Fatalf("connectors after delete = %+v, want none", afterDelete.Msg.GetConnectors())
	}

	if _, err := client.DeletePlacement(ctx, connect.NewRequest(&diagv1.DeletePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: dbElement.Msg.GetElement().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	afterPlacementDelete, err := client.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
		ViewId:         root.Msg.GetView().GetId(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(afterPlacementDelete.Msg.GetContent().GetPlacements()) != 1 {
		t.Fatalf("placements after delete = %+v, want one remaining placement", afterPlacementDelete.Msg.GetContent().GetPlacements())
	}
	library, err := client.ListElements(ctx, connect.NewRequest(&diagv1.ListElementsRequest{Search: "Database"}))
	if err != nil {
		t.Fatal(err)
	}
	if library.Msg.GetPagination().GetTotalCount() != 1 {
		t.Fatalf("database library search total = %d, want element preserved after placement delete", library.Msg.GetPagination().GetTotalCount())
	}
}

func TestWorkspaceServiceSQLiteElementBypassNoiseGateCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	client := newSQLiteWorkspaceClient(t)

	created, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name: "Manual Element",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if created.Msg.GetElement().GetBypassNoiseGate() {
		t.Fatal("CreateElement should default bypass_noise_gate to false")
	}

	enabled := true
	updated, err := client.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId:       created.Msg.GetElement().GetId(),
		Name:            "Manual Element",
		BypassNoiseGate: &enabled,
		TechnologyLinks: created.Msg.GetElement().GetTechnologyLinks(),
		Tags:            created.Msg.GetElement().GetTags(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Msg.GetElement().GetBypassNoiseGate() {
		t.Fatal("UpdateElement should enable explicit bypass_noise_gate=true")
	}

	preserved, err := client.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId:       created.Msg.GetElement().GetId(),
		Name:            "Manual Element Renamed",
		TechnologyLinks: updated.Msg.GetElement().GetTechnologyLinks(),
		Tags:            updated.Msg.GetElement().GetTags(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !preserved.Msg.GetElement().GetBypassNoiseGate() {
		t.Fatal("UpdateElement should preserve bypass_noise_gate when omitted")
	}

	disabled := false
	updated, err = client.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId:       created.Msg.GetElement().GetId(),
		Name:            "Manual Element Renamed",
		BypassNoiseGate: &disabled,
		TechnologyLinks: preserved.Msg.GetElement().GetTechnologyLinks(),
		Tags:            preserved.Msg.GetElement().GetTags(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Msg.GetElement().GetBypassNoiseGate() {
		t.Fatal("UpdateElement should disable explicit bypass_noise_gate=false")
	}
}

func TestWorkspaceServiceSQLiteGetWorkspaceContentPreservesPlacementBypassNoiseGate(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	client := newSQLiteWorkspaceClient(t)

	view, err := client.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		OrgId: orgID.String(),
		Name:  "Workspace Content",
	}))
	if err != nil {
		t.Fatal(err)
	}
	element, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name: "Bypassed Element",
		Kind: ptr("service"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := client.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId:       element.Msg.GetElement().GetId(),
		Name:            element.Msg.GetElement().GetName(),
		Kind:            element.Msg.GetElement().Kind,
		BypassNoiseGate: &enabled,
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    view.Msg.GetView().GetId(),
		ElementId: element.Msg.GetElement().GetId(),
	})); err != nil {
		t.Fatal(err)
	}

	reloadedView, err := client.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
		ViewId:         view.Msg.GetView().GetId(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	viewPlacements := reloadedView.Msg.GetContent().GetPlacements()
	if len(viewPlacements) != 1 || !viewPlacements[0].GetBypassNoiseGate() {
		t.Fatalf("GetView placement bypass_noise_gate = %+v, want true", viewPlacements)
	}

	workspace, err := client.GetWorkspace(ctx, connect.NewRequest(&diagv1.GetWorkspaceRequest{
		OrgId:          orgID.String(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	workspacePlacements := workspace.Msg.GetContent()[view.Msg.GetView().GetId()].GetPlacements()
	if len(workspacePlacements) != 1 || !workspacePlacements[0].GetBypassNoiseGate() {
		t.Fatalf("GetWorkspace placement bypass_noise_gate = %+v, want true", workspacePlacements)
	}
}

func TestWorkspaceServiceSQLiteGetViewHonorsElementOverrideThreshold(t *testing.T) {
	for _, tt := range []struct {
		name           string
		noiseGateLevel int
		hiddenDensity  int
		visibleDensity int
	}{
		{name: "rich gate hides at normal", noiseGateLevel: 1, hiddenDensity: 0, visibleDensity: 1},
		{name: "full gate hides at rich", noiseGateLevel: 2, hiddenDensity: 1, visibleDensity: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
			store, client := newSQLiteWorkspaceClientWithStore(t)

			view, err := client.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
				OrgId: orgID.String(),
				Name:  "Noise Gate",
			}))
			if err != nil {
				t.Fatal(err)
			}
			source, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
				Name: "Visible Source",
				Kind: ptr("service"),
			}))
			if err != nil {
				t.Fatal(err)
			}
			target, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
				Name: "Gated Target",
				Kind: ptr("service"),
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
				ViewId:    view.Msg.GetView().GetId(),
				ElementId: source.Msg.GetElement().GetId(),
			})); err != nil {
				t.Fatal(err)
			}
			if _, err := client.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
				ViewId:    view.Msg.GetView().GetId(),
				ElementId: target.Msg.GetElement().GetId(),
			})); err != nil {
				t.Fatal(err)
			}
			connector, err := client.CreateConnector(ctx, connect.NewRequest(&diagv1.CreateConnectorRequest{
				ViewId:          view.Msg.GetView().GetId(),
				SourceElementId: source.Msg.GetElement().GetId(),
				TargetElementId: target.Msg.GetElement().GetId(),
				Direction:       "forward",
				Style:           "bezier",
			}))
			if err != nil {
				t.Fatal(err)
			}

			viewID := int64(view.Msg.GetView().GetId())
			targetID := target.Msg.GetElement().GetId()
			connectorID := connector.Msg.GetConnector().GetId()
			if _, err := store.SetVisibilityOverride(ctx, viewID, "element", int64(targetID), -tt.noiseGateLevel); err != nil {
				t.Fatal(err)
			}
			if err := store.SetViewDensityLevel(ctx, viewID, tt.hiddenDensity); err != nil {
				t.Fatal(err)
			}

			hidden, err := client.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
				ViewId:         view.Msg.GetView().GetId(),
				IncludeContent: true,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if protoContentHasPlacement(hidden.Msg.GetContent(), targetID) {
				t.Fatalf("gated element appeared at density %d: %+v", tt.hiddenDensity, hidden.Msg.GetContent().GetPlacements())
			}
			if protoContentHasConnector(hidden.Msg.GetContent(), connectorID) {
				t.Fatalf("connector incident to gated element appeared at density %d: %+v", tt.hiddenDensity, hidden.Msg.GetContent().GetConnectors())
			}

			if err := store.SetViewDensityLevel(ctx, viewID, tt.visibleDensity); err != nil {
				t.Fatal(err)
			}
			visible, err := client.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
				ViewId:         view.Msg.GetView().GetId(),
				IncludeContent: true,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !protoContentHasPlacement(visible.Msg.GetContent(), targetID) {
				t.Fatalf("gated element missing at density %d: %+v", tt.visibleDensity, visible.Msg.GetContent().GetPlacements())
			}
			if !protoContentHasConnector(visible.Msg.GetContent(), connectorID) {
				t.Fatalf("connector incident to gated element missing at density %d: %+v", tt.visibleDensity, visible.Msg.GetContent().GetConnectors())
			}
		})
	}
}

func TestWorkspaceServiceSQLiteConnectorTagClearRoundTrip(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	client := newSQLiteWorkspaceClient(t)

	view, err := client.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		OrgId: orgID.String(),
		Name:  "Tag Clear",
	}))
	if err != nil {
		t.Fatal(err)
	}
	source, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{Name: "Source"}))
	if err != nil {
		t.Fatal(err)
	}
	target, err := client.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{Name: "Target"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, elementID := range []int32{source.Msg.GetElement().GetId(), target.Msg.GetElement().GetId()} {
		if _, err := client.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
			ViewId:    view.Msg.GetView().GetId(),
			ElementId: elementID,
		})); err != nil {
			t.Fatal(err)
		}
	}

	connector, err := client.CreateConnector(ctx, connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId:          view.Msg.GetView().GetId(),
		SourceElementId: source.Msg.GetElement().GetId(),
		TargetElementId: target.Msg.GetElement().GetId(),
		Direction:       "forward",
		Style:           "bezier",
		Tags:            []string{"runtime"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	connectorID := connector.Msg.GetConnector().GetId()

	withTags, err := client.UpdateConnector(ctx, connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId: connectorID,
		Tags:        []string{"critical", "edge"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := withTags.Msg.GetConnector().GetTags(); len(got) != 2 || got[0] != "critical" || got[1] != "edge" {
		t.Fatalf("connector tags = %#v, want [critical edge]", got)
	}

	cleared, err := client.UpdateConnector(ctx, connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId: connectorID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := cleared.Msg.GetConnector().GetTags(); len(got) != 0 {
		t.Fatalf("connector tags = %#v, want empty after clear", got)
	}
}

func newSQLiteWorkspaceClient(t *testing.T) diagv1connect.WorkspaceServiceClient {
	t.Helper()
	_, client := newSQLiteWorkspaceClientWithStore(t)
	return client
}

func newSQLiteWorkspaceClientWithStore(t *testing.T) (*app.Store, diagv1connect.WorkspaceServiceClient) {
	t.Helper()

	store, err := app.OpenStore(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	path, handler := diagv1connect.NewWorkspaceServiceHandler(&WorkspaceService{
		Store: NewAPIStore(store),
	})
	mux := http.NewServeMux()
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return store, diagv1connect.NewWorkspaceServiceClient(server.Client(), server.URL+"/api")
}

func workspaceHasView(views []*diagv1.View, id int32) bool {
	for _, view := range views {
		if view.GetId() == id || workspaceHasView(view.GetChildren(), id) {
			return true
		}
	}
	return false
}

func protoContentHasPlacement(content *diagv1.ViewContent, elementID int32) bool {
	for _, placement := range content.GetPlacements() {
		if placement.GetElementId() == elementID {
			return true
		}
	}
	return false
}

func protoContentHasConnector(content *diagv1.ViewContent, connectorID int32) bool {
	for _, connector := range content.GetConnectors() {
		if connector.GetId() == connectorID {
			return true
		}
	}
	return false
}

func ptr[T any](value T) *T {
	return &value
}
