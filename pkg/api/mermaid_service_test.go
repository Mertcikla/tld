package api

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func TestMermaidServiceParseMermaid(t *testing.T) {
	t.Parallel()

	service := &MermaidService{}
	resp, err := service.ParseMermaid(context.Background(), connect.NewRequest(&diagv1.ParseMermaidRequest{
		Source: "flowchart LR\n  A[API] -->|reads| B[DB]",
	}))
	if err != nil {
		t.Fatalf("ParseMermaid() error = %v", err)
	}
	if len(resp.Msg.GetElements()) != 2 {
		t.Fatalf("ParseMermaid() elements = %d, want 2", len(resp.Msg.GetElements()))
	}
	if len(resp.Msg.GetConnectors()) != 1 {
		t.Fatalf("ParseMermaid() connectors = %d, want 1", len(resp.Msg.GetConnectors()))
	}
	if got := resp.Msg.GetDirection(); got != diagv1.MermaidDirection_MERMAID_DIRECTION_LR {
		t.Fatalf("ParseMermaid() direction = %v", got)
	}
}

func TestMermaidServiceParseMermaidFontAwesomeNodeLabel(t *testing.T) {
	t.Parallel()

	service := &MermaidService{}
	resp, err := service.ParseMermaid(context.Background(), connect.NewRequest(&diagv1.ParseMermaidRequest{
		Source: "flowchart LR\n  F[fa:fa-car Car]",
	}))
	if err != nil {
		t.Fatalf("ParseMermaid() error = %v", err)
	}
	if len(resp.Msg.GetElements()) != 1 {
		t.Fatalf("ParseMermaid() elements = %d, want 1", len(resp.Msg.GetElements()))
	}
	element := resp.Msg.GetElements()[0]
	if element.GetName() != "Car" {
		t.Fatalf("element name = %q, want Car", element.GetName())
	}
	links := element.GetTechnologyLinks()
	if len(links) != 1 {
		t.Fatalf("technology links = %d, want 1", len(links))
	}
	if links[0].GetType() != "custom" || links[0].GetLabel() != "fa:fa-car" || !links[0].GetIsPrimaryIcon() {
		t.Fatalf("technology link = %+v, want primary custom fa:fa-car", links[0])
	}
}

func TestMermaidServiceImportMermaidFontAwesomeNodeLabel(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	var createdInput ElementInput
	transactionCalled := false
	var store *contractStore
	store = &contractStore{
		runInTransaction: func(ctx context.Context, fn func(context.Context, Store) error) error {
			transactionCalled = true
			return fn(ctx, store)
		},
		createElement: func(_ context.Context, id uuid.UUID, input ElementInput) (*diagv1.Element, error) {
			if id != workspaceID {
				t.Fatalf("workspace id = %s, want %s", id, workspaceID)
			}
			createdInput = input
			return &diagv1.Element{
				Id:              10,
				Name:            input.Name,
				TechnologyLinks: input.TechLinks,
			}, nil
		},
	}
	service := &MermaidService{Store: store}

	_, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(&diagv1.ImportMermaidIntoViewRequest{
		OrgId:  workspaceID.String(),
		ViewId: 7,
		Source: "flowchart LR\n  F[fa:fa-car Car]",
		DryRun: false,
	}))
	if err != nil {
		t.Fatalf("ImportMermaidIntoView() error = %v", err)
	}
	if !transactionCalled {
		t.Fatal("ImportMermaidIntoView() did not run inside a transaction")
	}
	if createdInput.Name != "Car" {
		t.Fatalf("created name = %q, want Car", createdInput.Name)
	}
	links := createdInput.TechLinks
	if len(links) != 1 {
		t.Fatalf("created technology links = %d, want 1", len(links))
	}
	if links[0].GetType() != "custom" || links[0].GetLabel() != "fa:fa-car" || !links[0].GetIsPrimaryIcon() {
		t.Fatalf("created technology link = %+v, want primary custom fa:fa-car", links[0])
	}
	if createdInput.Technology != nil {
		t.Fatalf("created technology = %v, want nil", createdInput.Technology)
	}
	if createdInput.LogoURL != nil {
		t.Fatalf("created logo url = %v, want nil", createdInput.LogoURL)
	}
}

