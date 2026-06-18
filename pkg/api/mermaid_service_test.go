package api

import (
	"context"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
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
