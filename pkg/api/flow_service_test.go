package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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

type denyFlowWrites struct {
	NopWorkspaceHooks
	afterWrites int
}

func (denyFlowWrites) CheckWrite(context.Context, uuid.UUID, string) error {
	return connect.NewError(connect.CodePermissionDenied, errors.New("read only"))
}

func (h *denyFlowWrites) AfterWrite(context.Context, uuid.UUID, string, string, string, map[string]any, any) {
	h.afterWrites++
}

func TestFlowServiceReadOnly(t *testing.T) {
	hooks := &denyFlowWrites{}
	svc := &FlowService{Hooks: hooks}
	_, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: uuid.NewString()}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("save: %v", err)
	}
	_, err = svc.DeleteFlow(context.Background(), connect.NewRequest(&diagv1.DeleteFlowRequest{OrgId: uuid.NewString()}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("delete: %v", err)
	}
	if hooks.afterWrites != 0 {
		t.Fatalf("failed writes called AfterWrite %d times", hooks.afterWrites)
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

func TestFlowControlOperationsRoundTrip(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	ctx := context.Background()
	parentView, err := store.CreateView(ctx, "Checkout", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	childView, err := store.CreateView(ctx, "Payments", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	child, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: &diagv1.Flow{ViewId: int32(childView.ID), Name: "Payment"}}))
	if err != nil {
		t.Fatal(err)
	}
	flow := &diagv1.Flow{ViewId: int32(parentView.ID), Name: "Purchase", Steps: []*diagv1.FlowStep{
		{Id: "intro", Overview: true},
		{Id: "decision", Branches: &diagv1.FlowBranches{Mode: "alternate", Paths: []*diagv1.FlowBranch{
			{Id: "yes", Name: "Success", Steps: []*diagv1.FlowStep{{Id: "call", GoToFlowId: child.Msg.Flow.Id}}},
			{Id: "no", Name: "Failure", Steps: []*diagv1.FlowStep{{Id: "notify", Branches: &diagv1.FlowBranches{Mode: "parallel", Paths: []*diagv1.FlowBranch{
				{Id: "customer", Name: "Customer", Steps: []*diagv1.FlowStep{{Id: "email", Connectors: []*diagv1.FlowConnectorHighlight{{Id: 10, Reverse: true}}}}},
				{Id: "ops", Name: "Operations", Steps: []*diagv1.FlowStep{{Id: "log", ElementIds: []int32{1}}}},
			}}}}},
		}}},
		{Id: "outro", Overview: true},
	}}
	saved, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: flow}))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: int32(parentView.ID)}))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Msg.Flows) != 1 || !proto.Equal(listed.Msg.Flows[0], saved.Msg.Flow) {
		t.Fatalf("nested controls did not round-trip: %v", listed.Msg)
	}
	_, err = svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: child.Msg.Flow.Id, ViewId: int32(childView.ID), ExpectedRevision: 1}))
	if err != nil {
		t.Fatal(err)
	}
	updated := proto.Clone(saved.Msg.Flow).(*diagv1.Flow)
	updated.Description = "safe unrelated edit"
	updated, err = func() (*diagv1.Flow, error) {
		response, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: updated}))
		if err != nil {
			return nil, err
		}
		return response.Msg.Flow, nil
	}()
	if err != nil || updated.Revision != 2 {
		t.Fatalf("unrelated update to flow with an existing broken link: flow=%v err=%v", updated, err)
	}
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: saved.Msg.Flow})); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("stale caller containing a broken link status=%v err=%v", connect.CodeOf(err), err)
	}
	added := proto.Clone(updated).(*diagv1.Flow)
	added.Steps = append(added.Steps, &diagv1.FlowStep{Id: "new-link", GoToFlowId: child.Msg.Flow.Id})
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: added})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("accepted a newly introduced unavailable link: %v", err)
	}
	if _, err := svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: updated.Id, ViewId: updated.ViewId, ExpectedRevision: updated.Revision})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{Flow: updated})); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("missing caller row status=%v err=%v", connect.CodeOf(err), err)
	}
}