func TestMermaidServiceImportMermaidCreatesReverseConnector(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	var createdInput ConnectorInput
	store := &contractStore{
		getElement: func(_ context.Context, id int32, _ uuid.UUID) (*diagv1.Element, error) {
			return &diagv1.Element{Id: id, Name: map[int32]string{1: "A", 2: "B"}[id]}, nil
		},
		listConnectors: func(context.Context, int32, uuid.UUID) ([]*diagv1.Connector, error) {
			label := "calls"
			return []*diagv1.Connector{{
				Id:              10,
				ViewId:          7,
				SourceElementId: 1,
				TargetElementId: 2,
				Label:           &label,
				Direction:       "forward",
				Style:           "bezier",
			}}, nil
		},
		createConnector: func(_ context.Context, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			createdInput = input
			return &diagv1.Connector{
				Id:              11,
				ViewId:          input.ViewID,
				SourceElementId: input.SourceID,
				TargetElementId: input.TargetID,
				Label:           input.Label,
				Direction:       input.Direction,
				Style:           input.Style,
			}, nil
		},
	}
	service := &MermaidService{Store: store}

	resp, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(&diagv1.ImportMermaidIntoViewRequest{
		OrgId:  workspaceID.String(),
		ViewId: 7,
		Source: `flowchart LR
  node_1["A"]
  %% tld-element ref=node_1
  node_2["B"]
  %% tld-element ref=node_2
  node_2 -- "calls" --> node_1`,
	}))
	if err != nil {
		t.Fatalf("ImportMermaidIntoView() error = %v", err)
	}
	if createdInput.SourceID != 2 || createdInput.TargetID != 1 {
		t.Fatalf("created connector endpoints = %d -> %d, want 2 -> 1", createdInput.SourceID, createdInput.TargetID)
	}
	if got := resp.Msg.GetSummary().GetCreatedConnectorCount(); got != 1 {
		t.Fatalf("created connector count = %d, want 1", got)
	}
	if got := resp.Msg.GetSummary().GetResolvedConnectorCount(); got != 0 {
		t.Fatalf("resolved connector count = %d, want 0", got)
	}
}

