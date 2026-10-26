package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestFlowServiceListsLegacyAndCurrentFlows(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	ctx := context.Background()
	view, err := store.CreateView(ctx, "Legacy flow", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range []string{
		`{"name":"Legacy","steps":[{"id":"message","kind":"message","connectorId":652,"reverse":true,"title":"Reply"},{"id":"process","kind":"process","elementId":719},{"id":"note","kind":"note","description":"Context"}]}`,
		`{"name":"Current","steps":[{"id":"mixed","elementIds":[719],"connectors":[{"id":653}],"metadata":{"owner":"Team"}}]}`,
	} {
		if _, err := store.SaveFlow(ctx, app.FlowRecord{ViewID: view.ID, Document: document}); err != nil {
			t.Fatal(err)
		}
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	response, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: int32(view.ID)}))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Msg.Flows) != 2 {
		t.Fatalf("expected both flows: %v", response.Msg)
	}
	for _, flow := range response.Msg.Flows {
		if flow.Name == "Legacy" {
			if len(flow.Steps[0].Connectors) != 1 || flow.Steps[0].Connectors[0].Id != 652 || !flow.Steps[0].Connectors[0].Reverse || flow.Steps[0].Title != "Reply" || len(flow.Steps[1].ElementIds) != 1 || flow.Steps[1].ElementIds[0] != 719 || flow.Steps[2].Description != "Context" {
				t.Fatalf("legacy references lost: %v", flow)
			}
		} else if flow.Steps[0].Metadata["owner"] != "Team" || flow.Steps[0].Connectors[0].Id != 653 {
			t.Fatalf("current flow changed: %v", flow)
		}
		if _, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: flow})); err != nil {
			t.Fatalf("converted flow cannot be saved: %v", err)
		}
	}
}

func TestFlowRecordRejectsUnknownFields(t *testing.T) {
	for _, document := range []string{`{"name":"Flow","futureField":true}`, `{"steps":[{"id":"step","futureField":true}]}`, `{"steps":"invalid"}`} {
		if _, err := flowFromRecord(app.FlowRecord{Document: document}); err == nil {
			t.Fatalf("silently discarded unsupported data: %s", document)
		}
	}
}

func TestFlowServiceRoundTrip(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	ctx := context.Background()
	view, err := store.CreateView(ctx, "Checkout", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, handler := diagv1connect.NewFlowServiceHandler(&FlowService{Store: NewAPIStore(store)})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := diagv1connect.NewFlowServiceClient(server.Client(), server.URL, connect.WithProtoJSON())
	// Standalone clients omit org_id; hosted clients use the request context.
	orgID := ""
	saved, err := client.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: orgID, Flow: &diagv1.Flow{
		ViewId: int32(view.ID), Name: "Purchase", Steps: []*diagv1.FlowStep{
			{Id: "intro", Title: "Customer checkout"},
			{Id: "response", ElementIds: []int32{1, 2}, Connectors: []*diagv1.FlowConnectorHighlight{{Id: 42, Reverse: true}, {Id: 43}}, Description: "Respond", Metadata: map[string]string{"owner": "Commerce"}},
		},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Msg.Flow.Revision != 1 || saved.Msg.Flow.Id == "" {
		t.Fatalf("invalid response: %v", saved.Msg)
	}
	listed, err := client.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgID, ViewId: int32(view.ID)}))
	if err != nil || len(listed.Msg.Flows) != 1 {
		t.Fatalf("list: %v %v", listed, err)
	}
	if !listed.Msg.Flows[0].Steps[1].Connectors[0].Reverse || len(listed.Msg.Flows[0].Steps[1].Connectors) != 2 || len(listed.Msg.Flows[0].Steps[1].ElementIds) != 2 || listed.Msg.Flows[0].Steps[1].Metadata["owner"] != "Commerce" {
		t.Fatal("lost reverse direction")
	}
	_, err = client.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: orgID, Flow: saved.Msg.Flow}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: orgID, Flow: saved.Msg.Flow}))
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("stale save: %v", err)
	}
	_, err = client.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{OrgId: orgID, Id: saved.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: 2}))
	if err != nil {
		t.Fatal(err)
	}
	listed, err = client.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgID}))
	if err != nil || len(listed.Msg.Flows) != 0 {
		t.Fatalf("list after delete: %v %v", listed, err)
	}
}

type denyFlowWrites struct{ NopWorkspaceHooks }

func (denyFlowWrites) CheckWrite(context.Context, uuid.UUID, string) error {
	return connect.NewError(connect.CodePermissionDenied, errors.New("read only"))
}

func TestFlowServiceReadOnly(t *testing.T) {
	svc := &FlowService{Hooks: denyFlowWrites{}}
	_, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: uuid.NewString()}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("save: %v", err)
	}
	_, err = svc.DeleteFlow(context.Background(), connect.NewRequest(&diagv1.DeleteFlowRequest{OrgId: uuid.NewString()}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("delete: %v", err)
	}
}

func TestFlowWorkspaceMismatch(t *testing.T) {
	ctx := WithWorkspaceID(context.Background(), uuid.New())
	_, err := flowWorkspaceID(ctx, uuid.NewString())
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("mismatched workspace: %v", err)
	}
}

func TestValidateFlow(t *testing.T) {
	for _, step := range []*diagv1.FlowStep{
		nil, {Id: " "},
		{Id: "1", ElementIds: []int32{0}}, {Id: "1", ElementIds: []int32{1, 1}},
		{Id: "1", Connectors: []*diagv1.FlowConnectorHighlight{nil}},
		{Id: "1", Connectors: []*diagv1.FlowConnectorHighlight{{Id: -1}}},
		{Id: "1", Connectors: []*diagv1.FlowConnectorHighlight{{Id: 1}, {Id: 1, Reverse: true}}},
		{Id: "1", Metadata: map[string]string{" ": "invalid"}},
		{Id: "1", Metadata: map[string]string{"owner": strings.Repeat("x", 10001)}},
		{Id: "1", ElementIds: make([]int32, 501)},
		{Id: "1", Connectors: make([]*diagv1.FlowConnectorHighlight, 501)},
	} {
		if err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{step}}); err == nil {
			t.Errorf("accepted invalid step: %v", step)
		}
	}
	for _, steps := range [][]*diagv1.FlowStep{nil, {{Id: "empty"}}, {{Id: "mixed", ElementIds: []int32{1, 2}, Connectors: []*diagv1.FlowConnectorHighlight{{Id: 3}, {Id: 4, Reverse: true}}, Metadata: map[string]string{"owner": "Commerce"}}}} {
		if err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Flow", Steps: steps}); err != nil {
			t.Errorf("rejected valid steps: %v", err)
		}
	}
	for _, steps := range [][]*diagv1.FlowStep{{{Id: "same"}, {Id: "same"}}, make([]*diagv1.FlowStep, 501)} {
		if err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Flow", Steps: steps}); err == nil {
			t.Error("accepted duplicate IDs or too many steps")
		}
	}
}