func TestFlowExplicitWorkspaceScopesStorageAndHooks(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	orgA, orgB := uuid.New(), uuid.New()
	viewA, err := store.CreateView(app.WithTenantOrgID(context.Background(), orgA), "Scoped view A", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := store.CreateView(app.WithTenantOrgID(context.Background(), orgB), "Scoped view B", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	serviceHooks := &flowContextHooks{}
	svc := &FlowService{Store: NewAPIStore(store), Hooks: serviceHooks}
	var flowBID string
	for _, target := range []struct {
		orgID  uuid.UUID
		viewID int32
	}{{orgA, int32(viewA.ID)}, {orgB, int32(viewB.ID)}} {
		serviceHooks.want = target.orgID
		saved, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{
			OrgId: target.orgID.String(), Flow: &diagv1.Flow{ViewId: target.viewID, Name: "Scoped " + target.orgID.String()},
		}))
		if err != nil {
			t.Fatalf("save for %s: %v", target.orgID, err)
		}
		if target.orgID == orgB {
			flowBID = saved.Msg.Flow.Id
		}
	}
	for _, test := range []struct {
		orgID uuid.UUID
		want  int
	}{{orgA, 1}, {orgB, 1}} {
		serviceHooks.want = test.orgID
		listed, err := svc.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: test.orgID.String()}))
		if err != nil || len(listed.Msg.Flows) != test.want || listed.Msg.Flows[0].Name == "" {
			t.Fatalf("list for %s: got=%v err=%v", test.orgID, listed, err)
		}
	}
	serviceHooks.want = orgA
	for _, orgID := range []string{"", orgA.String()} {
		if _, err := svc.ListFlows(WithWorkspaceID(context.Background(), orgA), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgID})); err != nil {
			t.Fatalf("matching host context with org_id %q: %v", orgID, err)
		}
	}
	if _, err := svc.ListFlows(WithWorkspaceID(context.Background(), orgA), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgB.String()})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("mismatched host/request scope: %v", err)
	}
	if _, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: orgA.String(), Flow: &diagv1.Flow{
		ViewId: int32(viewA.ID), Name: "Cross-scope link", Steps: []*diagv1.FlowStep{{Id: "link", GoToFlowId: flowBID}},
	}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("cross-scope link status=%v err=%v", connect.CodeOf(err), err)
	}
	for _, malformed := range []func() error{
		func() error {
			_, err := svc.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: "malformed"}))
			return err
		},
		func() error {
			_, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: "malformed", Flow: &diagv1.Flow{ViewId: int32(viewA.ID), Name: "Bad scope"}}))
			return err
		},
	} {
		if err := malformed(); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("malformed org_id status=%v err=%v", connect.CodeOf(err), err)
		}
	}
	_, handler := diagv1connect.NewFlowServiceHandler(svc)
	server := httptest.NewServer(handler)
	defer server.Close()
	client := diagv1connect.NewFlowServiceClient(server.Client(), server.URL, connect.WithProtoJSON())
	if _, err := client.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: "malformed"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("wire ListFlows malformed org status=%v err=%v", connect.CodeOf(err), err)
	}
	if _, err := client.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: "malformed", Flow: &diagv1.Flow{ViewId: int32(viewA.ID), Name: "Bad scope"}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("wire SaveFlow malformed org status=%v err=%v", connect.CodeOf(err), err)
	}
	if serviceHooks.writeChecks != 3 || serviceHooks.readChecks != 4 || serviceHooks.afterWrites != 2 {
		t.Fatalf("hook calls: writes=%d reads=%d after=%d", serviceHooks.writeChecks, serviceHooks.readChecks, serviceHooks.afterWrites)
	}
}

type flowHookEvent struct {
	action       string
	resourceType string
	resourceID   string
	details      map[string]any
	response     any
}

type flowRecordingHooks struct {
	NopWorkspaceHooks
	readErr error
	events  []flowHookEvent
}