func TestMermaidServiceImportResolvesWorkspaceConnectorProjectedIntoTargetView(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	addedPlacements := map[int32]struct{}{}
	label := "SQL reads"
	description := "Read path"
	relationship := "SQL"
	url := "https://example.test/runbook"
	sourceHandle := "bottom"
	targetHandle := "top"
	store := &contractStore{
		getElement: func(_ context.Context, id int32, _ uuid.UUID) (*diagv1.Element, error) {
			switch id {
			case 1:
				return &diagv1.Element{Id: 1, Name: "API"}, nil
			case 2:
				return &diagv1.Element{Id: 2, Name: "DB"}, nil
			default:
				return nil, sql.ErrNoRows
			}
		},
		addPlacement: func(_ context.Context, viewID, elementID int32, _, _ float64) (*diagv1.PlacedElement, error) {
			if viewID != 7 {
				t.Fatalf("placement view id = %d, want 7", viewID)
			}
			addedPlacements[elementID] = struct{}{}
			return &diagv1.PlacedElement{Id: elementID + 100, ViewId: viewID, ElementId: elementID}, nil
		},
		listAllConnectors: func(context.Context, uuid.UUID) ([]*diagv1.Connector, error) {
			return []*diagv1.Connector{{
				Id: 55, ViewId: 3, SourceElementId: 1, TargetElementId: 2,
				Label: &label, Description: &description, Relationship: &relationship,
				Direction: "both", Style: "smoothstep", Url: &url,
				SourceHandle: &sourceHandle, TargetHandle: &targetHandle,
			}}, nil
		},
		createConnector: func(context.Context, uuid.UUID, ConnectorInput) (*diagv1.Connector, error) {
			t.Fatal("import should reuse the workspace connector projected through matching endpoints")
			return nil, nil
		},
	}
	service := &MermaidService{Store: store}

	resp, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(&diagv1.ImportMermaidIntoViewRequest{
		OrgId:  workspaceID.String(),
		ViewId: 7,
		Source: `flowchart LR
%% tld/v1 view=3
  node_1["API"]
%% tld-element ref=node_1 x=120 y=80
  node_2["DB"]
%% tld-element ref=node_2 x=460 y=240
  node_1 -- "SQL reads" --> node_2
%% tld-connector ref=99 source=node_1 target=node_2 label=SQL reads desc=Read path rel=SQL dir=both style=smoothstep url=https://example.test/runbook sourceHandle=bottom targetHandle=top`,
	}))
	if err != nil {
		t.Fatalf("ImportMermaidIntoView() error = %v", err)
	}
	summary := resp.Msg.GetSummary()
	if summary.GetResolvedElementCount() != 2 || summary.GetCreatedElementCount() != 0 || summary.GetResolvedConnectorCount() != 1 || summary.GetCreatedConnectorCount() != 0 {
		t.Fatalf("summary = %+v, want existing elements and workspace connector resolved", summary)
	}
	if _, ok := addedPlacements[1]; !ok {
		t.Fatal("API placement was not added to target view")
	}
	if _, ok := addedPlacements[2]; !ok {
		t.Fatal("DB placement was not added to target view")
	}
}

