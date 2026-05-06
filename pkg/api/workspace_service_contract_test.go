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
		Label:           strPtr("uses"),
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
				LogoUrl: strPtr("https://example.com/logo.svg"),
				TechnologyLinks: []*diagv1.TechnologyLink{{
					Type:          "catalog",
					Label:         "Go",
					Slug:          strPtr("go"),
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
			Slug:  strPtr("kafka"),
		}},
		LogoUrl: strPtr("https://example.com/kafka.svg"),
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
		Slug:          strPtr("go"),
		IsPrimaryIcon: true,
	}}
	var update ElementInput
	store := &contractStore{
		getElement: func(context.Context, int32, uuid.UUID) (*diagv1.Element, error) {
			return &diagv1.Element{Id: 42, Name: "API", LogoUrl: strPtr("go.svg"), TechnologyLinks: existingLinks}, nil
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

func strPtr(value string) *string {
	return &value
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
	listElements    func(context.Context, uuid.UUID, int32, int32, string) ([]*diagv1.Element, int, error)
	getElement      func(context.Context, int32, uuid.UUID) (*diagv1.Element, error)
	updateElement   func(context.Context, int32, uuid.UUID, ElementInput) (*diagv1.Element, error)
	getView         func(context.Context, int32, uuid.UUID) (*diagv1.View, error)
	createConnector func(context.Context, uuid.UUID, ConnectorInput) (*diagv1.Connector, error)
}

var _ Store = (*contractStore)(nil)

func (s *contractStore) ListViews(context.Context, uuid.UUID) ([]*diagv1.View, error) {
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
func (s *contractStore) CreateView(context.Context, uuid.UUID, *int32, string, *string, bool) (*diagv1.View, error) {
	return nil, nil
}
func (s *contractStore) UpdateView(context.Context, int32, uuid.UUID, string, *string) (*diagv1.View, error) {
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
func (s *contractStore) AddPlacement(context.Context, int32, int32, float64, float64) (*diagv1.PlacedElement, error) {
	return nil, nil
}
func (s *contractStore) UpdatePlacementPosition(context.Context, int32, int32, float64, float64) error {
	return nil
}
func (s *contractStore) RemovePlacement(context.Context, int32, int32) error { return nil }
func (s *contractStore) ListConnectors(context.Context, int32, uuid.UUID) ([]*diagv1.Connector, error) {
	return nil, nil
}
func (s *contractStore) ListAllConnectors(context.Context, uuid.UUID) ([]*diagv1.Connector, error) {
	return nil, nil
}
func (s *contractStore) GetConnector(context.Context, int32, uuid.UUID) (*diagv1.Connector, error) {
	return nil, nil
}
func (s *contractStore) CreateConnector(ctx context.Context, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
	if s.createConnector != nil {
		return s.createConnector(ctx, workspaceID, input)
	}
	return nil, nil
}
func (s *contractStore) UpdateConnector(context.Context, int32, uuid.UUID, ConnectorInput) (*diagv1.Connector, error) {
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