func (h *flowRecordingHooks) CheckRead(context.Context, uuid.UUID) error { return h.readErr }

func (h *flowRecordingHooks) AfterWrite(_ context.Context, _ uuid.UUID, action, resourceType, resourceID string, details map[string]any, response any) {
	h.events = append(h.events, flowHookEvent{action: action, resourceType: resourceType, resourceID: resourceID, details: details, response: response})
}

func TestFlowWriteHookEventContractAndFailureSuppression(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	orgID := uuid.New()
	view, err := store.CreateView(app.WithTenantOrgID(context.Background(), orgID), "Hook events", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hooks := &flowRecordingHooks{}
	svc := &FlowService{Store: NewAPIStore(store), Hooks: hooks}
	created, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{
		OrgId: orgID.String(), Flow: &diagv1.Flow{ViewId: int32(view.ID), Name: "Hooked flow"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	updatedInput := proto.Clone(created.Msg.Flow).(*diagv1.Flow)
	updatedInput.Description = "Updated description"
	updated, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: orgID.String(), Flow: updatedInput}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteFlow(context.Background(), connect.NewRequest(&diagv1.DeleteFlowRequest{
		OrgId: orgID.String(), Id: updated.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: updated.Msg.Flow.Revision,
	})); err != nil {
		t.Fatal(err)
	}
	if len(hooks.events) != 3 {
		t.Fatalf("got %d success events, want create/update/delete", len(hooks.events))
	}
	wantActions := []string{"create", "update", "delete"}
	wantIDs := []string{created.Msg.Flow.Id, created.Msg.Flow.Id, created.Msg.Flow.Id}
	wantResponses := []*diagv1.SaveFlowResponse{created.Msg, updated.Msg, nil}
	for i, event := range hooks.events {
		if event.action != wantActions[i] || event.resourceType != "flow" || event.resourceID != wantIDs[i] || event.details["view_id"] != int32(view.ID) {
			t.Errorf("event %d = %+v", i, event)
		}
		if i < 2 {
			response, ok := event.response.(*diagv1.SaveFlowResponse)
			if !ok || !proto.Equal(response, wantResponses[i]) {
				t.Errorf("event %d response = %#v, want %#v", i, event.response, wantResponses[i])
			}
		} else if event.response != nil {
			t.Errorf("delete event response = %#v, want nil", event.response)
		}
	}

	before := len(hooks.events)
	failedSaves := []struct {
		name string
		flow *diagv1.Flow
	}{
		{"invalid", &diagv1.Flow{ViewId: int32(view.ID), Name: " "}},
		{"stale revision", created.Msg.Flow},
		{"missing view", &diagv1.Flow{ViewId: int32(view.ID + 1000), Name: "Missing view"}},
		{"missing update row", &diagv1.Flow{Id: "missing-flow", ViewId: int32(view.ID), Name: "Missing row", Revision: 1}},
	}
	wantCodes := []connect.Code{connect.CodeInvalidArgument, connect.CodeAborted, connect.CodeNotFound, connect.CodeAborted}
	for i, test := range failedSaves {
		_, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: orgID.String(), Flow: test.flow}))
		if connect.CodeOf(err) != wantCodes[i] {
			t.Errorf("%s save status=%v err=%v", test.name, connect.CodeOf(err), err)
		}
		if len(hooks.events) != before {
			t.Fatalf("%s save emitted success hook: %+v", test.name, hooks.events[before:])
		}
	}
	_, err = svc.DeleteFlow(context.Background(), connect.NewRequest(&diagv1.DeleteFlowRequest{
		OrgId: orgID.String(), Id: created.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: created.Msg.Flow.Revision,
	}))
	if connect.CodeOf(err) != connect.CodeAborted || len(hooks.events) != before {
		t.Fatalf("stale delete status=%v err=%v, events=%+v", connect.CodeOf(err), err, hooks.events[before:])
	}
}