func TestMermaidServiceImportPreservesTldMetadata(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	createdElements := map[string]ElementInput{}
	placements := map[int32]struct{ x, y float64 }{}
	var createdViewOwner int32
	var createdViewLabel string
	var createdConnector ConnectorInput
	nextElementID := int32(10)

	store := &contractStore{
		getElement: func(context.Context, int32, uuid.UUID) (*diagv1.Element, error) {
			return nil, sql.ErrNoRows
		},
		createElement: func(_ context.Context, _ uuid.UUID, input ElementInput) (*diagv1.Element, error) {
			createdElements[input.Name] = input
			id := nextElementID
			nextElementID++
			return elementFromInput(id, input), nil
		},
		createView: func(_ context.Context, _ uuid.UUID, ownerElementID *int32, _ string, label *string, _ bool) (*diagv1.View, error) {
			if ownerElementID == nil {
				t.Fatal("internal view owner element id = nil")
			}
			createdViewOwner = *ownerElementID
			createdViewLabel = derefString(label)
			return &diagv1.View{Id: 99, OwnerElementId: ownerElementID, LevelLabel: label}, nil
		},
		addPlacement: func(_ context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error) {
			if viewID != 7 {
				t.Fatalf("placement view id = %d, want 7", viewID)
			}
			placements[elementID] = struct{ x, y float64 }{x: x, y: y}
			return &diagv1.PlacedElement{Id: elementID + 100, ViewId: viewID, ElementId: elementID, PositionX: x, PositionY: y}, nil
		},
		createConnector: func(_ context.Context, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			createdConnector = input
			return &diagv1.Connector{
				Id: 77, ViewId: input.ViewID, SourceElementId: input.SourceID, TargetElementId: input.TargetID,
				Label: input.Label, Description: input.Description, Relationship: input.Relationship,
				Direction: input.Direction, Style: input.Style, Url: input.URL,
				SourceHandle: input.SourceHandle, TargetHandle: input.TargetHandle,
			}, nil
		},
	}
	service := &MermaidService{Store: store}

	resp, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(&diagv1.ImportMermaidIntoViewRequest{
		OrgId:   workspaceID.String(),
		ViewId:  7,
		Source:  tldMetadataRoundTripSource(),
		CenterX: 900,
		CenterY: 700,
	}))
	if err != nil {
		t.Fatalf("ImportMermaidIntoView() error = %v", err)
	}
	summary := resp.Msg.GetSummary()
	if summary.GetCreatedElementCount() != 2 || summary.GetCreatedConnectorCount() != 1 {
		t.Fatalf("summary = %+v, want 2 created elements and 1 connector", summary)
	}

	api := createdElements["QA API"]
	if api.Name != "QA API" || derefString(api.Kind) != "service" || derefString(api.Description) != "Handles checkout" || derefString(api.Technology) != "Go" || derefString(api.URL) != "https://example.test/api" || derefString(api.LogoURL) != "/icons/go.svg" {
		t.Fatalf("API element metadata = %+v, want name/kind/description/technology/source fields", api)
	}
	if !slices.Equal(api.Tags, []string{"backend", "round-trip"}) {
		t.Fatalf("API tags = %v, want backend and round-trip", api.Tags)
	}
	if len(api.TechLinks) != 1 || api.TechLinks[0].GetType() != "catalog" || api.TechLinks[0].GetSlug() != "go" || api.TechLinks[0].GetLabel() != "Go" || !api.TechLinks[0].GetIsPrimaryIcon() {
		t.Fatalf("API technology links = %+v, want primary Go catalog link", api.TechLinks)
	}
	if derefString(api.Repo) != "github.com/example/shop" || derefString(api.Branch) != "main" || derefString(api.FilePath) != "cmd/api/main.go" || derefString(api.Language) != "go" {
		t.Fatalf("API source link metadata = repo:%v branch:%v file:%v language:%v", api.Repo, api.Branch, api.FilePath, api.Language)
	}
	if api.BypassNoiseGate == nil || !*api.BypassNoiseGate || !api.HasView || derefString(api.ViewLabel) != "Component" {
		t.Fatalf("API view flags = bypass:%v hasView:%v viewLabel:%v", api.BypassNoiseGate, api.HasView, api.ViewLabel)
	}
	if createdViewOwner != 10 || createdViewLabel != "Component" {
		t.Fatalf("created internal view = owner:%d label:%q, want owner 10 label Component", createdViewOwner, createdViewLabel)
	}
	if placement := placements[10]; placement.x != 120 || placement.y != 80 {
		t.Fatalf("API placement = %+v, want exact metadata position 120/80", placement)
	}
	if placement := placements[11]; placement.x != 460 || placement.y != 240 {
		t.Fatalf("DB placement = %+v, want exact metadata position 460/240", placement)
	}
	if createdConnector.ViewID != 7 || createdConnector.SourceID != 10 || createdConnector.TargetID != 11 || derefString(createdConnector.Label) != "SQL reads" || derefString(createdConnector.Description) != "Read path" || derefString(createdConnector.Relationship) != "SQL" || createdConnector.Direction != "both" || createdConnector.Style != "smoothstep" || derefString(createdConnector.URL) != "https://example.test/runbook" || derefString(createdConnector.SourceHandle) != "bottom" || derefString(createdConnector.TargetHandle) != "top" {
		t.Fatalf("connector metadata = %+v, want full route/handle/details", createdConnector)
	}
}

