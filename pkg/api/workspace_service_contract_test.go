package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
)

func TestWorkspaceService_ListElementsReturnsPaginationAndChecksRead(t *testing.T) {
	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	store := &contractStore{
		listElements: func(ctx context.Context, id uuid.UUID, limit, offset int32, search string) ([]*diagv1.Element, int, error) {
			if id != workspaceID {
				t.Fatalf("workspace id = %s, want %s", id, workspaceID)
			}
			if limit != 2 || offset != 4 || search != "api" {
				t.Fatalf("query = limit:%d offset:%d search:%q, want 2/4/api", limit, offset, search)
			}
			return []*diagv1.Element{{Id: 10, Name: "API"}}, 7, nil
		},
	}
	hooks := &recordingHooks{}
	service := &WorkspaceService{Store: store, Hooks: hooks}

	resp, err := service.ListElements(WithWorkspaceID(context.Background(), workspaceID), connect.NewRequest(&diagv1.ListElementsRequest{
		Limit:  2,
		Offset: 4,
		Search: "api",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Msg.GetPagination().GetTotalCount(); got != 7 {
		t.Fatalf("total count = %d, want 7", got)
	}
	if len(resp.Msg.GetElements()) != 1 || resp.Msg.GetElements()[0].GetId() != 10 {
		t.Fatalf("elements = %+v, want API element", resp.Msg.GetElements())
	}
	if strings.Join(hooks.events, ",") != "read" {
		t.Fatalf("hook events = %v, want read", hooks.events)
	}
}

func TestWorkspaceService_CreateConnectorDefaultsValidatesAndAudits(t *testing.T) {
	store := &contractStore{
		createConnector: func(_ context.Context, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			if input.ViewID != 3 || input.SourceID != 4 || input.TargetID != 5 {
				t.Fatalf("connector ids = %+v, want view/source/target 3/4/5", input)
			}
			if input.Direction != "forward" || input.Style != "bezier" {
				t.Fatalf("connector defaults = direction:%q style:%q, want forward/bezier", input.Direction, input.Style)
			}
			if input.Label == nil || *input.Label != "uses" {
				t.Fatalf("label = %v, want uses", input.Label)
			}
			return &diagv1.Connector{Id: 99, ViewId: 3, SourceElementId: 4, TargetElementId: 5, Direction: input.Direction, Style: input.Style, Label: input.Label}, nil
		},
	}
	hooks := &recordingHooks{}
	service := &WorkspaceService{Store: store, Hooks: hooks}

	resp, err := service.CreateConnector(context.Background(), connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId:          3,
		SourceElementId: 4,
		TargetElementId: 5,
		Label:           new("uses"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetConnector().GetId() != 99 {
		t.Fatalf("connector id = %d, want 99", resp.Msg.GetConnector().GetId())
	}
	if got := strings.Join(hooks.events, ","); got != "write:connectors,after:create:connector:99" {
		t.Fatalf("hook events = %s", got)
	}
}

func TestWorkspaceService_CreateConnectorRejectsInvalidStyleBeforeStoreWrite(t *testing.T) {
	store := &contractStore{
		createConnector: func(context.Context, uuid.UUID, ConnectorInput) (*diagv1.Connector, error) {
			t.Fatal("store should not be called for invalid style")
			return nil, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	_, err := service.CreateConnector(context.Background(), connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId:          3,
		SourceElementId: 4,
		TargetElementId: 5,
		Style:           "zigzag",
	}))
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("code = %s, want invalid_argument: %v", code, err)
	}
}

func TestWorkspaceService_UpdateElementClearsLogoWhenNoPrimaryIcon(t *testing.T) {
	var update ElementInput
	store := &contractStore{
		getElement: func(context.Context, int32, uuid.UUID) (*diagv1.Element, error) {
			return &diagv1.Element{
				Id:      42,
				Name:    "API",
				LogoUrl: new("https://example.com/logo.svg"),
				TechnologyLinks: []*diagv1.TechnologyLink{{
					Type:          "catalog",
					Label:         "Go",
					Slug:          new("go"),
					IsPrimaryIcon: true,
				}},
			}, nil
		},
		updateElement: func(_ context.Context, id int32, _ uuid.UUID, input ElementInput) (*diagv1.Element, error) {
			if id != 42 {
				t.Fatalf("id = %d, want 42", id)
			}
			update = input
			return &diagv1.Element{Id: id, Name: input.Name, LogoUrl: input.LogoURL, TechnologyLinks: input.TechLinks}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	resp, err := service.UpdateElement(context.Background(), connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId: 42,
		Name:      "API",
		TechnologyLinks: []*diagv1.TechnologyLink{{
			Type:  "catalog",
			Label: "Kafka",
			Slug:  new("kafka"),
		}},
		LogoUrl: new("https://example.com/kafka.svg"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if update.LogoURL == nil || *update.LogoURL != "" {
		t.Fatalf("update logo url = %v, want explicit empty string", update.LogoURL)
	}
	if got := resp.Msg.GetElement().GetLogoUrl(); got != "" {
		t.Fatalf("response logo url = %q, want cleared", got)
	}
}

func TestWorkspaceService_UpdateElementPreservesExistingTechnologyLinksWhenOmitted(t *testing.T) {
	existingLinks := []*diagv1.TechnologyLink{{
		Type:          "catalog",
		Label:         "Go",
		Slug:          new("go"),
		IsPrimaryIcon: true,
	}}
	var update ElementInput
	store := &contractStore{
		getElement: func(context.Context, int32, uuid.UUID) (*diagv1.Element, error) {
			return &diagv1.Element{Id: 42, Name: "API", LogoUrl: new("go.svg"), TechnologyLinks: existingLinks}, nil
		},
		updateElement: func(_ context.Context, id int32, _ uuid.UUID, input ElementInput) (*diagv1.Element, error) {
			update = input
			return &diagv1.Element{Id: id, Name: input.Name, LogoUrl: input.LogoURL, TechnologyLinks: input.TechLinks}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	_, err := service.UpdateElement(context.Background(), connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId: 42,
		Name:      "API",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if update.TechLinks != nil {
		t.Fatalf("tech link patch = %+v, want nil so store preserves existing links", update.TechLinks)
	}
	if update.LogoURL != nil {
		t.Fatalf("logo patch = %v, want nil so store preserves existing logo", update.LogoURL)
	}
}

func TestWorkspaceService_UpdateViewPreservesExistingLabelWhenOmitted(t *testing.T) {
	label := "Container"
	store := &contractStore{
		getView: func(context.Context, int32, uuid.UUID) (*diagv1.View, error) {
			return &diagv1.View{Id: 9, Name: "Old", LevelLabel: &label}, nil
		},
		updateView: func(_ context.Context, id int32, _ uuid.UUID, name string, gotLabel *string) (*diagv1.View, error) {
			if id != 9 || name != "New" {
				t.Fatalf("view update = id:%d name:%q, want 9/New", id, name)
			}
			if gotLabel == nil || *gotLabel != label {
				t.Fatalf("label = %v, want preserved %q", gotLabel, label)
			}
			return &diagv1.View{Id: id, Name: name, LevelLabel: gotLabel}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	resp, err := service.UpdateView(context.Background(), connect.NewRequest(&diagv1.UpdateViewRequest{
		ViewId: 9,
		Name:   "New",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetView().GetLevelLabel() != label {
		t.Fatalf("response label = %q, want %q", resp.Msg.GetView().GetLevelLabel(), label)
	}
}

func TestWorkspaceService_UpdateConnectorCanRestoreAllUndoableFields(t *testing.T) {
	existing := &diagv1.Connector{
		Id: 7, ViewId: 3, SourceElementId: 4, TargetElementId: 5,
		Label: new("changed"), Description: new("changed description"),
		Relationship: new("HTTP"), Direction: "forward", Style: "bezier",
		Url: new("https://example.com/changed"), SourceHandle: new("bottom"), TargetHandle: new("top"),
	}
	store := &contractStore{
		getConnector: func(context.Context, int32, uuid.UUID) (*diagv1.Connector, error) {
			return existing, nil
		},
		updateConnector: func(_ context.Context, id int32, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			if id != 7 {
				t.Fatalf("connector id = %d, want 7", id)
			}
			if input.ViewID != 3 || input.SourceID != 10 || input.TargetID != 11 {
				t.Fatalf("ids = %+v, want view 3 source 10 target 11", input)
			}
			if input.Label == nil || *input.Label != "reads" || input.Description == nil || *input.Description != "original description" || input.Relationship == nil || *input.Relationship != "SQL" || input.Direction != "both" || input.Style != "smoothstep" || input.URL == nil || *input.URL != "https://example.com/original" || input.SourceHandle == nil || *input.SourceHandle != "right" || input.TargetHandle == nil || *input.TargetHandle != "left" {
				t.Fatalf("connector restore input = %+v, want original fields", input)
			}
			return &diagv1.Connector{
				Id: id, ViewId: input.ViewID, SourceElementId: input.SourceID, TargetElementId: input.TargetID,
				Label: input.Label, Description: input.Description, Relationship: input.Relationship,
				Direction: input.Direction, Style: input.Style, Url: input.URL,
				SourceHandle: input.SourceHandle, TargetHandle: input.TargetHandle,
			}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	resp, err := service.UpdateConnector(context.Background(), connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId:     7,
		SourceElementId: new(int32(10)),
		TargetElementId: new(int32(11)),
		Label:           new("reads"),
		Description:     new("original description"),
		Relationship:    new("SQL"),
		Direction:       "both",
		Style:           "smoothstep",
		Url:             new("https://example.com/original"),
		SourceHandle:    new("right"),
		TargetHandle:    new("left"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetConnector().GetSourceElementId() != 10 || resp.Msg.GetConnector().GetTargetElementId() != 11 || resp.Msg.GetConnector().GetSourceHandle() != "right" || resp.Msg.GetConnector().GetTargetHandle() != "left" {
		t.Fatalf("connector response = %+v, want restored endpoints and handles", resp.Msg.GetConnector())
	}
}

func TestWorkspaceService_UpdateConnectorNormalizesLegacyStoredStyle(t *testing.T) {
	existing := &diagv1.Connector{
		Id: 7, ViewId: 3, SourceElementId: 4, TargetElementId: 5,
		Direction: "forward", Style: "solid",
	}
	store := &contractStore{
		getConnector: func(context.Context, int32, uuid.UUID) (*diagv1.Connector, error) {
			return existing, nil
		},
		updateConnector: func(_ context.Context, id int32, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			if id != 7 {
				t.Fatalf("connector id = %d, want 7", id)
			}
			if input.Style != "bezier" {
				t.Fatalf("connector style = %q, want bezier", input.Style)
			}
			return &diagv1.Connector{
				Id: id, ViewId: input.ViewID, SourceElementId: input.SourceID, TargetElementId: input.TargetID,
				Label: input.Label, Description: input.Description, Relationship: input.Relationship,
				Direction: input.Direction, Style: input.Style, Url: input.URL,
				SourceHandle: input.SourceHandle, TargetHandle: input.TargetHandle,
			}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	resp, err := service.UpdateConnector(context.Background(), connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId: 7,
		Label:       new("reads"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetConnector().GetStyle() != "bezier" {
		t.Fatalf("connector response style = %q, want bezier", resp.Msg.GetConnector().GetStyle())
	}
}

func TestWorkspaceService_CreateConnectorAcceptsDeleteUndoPayload(t *testing.T) {
	store := &contractStore{
		createConnector: func(_ context.Context, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			if input.ViewID != 3 || input.SourceID != 4 || input.TargetID != 5 || input.Label == nil || *input.Label != "reads" || input.Description == nil || *input.Description != "query path" || input.Relationship == nil || *input.Relationship != "SQL" || input.Direction != "both" || input.Style != "smoothstep" || input.URL == nil || *input.URL != "https://example.com/runbook" || input.SourceHandle == nil || *input.SourceHandle != "right" || input.TargetHandle == nil || *input.TargetHandle != "left" {
				t.Fatalf("create connector input = %+v, want full undo payload", input)
			}
			return &diagv1.Connector{Id: 99, ViewId: input.ViewID, SourceElementId: input.SourceID, TargetElementId: input.TargetID, Label: input.Label, Description: input.Description, Relationship: input.Relationship, Direction: input.Direction, Style: input.Style, Url: input.URL, SourceHandle: input.SourceHandle, TargetHandle: input.TargetHandle}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	if _, err := service.CreateConnector(context.Background(), connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId: 3, SourceElementId: 4, TargetElementId: 5,
		Label: new("reads"), Description: new("query path"), Relationship: new("SQL"),
		Direction: "both", Style: "smoothstep", Url: new("https://example.com/runbook"),
		SourceHandle: new("right"), TargetHandle: new("left"),
	})); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceService_PlacementDeleteCreateRestoresPosition(t *testing.T) {
	store := &contractStore{
		getView: func(context.Context, int32, uuid.UUID) (*diagv1.View, error) {
			return &diagv1.View{Id: 3}, nil
		},
		removePlacement: func(_ context.Context, viewID, elementID int32) error {
			if viewID != 3 || elementID != 4 {
				t.Fatalf("remove placement = view:%d element:%d, want 3/4", viewID, elementID)
			}
			return nil
		},
		addPlacement: func(_ context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error) {
			if viewID != 3 || elementID != 4 || x != 120 || y != 80 {
				t.Fatalf("add placement = view:%d element:%d pos:%f/%f, want 3/4/120/80", viewID, elementID, x, y)
			}
			return &diagv1.PlacedElement{Id: 77, ViewId: viewID, ElementId: elementID, PositionX: x, PositionY: y}, nil
		},
	}
	service := &WorkspaceService{Store: store, Hooks: &recordingHooks{}}

	if _, err := service.DeletePlacement(context.Background(), connect.NewRequest(&diagv1.DeletePlacementRequest{ViewId: 3, ElementId: 4})); err != nil {
		t.Fatal(err)
	}
	resp, err := service.CreatePlacement(context.Background(), connect.NewRequest(&diagv1.CreatePlacementRequest{ViewId: 3, ElementId: 4, PositionX: 120, PositionY: 80}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetPlacement().GetPositionX() != 120 || resp.Msg.GetPlacement().GetPositionY() != 80 {
		t.Fatalf("placement response = %+v, want restored position", resp.Msg.GetPlacement())
	}
}

func TestWorkspaceService_CreateViewLayerValidatesViewAndName(t *testing.T) {
	tests := []struct {
		name    string
		req     *diagv1.CreateViewLayerRequest
		store   *contractStore
		wantErr connect.Code
	}{
		{
			name: "missing view id",
			req:  &diagv1.CreateViewLayerRequest{Name: "Runtime"},
			store: &contractStore{
				getView: func(context.Context, int32, uuid.UUID) (*diagv1.View, error) {
					t.Fatal("store should not be called without a view id")
					return nil, nil
				},
			},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name: "unknown view",
			req:  &diagv1.CreateViewLayerRequest{ViewId: 7, Name: "Runtime"},
			store: &contractStore{
				getView: func(context.Context, int32, uuid.UUID) (*diagv1.View, error) {
					return nil, errors.New("missing")
				},
			},
			wantErr: connect.CodeNotFound,
		},
		{
			name: "blank name",
			req:  &diagv1.CreateViewLayerRequest{ViewId: 7, Name: "   "},
			store: &contractStore{
				getView: func(context.Context, int32, uuid.UUID) (*diagv1.View, error) {
					return &diagv1.View{Id: 7}, nil
				},
			},
			wantErr: connect.CodeInvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &WorkspaceService{Store: tt.store, Hooks: &recordingHooks{}}
			_, err := service.CreateViewLayer(context.Background(), connect.NewRequest(tt.req))
			if code := connect.CodeOf(err); code != tt.wantErr {
				t.Fatalf("code = %s, want %s: %v", code, tt.wantErr, err)
			}
		})
	}
}

type recordingHooks struct {
	NopWorkspaceHooks
	events []string
}

func (h *recordingHooks) CheckRead(context.Context, uuid.UUID) error {
	h.events = append(h.events, "read")
	return nil
}

func (h *recordingHooks) CheckWrite(_ context.Context, _ uuid.UUID, resourceType string) error {
	h.events = append(h.events, "write:"+resourceType)
	return nil
}

func (h *recordingHooks) AfterWrite(_ context.Context, _ uuid.UUID, action string, resourceType string, resourceID string, _ map[string]any, _ any) {
	h.events = append(h.events, "after:"+action+":"+resourceType+":"+resourceID)
}

type contractStore struct {
	listViews       func(context.Context, uuid.UUID) ([]*diagv1.View, error)
	listElements    func(context.Context, uuid.UUID, int32, int32, string) ([]*diagv1.Element, int, error)
	getElement      func(context.Context, int32, uuid.UUID) (*diagv1.Element, error)
	createView      func(context.Context, uuid.UUID, *int32, string, *string, bool) (*diagv1.View, error)
	updateElement   func(context.Context, int32, uuid.UUID, ElementInput) (*diagv1.Element, error)
	getView         func(context.Context, int32, uuid.UUID) (*diagv1.View, error)
	updateView      func(context.Context, int32, uuid.UUID, string, *string) (*diagv1.View, error)
	addPlacement    func(context.Context, int32, int32, float64, float64) (*diagv1.PlacedElement, error)
	removePlacement func(context.Context, int32, int32) error
	createConnector func(context.Context, uuid.UUID, ConnectorInput) (*diagv1.Connector, error)
	getConnector    func(context.Context, int32, uuid.UUID) (*diagv1.Connector, error)
	updateConnector func(context.Context, int32, uuid.UUID, ConnectorInput) (*diagv1.Connector, error)
}

var _ Store = (*contractStore)(nil)

func (s *contractStore) ListViews(context.Context, uuid.UUID) ([]*diagv1.View, error) {
	if s.listViews != nil {
		return s.listViews(context.Background(), uuid.Nil)
	}
	return nil, nil
}
func (s *contractStore) GetViews(context.Context, uuid.UUID, *int32, *bool, string, int, int) ([]*diagv1.View, int, error) {
	return nil, 0, nil
}
func (s *contractStore) GetView(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.View, error) {
	if s.getView != nil {
		return s.getView(ctx, id, workspaceID)
	}
	return &diagv1.View{Id: id}, nil
}
func (s *contractStore) CreateView(ctx context.Context, workspaceID uuid.UUID, ownerElementID *int32, name string, label *string, isRoot bool) (*diagv1.View, error) {
	if s.createView != nil {
		return s.createView(ctx, workspaceID, ownerElementID, name, label, isRoot)
	}
	return nil, nil
}
func (s *contractStore) UpdateView(ctx context.Context, id int32, workspaceID uuid.UUID, name string, label *string) (*diagv1.View, error) {
	if s.updateView != nil {
		return s.updateView(ctx, id, workspaceID, name, label)
	}
	return nil, nil
}
func (s *contractStore) DeleteView(context.Context, int32, uuid.UUID) error { return nil }
func (s *contractStore) ListElements(ctx context.Context, workspaceID uuid.UUID, limit, offset int32, search string) ([]*diagv1.Element, int, error) {
	if s.listElements != nil {
		return s.listElements(ctx, workspaceID, limit, offset, search)
	}
	return nil, 0, nil
}
func (s *contractStore) GetElement(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.Element, error) {
	if s.getElement != nil {
		return s.getElement(ctx, id, workspaceID)
	}
	return nil, errors.New("element not found")
}
func (s *contractStore) CreateElement(context.Context, uuid.UUID, ElementInput) (*diagv1.Element, error) {
	return nil, nil
}
func (s *contractStore) UpdateElement(ctx context.Context, id int32, workspaceID uuid.UUID, input ElementInput) (*diagv1.Element, error) {
	if s.updateElement != nil {
		return s.updateElement(ctx, id, workspaceID, input)
	}
	return nil, nil
}
func (s *contractStore) DeleteElement(context.Context, int32, uuid.UUID) error { return nil }
func (s *contractStore) ListPlacements(context.Context, int32) ([]*diagv1.PlacedElement, error) {
	return nil, nil
}
func (s *contractStore) ListAllPlacements(context.Context, uuid.UUID) ([]*diagv1.PlacedElement, error) {
	return nil, nil
}
func (s *contractStore) ListElementPlacements(context.Context, int32, uuid.UUID) ([]*diagv1.ViewPlacementInfo, error) {
	return nil, nil
}
func (s *contractStore) AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error) {
	if s.addPlacement != nil {
		return s.addPlacement(ctx, viewID, elementID, x, y)
	}
	return nil, nil
}
func (s *contractStore) UpdatePlacementPosition(context.Context, int32, int32, float64, float64) error {
	return nil
}
func (s *contractStore) RemovePlacement(ctx context.Context, viewID, elementID int32) error {
	if s.removePlacement != nil {
		return s.removePlacement(ctx, viewID, elementID)
	}
	return nil
}
func (s *contractStore) ListConnectors(context.Context, int32, uuid.UUID) ([]*diagv1.Connector, error) {
	return nil, nil
}
func (s *contractStore) ListAllConnectors(context.Context, uuid.UUID) ([]*diagv1.Connector, error) {
	return nil, nil
}
func (s *contractStore) GetConnector(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.Connector, error) {
	if s.getConnector != nil {
		return s.getConnector(ctx, id, workspaceID)
	}
	return nil, nil
}
func (s *contractStore) CreateConnector(ctx context.Context, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
	if s.createConnector != nil {
		return s.createConnector(ctx, workspaceID, input)
	}
	return nil, nil
}
func (s *contractStore) UpdateConnector(ctx context.Context, id int32, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
	if s.updateConnector != nil {
		return s.updateConnector(ctx, id, workspaceID, input)
	}
	return nil, nil
}
func (s *contractStore) DeleteConnector(context.Context, int32, uuid.UUID) error { return nil }
func (s *contractStore) ListElementNavigations(context.Context, uuid.UUID, int32) ([]*diagv1.ElementNavigationInfo, error) {
	return nil, nil
}
func (s *contractStore) ListIncomingElementNavigations(context.Context, int32) ([]*diagv1.IncomingElementNavigationInfo, error) {
	return nil, nil
}
func (s *contractStore) ListViewLayers(context.Context, int32) ([]*diagv1.ViewLayer, error) {
	return nil, nil
}
func (s *contractStore) ListAllViewLayers(context.Context, uuid.UUID) ([]*diagv1.ViewLayer, error) {
	return nil, nil
}
func (s *contractStore) GetViewLayer(context.Context, int32) (*diagv1.ViewLayer, error) {
	return nil, nil
}
func (s *contractStore) CreateViewLayer(context.Context, int32, string, []string, string) (*diagv1.ViewLayer, error) {
	return nil, nil
}
func (s *contractStore) UpdateViewLayer(context.Context, int32, *string, []string, *string) (*diagv1.ViewLayer, error) {
	return nil, nil
}
func (s *contractStore) DeleteViewLayer(context.Context, int32) error { return nil }
func (s *contractStore) Tags(context.Context, uuid.UUID) (map[string]*diagv1.Tag, error) {
	return nil, nil
}
func (s *contractStore) UpdateTag(context.Context, uuid.UUID, string, string, *string) error {
	return nil
}
func (s *contractStore) ApplyPlan(context.Context, uuid.UUID, *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
	return nil, nil
}
func (s *contractStore) ListVersions(context.Context, uuid.UUID, int) ([]*diagv1.WorkspaceVersionInfo, error) {
	return nil, nil
}
func (s *contractStore) GetLatestVersion(context.Context, uuid.UUID) (*diagv1.WorkspaceVersionInfo, error) {
	return nil, nil
}
func (s *contractStore) CreateVersion(context.Context, uuid.UUID, string, string, *int32, int, int, int, *string, *string) (*diagv1.WorkspaceVersionInfo, error) {
	return nil, nil
}
func (s *contractStore) GetVersioningEnabled(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *contractStore) SetVersioningEnabled(context.Context, uuid.UUID, bool) error { return nil }
func (s *contractStore) GetWorkspaceResourceCounts(context.Context, uuid.UUID) (int, int, int, error) {
	return 0, 0, 0, nil
}