func TestFlowReadHookDenialIsPreserved(t *testing.T) {
	want := connect.NewError(connect.CodePermissionDenied, errors.New("read denied"))
	hooks := &flowRecordingHooks{readErr: want}
	svc := &FlowService{Hooks: hooks}
	_, err := svc.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: uuid.NewString()}))
	if connect.CodeOf(err) != connect.CodePermissionDenied || len(hooks.events) != 0 {
		t.Fatalf("read denial status=%v err=%v events=%+v", connect.CodeOf(err), err, hooks.events)
	}
}

type flowContextHooks struct {
	NopWorkspaceHooks
	want        uuid.UUID
	writeChecks int
	readChecks  int
	afterWrites int
}

func (h *flowContextHooks) check(ctx context.Context, workspaceID uuid.UUID) error {
	if workspaceID != h.want || WorkspaceIDFromCtx(ctx) != h.want || app.TenantOrgIDFromCtx(ctx) != h.want {
		return fmt.Errorf("workspace hook/store context mismatch: arg=%s api=%s store=%s want=%s", workspaceID, WorkspaceIDFromCtx(ctx), app.TenantOrgIDFromCtx(ctx), h.want)
	}
	return nil
}

func (h *flowContextHooks) CheckWrite(ctx context.Context, workspaceID uuid.UUID, _ string) error {
	h.writeChecks++
	return h.check(ctx, workspaceID)
}

func (h *flowContextHooks) CheckRead(ctx context.Context, workspaceID uuid.UUID) error {
	h.readChecks++
	return h.check(ctx, workspaceID)
}

func (h *flowContextHooks) AfterWrite(ctx context.Context, workspaceID uuid.UUID, _, resourceType, _ string, _ map[string]any, _ any) {
	h.afterWrites++
	if resourceType != "flow" {
		panic("flow hook received another resource type")
	}
	if err := h.check(ctx, workspaceID); err != nil {
		panic(err)
	}
}

func TestFlowMissingViewReturnsNotFound(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	svc := &FlowService{Store: NewAPIStore(store)}
	missingView := int32(99999)
	_, err := svc.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: missingView}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing list view status = %v, err=%v", connect.CodeOf(err), err)
	}
	_, err = svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{Flow: &diagv1.Flow{ViewId: missingView, Name: "Missing"}}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing save view status = %v, err=%v", connect.CodeOf(err), err)
	}
	_, err = svc.DeleteFlow(context.Background(), connect.NewRequest(&diagv1.DeleteFlowRequest{Id: "flow", ViewId: missingView, ExpectedRevision: 1}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing delete view status = %v, err=%v", connect.CodeOf(err), err)
	}
	if connect.CodeOf(flowViewError("view", sql.ErrNoRows)) != connect.CodeNotFound {
		t.Fatal("sql.ErrNoRows must map to NOT_FOUND")
	}
	if _, err := svc.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: -1})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("negative view id status=%v err=%v", connect.CodeOf(err), err)
	}
	if _, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{Flow: &diagv1.Flow{Name: "Invalid"}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("nonpositive save view status=%v err=%v", connect.CodeOf(err), err)
	}
}