func TestMermaidServiceReimportResolvesExistingMetadataPlacements(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	memory := newMermaidImportMemory(false)
	service := &MermaidService{Store: memory.store()}
	req := &diagv1.ImportMermaidIntoViewRequest{
		OrgId:   workspaceID.String(),
		ViewId:  7,
		Source:  tldMetadataRoundTripSource(),
		CenterX: 900,
		CenterY: 700,
	}

	first, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("first ImportMermaidIntoView() error = %v", err)
	}
	if first.Msg.GetSummary().GetCreatedElementCount() != 2 || first.Msg.GetSummary().GetCreatedConnectorCount() != 1 {
		t.Fatalf("first summary = %+v, want created elements/connectors", first.Msg.GetSummary())
	}
	second, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("second ImportMermaidIntoView() error = %v", err)
	}
	if second.Msg.GetSummary().GetCreatedElementCount() != 0 || second.Msg.GetSummary().GetCreatedConnectorCount() != 0 || second.Msg.GetSummary().GetResolvedElementCount() != 2 || second.Msg.GetSummary().GetResolvedConnectorCount() != 1 {
		t.Fatalf("second summary = %+v, want existing metadata-backed resources resolved", second.Msg.GetSummary())
	}
	if len(memory.state.elements) != 2 || len(memory.state.placements) != 2 || len(memory.state.connectors) != 1 {
		t.Fatalf("stored state = %d elements/%d placements/%d connectors, want 2/2/1", len(memory.state.elements), len(memory.state.placements), len(memory.state.connectors))
	}
}

func TestMermaidServiceFailedImportRollsBackPartialResources(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	memory := newMermaidImportMemory(true)
	service := &MermaidService{Store: memory.store()}

	_, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(&diagv1.ImportMermaidIntoViewRequest{
		OrgId:  workspaceID.String(),
		ViewId: 7,
		Source: tldMetadataRoundTripSource(),
	}))
	if err == nil {
		t.Fatal("ImportMermaidIntoView() error = nil, want CreateView failure")
	}
	if len(memory.state.elements) != 0 || len(memory.state.placements) != 0 || len(memory.state.connectors) != 0 {
		t.Fatalf("state after failed import = %d elements/%d placements/%d connectors, want no partial resources", len(memory.state.elements), len(memory.state.placements), len(memory.state.connectors))
	}
}

func TestMermaidServiceDryRunCountsConnectorsBetweenNewElements(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	store := &contractStore{
		createElement: func(context.Context, uuid.UUID, ElementInput) (*diagv1.Element, error) {
			t.Fatal("dry-run should not create elements")
			return nil, nil
		},
		createConnector: func(context.Context, uuid.UUID, ConnectorInput) (*diagv1.Connector, error) {
			t.Fatal("dry-run should not create connectors")
			return nil, nil
		},
	}
	service := &MermaidService{Store: store}

	resp, err := service.ImportMermaidIntoView(context.Background(), connect.NewRequest(&diagv1.ImportMermaidIntoViewRequest{
		OrgId:  workspaceID.String(),
		ViewId: 7,
		Source: "flowchart LR\n  A[API] -->|reads| B[DB]",
		DryRun: true,
	}))
	if err != nil {
		t.Fatalf("ImportMermaidIntoView() error = %v", err)
	}
	summary := resp.Msg.GetSummary()
	if got := summary.GetCreatedElementCount(); got != 2 {
		t.Fatalf("created element count = %d, want 2", got)
	}
	if got := summary.GetCreatedConnectorCount(); got != 1 {
		t.Fatalf("created connector count = %d, want 1", got)
	}
}

