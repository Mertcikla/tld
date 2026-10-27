package api

import (
	"context"
	"fmt"
	"slices"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
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
	requestElement := &diagv1.BatchElement{Ref: "api", Name: "API"}

	_, err := service.ImportResources(context.Background(), connect.NewRequest(&diagv1.ImportResourcesRequest{
		OrgId:    workspaceID.String(),
		Elements: []*diagv1.BatchElement{requestElement},
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
		Elements: []*diagv1.BatchElement{
			{Ref: "api", Name: "API", Kind: ptr("service"), Placements: []*diagv1.BatchViewPlacement{{ParentRef: "root"}}},
			{Ref: "db", Name: "DB", Kind: ptr("database"), Placements: []*diagv1.BatchViewPlacement{{ParentRef: "root"}}},
		},
		Connectors: []*diagv1.BatchConnector{
			{Ref: "api:db", ViewRef: "root", SourceElementRef: "api", TargetElementRef: "db", Label: ptr(label), Tags: []string{"feat"}},
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
	if tags := content.GetConnectors()[0].GetTags(); !slices.Equal(tags, []string{"feat"}) {
		t.Fatalf("connector tags = %v, want [feat]", tags)
	}
	positions := map[string]bool{}
	for _, placement := range content.GetPlacements() {
		positions[fmt.Sprintf("%.0f:%.0f", placement.GetPositionX(), placement.GetPositionY())] = true
	}
	if len(positions) != 2 {
		t.Fatalf("auto-layout positions not distinct: %v", positions)
	}
}

func TestImportResourcesConnectorTagPatches(t *testing.T) {
	for _, id := range []int32{0, 42} {
		for _, tc := range []struct {
			name    string
			planned *diagv1.BatchConnector
			want    []string
		}{
			{"omitted", &diagv1.BatchConnector{}, nil},
			{"replace", &diagv1.BatchConnector{Tags: []string{"feat"}}, []string{"feat"}},
			{"clear", &diagv1.BatchConnector{ReplaceTags: true}, []string{}},
		} {
			t.Run(fmt.Sprintf("id=%d/%s", id, tc.name), func(t *testing.T) {
				planned := tc.planned
				planned.Id, planned.ViewRef = &id, "root"
				planned.SourceElementRef, planned.TargetElementRef = "api", "db"
				data, err := proto.Marshal(planned)
				if err != nil {
					t.Fatal(err)
				}
				decoded := &diagv1.BatchConnector{}
				if err := proto.Unmarshal(data, decoded); err != nil {
					t.Fatal(err)
				}
				called := false
				check := func(input ConnectorInput) (*diagv1.Connector, error) {
					called = true
					if (input.Tags == nil) != (tc.want == nil) || !slices.Equal(input.Tags, tc.want) {
						t.Fatalf("tags = %#v, want %#v", input.Tags, tc.want)
					}
					return &diagv1.Connector{Id: 42}, nil
				}
				service := &ImportService{Store: &contractStore{
					createConnector: func(_ context.Context, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
						if id != 0 {
							t.Fatal("created instead of updating")
						}
						return check(input)
					},
					updateConnector: func(_ context.Context, _ int32, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
						if id == 0 {
							t.Fatal("updated instead of creating")
						}
						return check(input)
					},
				}}
				var writes []pendingWrite
				if err := service.addImportedConnectors(context.Background(), uuid.Nil, []*diagv1.BatchConnector{decoded}, map[string]int32{"api": 1, "db": 2}, map[string]int32{"root": 1}, &writes); err != nil {
					t.Fatal(err)
				}
				if !called {
					t.Fatal("connector mutation was not called")
				}
			})
		}
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