func TestFlowListFailsClosedOnMixedCorruptRows(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	view, err := store.CreateView(context.Background(), "Corrupt rows", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, document string }{
		{"valid", `{"name":"Valid","steps":[]}`},
		{"corrupt", `{"name":"Corrupt","futureField":true}`},
	} {
		if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO flows (id, view_id, document, revision) VALUES (?, ?, ?, 1)`, row.id, view.ID, row.document); err != nil {
			t.Fatal(err)
		}
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	if response, err := svc.ListFlows(context.Background(), connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: int32(view.ID)})); connect.CodeOf(err) != connect.CodeInternal || response != nil {
		t.Fatalf("mixed corrupt list must fail atomically: response=%v err=%v", response, err)
	}
	rows, err := store.ListFlows(context.Background(), view.ID)
	documents := map[string]string{}
	for _, row := range rows {
		documents[row.ID] = row.Document
	}
	if err != nil || len(rows) != 2 || documents["valid"] != `{"name":"Valid","steps":[]}` || documents["corrupt"] != `{"name":"Corrupt","futureField":true}` {
		t.Fatalf("read failure changed stored rows: rows=%+v err=%v", rows, err)
	}
}

func TestFlowFromRecordUsesAuthoritativeRowIdentity(t *testing.T) {
	flow, err := flowFromRecord(app.FlowRecord{ID: "row-id", ViewID: 42, Revision: 7, Document: `{"id":"document-id","viewId":9,"revision":2,"name":"Saved"}`})
	if err != nil {
		t.Fatal(err)
	}
	if flow.Id != "row-id" || flow.ViewId != 42 || flow.Revision != 7 {
		t.Fatalf("row identity did not override document fields: %v", flow)
	}
}

func TestFlowValidationUsesUTF8ByteLimitsAndSerializedSizeBoundary(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	view, err := store.CreateView(context.Background(), "Limit", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	viewID := int32(view.ID)
	nameFits := strings.Repeat("界", 66)
	if err := validateFlow(&diagv1.Flow{ViewId: viewID, Name: nameFits}); err != nil {
		t.Fatalf("accepted 198-byte name should fit: %v", err)
	}
	if err := validateFlow(&diagv1.Flow{ViewId: viewID, Name: strings.Repeat("界", 67)}); err == nil {
		t.Fatal("accepted a 201-byte name over the 200-byte limit")
	}

	flow := &diagv1.Flow{ViewId: viewID, Name: "Limit"}
	for i := 0; i < 104; i++ {
		flow.Steps = append(flow.Steps, &diagv1.FlowStep{Id: fmt.Sprintf("s%d", i), Description: strings.Repeat("x", 10000)})
	}
	flow.Steps = append(flow.Steps, &diagv1.FlowStep{Id: "last"})
	base, err := protojson.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	flow.Steps[len(flow.Steps)-1].Description = "x"
	oneChar, err := protojson.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	fieldCost := len(oneChar) - len(base)
	lastDescriptionSize := (1 << 20) - len(base) - fieldCost
	if lastDescriptionSize < 0 || lastDescriptionSize > 10000 {
		t.Fatalf("fixture cannot reach exact protobuf JSON limit: base=%d field=%d remaining=%d", len(base), fieldCost, lastDescriptionSize)
	}
	flow.Steps[len(flow.Steps)-1].Description = strings.Repeat("x", lastDescriptionSize)
	exact, err := protojson.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	remaining := (1 << 20) - len(exact)
	if remaining > 0 {
		flow.Steps[len(flow.Steps)-1].Description += strings.Repeat("x", remaining)
	} else if remaining < 0 {
		flow.Steps[len(flow.Steps)-1].Description = strings.TrimSuffix(flow.Steps[len(flow.Steps)-1].Description, strings.Repeat("x", -remaining))
	}
	exact, err = protojson.Marshal(flow)
	if err != nil || len(exact) != 1<<20 {
		t.Fatalf("expected exactly 1 MiB protobuf JSON, size=%d err=%v", len(exact), err)
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	if _, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{Flow: proto.Clone(flow).(*diagv1.Flow)})); err != nil {
		t.Fatalf("rejected exactly 1 MiB flow: %v", err)
	}
	over := proto.Clone(flow).(*diagv1.Flow)
	over.Steps[len(over.Steps)-1].Description += "x"
	if _, err := svc.SaveFlow(context.Background(), connect.NewRequest(&diagv1.SaveFlowRequest{Flow: over})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("one byte over 1 MiB status=%v err=%v", connect.CodeOf(err), err)
	}
}

func TestFlowStringValidationByteBoundaries(t *testing.T) {
	utf8Bytes := func(size int) string {
		return strings.Repeat("界", size/3) + strings.Repeat("x", size%3)
	}
	fields := []struct {
		name  string
		limit int
		build func(string) *diagv1.Flow
	}{
		{"flow name", 200, func(value string) *diagv1.Flow { return &diagv1.Flow{ViewId: 1, Name: value} }},
		{"flow description", 10000, func(value string) *diagv1.Flow { return &diagv1.Flow{ViewId: 1, Name: "Flow", Description: value} }},
		{"step id", 100, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: value}}}
		}},
		{"step title", 500, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", Title: value}}}
		}},
		{"step description", 10000, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", Description: value}}}
		}},
		{"branch id", 100, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", Branches: &diagv1.FlowBranches{Mode: "alternate", Paths: []*diagv1.FlowBranch{{Id: value, Name: "A"}, {Id: "b", Name: "B"}}}}}}
		}},
		{"branch name", 200, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", Branches: &diagv1.FlowBranches{Mode: "alternate", Paths: []*diagv1.FlowBranch{{Id: "a", Name: value}, {Id: "b", Name: "B"}}}}}}
		}},
		{"linked flow id", 100, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", GoToFlowId: value}}}
		}},
		{"metadata key", 100, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", Metadata: map[string]string{value: "value"}}}}
		}},
		{"metadata value", 10000, func(value string) *diagv1.Flow {
			return &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step", Metadata: map[string]string{"key": value}}}}
		}},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			if err := validateFlow(field.build(utf8Bytes(field.limit))); err != nil {
				t.Fatalf("rejected exact %d-byte boundary: %v", field.limit, err)
			}
			if err := validateFlow(field.build(utf8Bytes(field.limit + 1))); err == nil {
				t.Fatalf("accepted %d bytes over the %d-byte boundary", field.limit+1, field.limit)
			}
		})
	}
}

func TestFlowValidationCollectionBoundaries(t *testing.T) {
	flow := &diagv1.Flow{ViewId: 1, Name: "Collections", Steps: []*diagv1.FlowStep{{Id: "step", Metadata: map[string]string{}}}}
	step := flow.Steps[0]
	for i := 1; i <= 500; i++ {
		step.ElementIds = append(step.ElementIds, int32(i))
		step.Connectors = append(step.Connectors, &diagv1.FlowConnectorHighlight{Id: int32(i)})
	}
	for i := 0; i < 50; i++ {
		step.Metadata[fmt.Sprintf("key-%d", i)] = fmt.Sprintf("value-%d", i)
	}
	if err := validateFlow(flow); err != nil {
		t.Fatalf("rejected exact valid collection boundaries: %v", err)
	}
	step.ElementIds = append(step.ElementIds, 501)
	if err := validateFlow(flow); err == nil {
		t.Fatal("accepted 501 element highlights")
	}
	step.ElementIds = step.ElementIds[:500]
	step.Connectors = append(step.Connectors, &diagv1.FlowConnectorHighlight{Id: 501})
	if err := validateFlow(flow); err == nil {
		t.Fatal("accepted 501 connector highlights")
	}
	step.Connectors = step.Connectors[:500]
	step.Metadata["key-50"] = "value-50"
	if err := validateFlow(flow); err == nil {
		t.Fatal("accepted 51 metadata entries")
	}
}

func TestValidateFlowControls(t *testing.T) {
	branches := func() *diagv1.FlowBranches {
		return &diagv1.FlowBranches{Mode: "alternate", Paths: []*diagv1.FlowBranch{{Id: "a", Name: "A"}, {Id: "b", Name: "B"}}}
	}
	for _, step := range []*diagv1.FlowStep{
		{Id: "1", Overview: true, GoToFlowId: "other"},
		{Id: "1", Overview: true, Branches: branches()},
		{Id: "1", GoToFlowId: "other", Branches: branches()},
		{Id: "1", Branches: &diagv1.FlowBranches{Mode: "unknown", Paths: branches().Paths}},
		{Id: "1", Branches: &diagv1.FlowBranches{Mode: "parallel"}},
		{Id: "a", Branches: branches()},
		{Id: "1", GoToFlowId: " "},
		{Id: "1", Branches: &diagv1.FlowBranches{Mode: "alternate", Paths: []*diagv1.FlowBranch{nil, {Id: "b", Name: "B"}}}},
	} {
		if err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{step}}); err == nil {
			t.Errorf("accepted invalid controls: %v", step)
		}
	}
	steps := []*diagv1.FlowStep{{Id: "leaf"}}
	for i := 0; i < 9; i++ {
		steps = []*diagv1.FlowStep{{Id: fmt.Sprintf("s%d", i), Branches: &diagv1.FlowBranches{Mode: "parallel", Paths: []*diagv1.FlowBranch{{Id: fmt.Sprintf("a%d", i), Name: "A", Steps: steps}, {Id: fmt.Sprintf("b%d", i), Name: "B"}}}}}
	}
	if err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Deep", Steps: steps}); err == nil {
		t.Fatal("accepted excessive nesting")
	}
	branch := branches()
	for i := 0; i < maxFlowSteps; i++ {
		branch.Paths[i%2].Steps = append(branch.Paths[i%2].Steps, &diagv1.FlowStep{Id: fmt.Sprintf("s%d", i)})
	}
	if err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Wide", Steps: []*diagv1.FlowStep{{Id: "root", Branches: branch}}}); err == nil {
		t.Fatal("accepted more than the total step limit")
	}
}

func TestValidateFlowLimits(t *testing.T) {
	t.Run("total steps across paths", func(t *testing.T) {
		paths := []*diagv1.FlowBranch{{Id: "a", Name: "A"}, {Id: "b", Name: "B"}}
		for i := 0; i < maxFlowSteps-1; i++ {
			paths[i%2].Steps = append(paths[i%2].Steps, &diagv1.FlowStep{Id: fmt.Sprintf("s%d", i)})
		}
		flow := &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "root", Branches: &diagv1.FlowBranches{Mode: "parallel", Paths: paths}}}}
		if err := validateFlow(flow); err != nil {
			t.Fatalf("rejected exactly %d steps: %v", maxFlowSteps, err)
		}
		paths[0].Steps = append(paths[0].Steps, &diagv1.FlowStep{Id: "too-many"})
		if err := validateFlow(flow); err == nil {
			t.Fatal("accepted one step beyond the limit")
		}
	})
	t.Run("paths", func(t *testing.T) {
		branch := &diagv1.FlowBranches{Mode: "alternate"}
		for i := 0; i < maxFlowPaths; i++ {
			branch.Paths = append(branch.Paths, &diagv1.FlowBranch{Id: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("Path %d", i)})
		}
		flow := &diagv1.Flow{ViewId: 1, Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "root", Branches: branch}}}
		if err := validateFlow(flow); err != nil {
			t.Fatalf("rejected exactly %d paths: %v", maxFlowPaths, err)
		}
		branch.Paths = append(branch.Paths, &diagv1.FlowBranch{Id: "too-many", Name: "Too many"})
		if err := validateFlow(flow); err == nil {
			t.Fatal("accepted one path beyond the limit")
		}
	})
	t.Run("nesting", func(t *testing.T) {
		steps := []*diagv1.FlowStep{{Id: "leaf"}}
		for i := 0; i <= maxFlowBranchDepth; i++ {
			steps = []*diagv1.FlowStep{{Id: fmt.Sprintf("s%d", i), Branches: &diagv1.FlowBranches{Mode: "alternate", Paths: []*diagv1.FlowBranch{{Id: fmt.Sprintf("a%d", i), Name: "A", Steps: steps}, {Id: fmt.Sprintf("b%d", i), Name: "B"}}}}}
			err := validateFlow(&diagv1.Flow{ViewId: 1, Name: "Flow", Steps: steps})
			if i < maxFlowBranchDepth && err != nil {
				t.Fatalf("rejected allowed nesting: %v", err)
			}
			if i == maxFlowBranchDepth && err == nil {
				t.Fatal("accepted excessive nesting")
			}
		}
	})
}