func TestMermaidServiceParseMermaidInvalidArgument(t *testing.T) {
	t.Parallel()

	service := &MermaidService{}
	_, err := service.ParseMermaid(context.Background(), connect.NewRequest(&diagv1.ParseMermaidRequest{}))
	if err == nil {
		t.Fatal("ParseMermaid(empty) error = nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ParseMermaid(empty) code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func tldMetadataRoundTripSource() string {
	return `flowchart LR
%% tld/v1 view=42
  node_9001["QA API"]
%% tld-element ref=node_9001 x=120 y=80 kind=service desc=Handles checkout tech=Go url=https://example.test/api logo=/icons/go.svg tags=backend,round-trip techLinks=catalog:go:Go:1 repo=github.com/example/shop branch=main file=cmd/api/main.go lang=go bypass=1 hasView=1 viewLabel=Component
  node_9002["QA DB"]
%% tld-element ref=node_9002 x=460 y=240 kind=database tech=Postgres tags=data techLinks=catalog:postgresql:PostgreSQL:1
  node_9001 -- "SQL reads" --> node_9002
%% tld-connector ref=9901 source=node_9001 target=node_9002 label=SQL reads desc=Read path rel=SQL dir=both style=smoothstep url=https://example.test/runbook sourceHandle=bottom targetHandle=top`
}

type mermaidImportMemory struct {
	state          *mermaidImportState
	failCreateView bool
}

type mermaidImportState struct {
	nextElementID   int32
	nextPlacementID int32
	nextConnectorID int32
	nextViewID      int32
	elements        map[int32]*diagv1.Element
	placements      map[int32]*diagv1.PlacedElement
	connectors      map[int32]*diagv1.Connector
}

func newMermaidImportMemory(failCreateView bool) *mermaidImportMemory {
	return &mermaidImportMemory{
		failCreateView: failCreateView,
		state: &mermaidImportState{
			nextElementID:   10,
			nextPlacementID: 100,
			nextConnectorID: 200,
			nextViewID:      300,
			elements:        map[int32]*diagv1.Element{},
			placements:      map[int32]*diagv1.PlacedElement{},
			connectors:      map[int32]*diagv1.Connector{},
		},
	}
}

func (m *mermaidImportMemory) store() *contractStore {
	return &contractStore{
		runInTransaction: func(ctx context.Context, fn func(context.Context, Store) error) error {
			tx := &mermaidImportMemory{state: m.state.clone(), failCreateView: m.failCreateView}
			if err := fn(ctx, tx.store()); err != nil {
				return err
			}
			m.state = tx.state
			return nil
		},
		getElement: func(_ context.Context, id int32, _ uuid.UUID) (*diagv1.Element, error) {
			if element := m.state.elements[id]; element != nil {
				return proto.Clone(element).(*diagv1.Element), nil
			}
			return nil, sql.ErrNoRows
		},
		createElement: func(_ context.Context, _ uuid.UUID, input ElementInput) (*diagv1.Element, error) {
			id := m.state.nextElementID
			m.state.nextElementID++
			element := elementFromInput(id, input)
			m.state.elements[id] = element
			return proto.Clone(element).(*diagv1.Element), nil
		},
		createView: func(context.Context, uuid.UUID, *int32, string, *string, bool) (*diagv1.View, error) {
			if m.failCreateView {
				return nil, errors.New("create internal view failed")
			}
			id := m.state.nextViewID
			m.state.nextViewID++
			return &diagv1.View{Id: id}, nil
		},
		listPlacements: func(context.Context, int32) ([]*diagv1.PlacedElement, error) {
			out := make([]*diagv1.PlacedElement, 0, len(m.state.placements))
			for _, placement := range m.state.placements {
				out = append(out, proto.Clone(placement).(*diagv1.PlacedElement))
			}
			return out, nil
		},
		addPlacement: func(_ context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error) {
			element := m.state.elements[elementID]
			if element == nil {
				return nil, sql.ErrNoRows
			}
			id := m.state.nextPlacementID
			m.state.nextPlacementID++
			placement := placedElementFromElement(id, viewID, element, x, y)
			m.state.placements[elementID] = placement
			return proto.Clone(placement).(*diagv1.PlacedElement), nil
		},
		listConnectors: func(context.Context, int32, uuid.UUID) ([]*diagv1.Connector, error) {
			out := make([]*diagv1.Connector, 0, len(m.state.connectors))
			for _, connector := range m.state.connectors {
				out = append(out, proto.Clone(connector).(*diagv1.Connector))
			}
			return out, nil
		},
		createConnector: func(_ context.Context, _ uuid.UUID, input ConnectorInput) (*diagv1.Connector, error) {
			id := m.state.nextConnectorID
			m.state.nextConnectorID++
			connector := &diagv1.Connector{
				Id: id, ViewId: input.ViewID, SourceElementId: input.SourceID, TargetElementId: input.TargetID,
				Label: input.Label, Description: input.Description, Relationship: input.Relationship,
				Direction: input.Direction, Style: input.Style, Url: input.URL,
				SourceHandle: input.SourceHandle, TargetHandle: input.TargetHandle,
			}
			m.state.connectors[id] = connector
			return proto.Clone(connector).(*diagv1.Connector), nil
		},
		getProjectedViewContent: func(context.Context, int32, uuid.UUID, *int32) (*diagv1.ViewContent, error) {
			content := &diagv1.ViewContent{}
			for _, placement := range m.state.placements {
				content.Placements = append(content.Placements, proto.Clone(placement).(*diagv1.PlacedElement))
			}
			for _, connector := range m.state.connectors {
				content.Connectors = append(content.Connectors, proto.Clone(connector).(*diagv1.Connector))
			}
			return content, nil
		},
	}
}

func (s *mermaidImportState) clone() *mermaidImportState {
	out := &mermaidImportState{
		nextElementID:   s.nextElementID,
		nextPlacementID: s.nextPlacementID,
		nextConnectorID: s.nextConnectorID,
		nextViewID:      s.nextViewID,
		elements:        map[int32]*diagv1.Element{},
		placements:      map[int32]*diagv1.PlacedElement{},
		connectors:      map[int32]*diagv1.Connector{},
	}
	for id, element := range s.elements {
		out.elements[id] = proto.Clone(element).(*diagv1.Element)
	}
	for id, placement := range s.placements {
		out.placements[id] = proto.Clone(placement).(*diagv1.PlacedElement)
	}
	for id, connector := range s.connectors {
		out.connectors[id] = proto.Clone(connector).(*diagv1.Connector)
	}
	return out
}

func elementFromInput(id int32, input ElementInput) *diagv1.Element {
	bypass := false
	if input.BypassNoiseGate != nil {
		bypass = *input.BypassNoiseGate
	}
	return &diagv1.Element{
		Id:              id,
		Name:            input.Name,
		Description:     input.Description,
		Kind:            input.Kind,
		Technology:      input.Technology,
		Url:             input.URL,
		LogoUrl:         input.LogoURL,
		TechnologyLinks: cloneTechnologyLinks(input.TechLinks),
		Tags:            append([]string(nil), input.Tags...),
		Repo:            input.Repo,
		Branch:          input.Branch,
		Language:        input.Language,
		FilePath:        input.FilePath,
		HasView:         input.HasView,
		ViewLabel:       input.ViewLabel,
		BypassNoiseGate: bypass,
	}
}

func placedElementFromElement(id, viewID int32, element *diagv1.Element, x, y float64) *diagv1.PlacedElement {
	return &diagv1.PlacedElement{
		Id:              id,
		ViewId:          viewID,
		ElementId:       element.GetId(),
		PositionX:       x,
		PositionY:       y,
		Name:            element.GetName(),
		Description:     element.Description,
		Kind:            element.Kind,
		Technology:      element.Technology,
		Url:             element.Url,
		LogoUrl:         element.LogoUrl,
		TechnologyLinks: cloneTechnologyLinks(element.GetTechnologyLinks()),
		Tags:            append([]string(nil), element.GetTags()...),
		Repo:            element.Repo,
		Branch:          element.Branch,
		FilePath:        element.FilePath,
		Language:        element.Language,
		HasView:         element.GetHasView(),
		ViewLabel:       element.ViewLabel,
		BypassNoiseGate: element.GetBypassNoiseGate(),
	}
}

func cloneTechnologyLinks(values []*diagv1.TechnologyLink) []*diagv1.TechnologyLink {
	out := make([]*diagv1.TechnologyLink, 0, len(values))
	for _, value := range values {
		out = append(out, proto.Clone(value).(*diagv1.TechnologyLink))
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
