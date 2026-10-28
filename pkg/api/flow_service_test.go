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

func flowSaveRequest(flow *diagv1.Flow) *diagv1.SaveFlowRequest {
	return &diagv1.SaveFlowRequest{Id: flow.Id, ViewId: flow.ViewId, ExpectedRevision: flow.Revision, Content: flow.Content}
}

func flowBranches(mode diagv1.FlowBranchMode, paths ...*diagv1.FlowBranch) *diagv1.FlowStep_Branches {
	return &diagv1.FlowStep_Branches{Branches: &diagv1.FlowBranches{Mode: mode, Paths: paths}}
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
	content := &diagv1.FlowContent{Name: "Purchase", Steps: []*diagv1.FlowStep{
		{Id: "intro", Title: "Customer checkout"},
		{Id: "response", ElementIds: []int32{719, 720}, ConnectorIds: []int32{652, 653}, Description: "Respond"},
		{Id: "decision", Control: flowBranches(diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL,
			&diagv1.FlowBranch{Id: "a", Name: "A", Steps: []*diagv1.FlowStep{{Id: "call", Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: "missing-flow"}}}},
			&diagv1.FlowBranch{Id: "b", Name: "B"})},
	}}
	saved, err := client.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{ViewId: int32(view.ID), Content: content}))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Msg.Flow.Id == "" || saved.Msg.Flow.Revision != 1 || !proto.Equal(saved.Msg.Flow.Content, content) {
		t.Fatalf("invalid saved flow: %v", saved.Msg)
	}
	rows, err := store.ListFlows(ctx, view.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("stored flow: %v %v", rows, err)
	}
	if rows[0].Name != content.Name || len(rows[0].Steps) != len(content.Steps) {
		t.Fatalf("stored content differs: %+v", rows[0])
	}
	var stepCount, pathCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM flow_steps WHERE flow_id = ?", saved.Msg.Flow.Id).Scan(&stepCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM flow_paths WHERE flow_id = ?", saved.Msg.Flow.Id).Scan(&pathCount); err != nil {
		t.Fatal(err)
	}
	if stepCount != 4 || pathCount != 2 {
		t.Fatalf("normalized rows: steps=%d paths=%d", stepCount, pathCount)
	}

	listed, err := client.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: int32(view.ID)}))
	if err != nil || len(listed.Msg.Flows) != 1 || !proto.Equal(listed.Msg.Flows[0], saved.Msg.Flow) {
		t.Fatalf("list: %v %v", listed, err)
	}
	updated, err := client.SaveFlow(ctx, connect.NewRequest(flowSaveRequest(saved.Msg.Flow)))
	if err != nil || updated.Msg.Flow.Revision != 2 {
		t.Fatalf("update: %v %v", updated, err)
	}
	if _, err := client.SaveFlow(ctx, connect.NewRequest(flowSaveRequest(saved.Msg.Flow))); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := client.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: saved.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: 1})); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("stale delete: %v", err)
	}
	if _, err := client.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: saved.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: 2})); err != nil {
		t.Fatal(err)
	}
	listed, err = client.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{}))
	if err != nil || len(listed.Msg.Flows) != 0 {
		t.Fatalf("list after delete: %v %v", listed, err)
	}
}

func TestFlowSoftReferencesSurviveTargetDeletion(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	ctx := context.Background()
	view, err := store.CreateView(ctx, "Caller", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	target, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{ViewId: int32(view.ID), Content: &diagv1.FlowContent{Name: "Target"}}))
	if err != nil {
		t.Fatal(err)
	}
	req := &diagv1.SaveFlowRequest{ViewId: int32(view.ID), Content: &diagv1.FlowContent{Name: "Caller", Steps: []*diagv1.FlowStep{{Id: "link", ElementIds: []int32{999}, ConnectorIds: []int32{888}, Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: target.Msg.Flow.Id}}}}}
	caller, err := svc.SaveFlow(ctx, connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: target.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: 1})); err != nil {
		t.Fatal(err)
	}
	req = flowSaveRequest(caller.Msg.Flow)
	req.Content.Steps = append(req.Content.Steps, &diagv1.FlowStep{Id: "new-missing-link", Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: "never-existed"}})
	updated, err := svc.SaveFlow(ctx, connect.NewRequest(req))
	if err != nil || updated.Msg.Flow.Revision != 2 || !proto.Equal(req.Content, updated.Msg.Flow.Content) {
		t.Fatalf("soft references prevented editing: %v %v", updated, err)
	}
}

type flowTestHooks struct {
	NopWorkspaceHooks
	want              uuid.UUID
	readErr, writeErr error
	reads, writes     int
	events            []string
}

func (h *flowTestHooks) check(ctx context.Context, workspaceID uuid.UUID) {
	if workspaceID != h.want || WorkspaceIDFromCtx(ctx) != h.want || app.TenantOrgIDFromCtx(ctx) != h.want {
		panic("workspace hook and store contexts differ")
	}
}
func (h *flowTestHooks) CheckRead(ctx context.Context, id uuid.UUID) error {
	h.check(ctx, id)
	h.reads++
	return h.readErr
}
func (h *flowTestHooks) CheckWrite(ctx context.Context, id uuid.UUID, resource string) error {
	h.check(ctx, id)
	if resource != "flows" {
		panic("unexpected resource")
	}
	h.writes++
	return h.writeErr
}
func (h *flowTestHooks) AfterWrite(ctx context.Context, id uuid.UUID, action, resource, resourceID string, details map[string]any, response any) {
	h.check(ctx, id)
	if resource != "flow" || resourceID == "" || details["view_id"].(int32) <= 0 {
		panic("invalid flow event")
	}
	if action != "delete" && response.(*diagv1.SaveFlowResponse).Flow.Id != resourceID {
		panic("invalid flow response")
	}
	h.events = append(h.events, action)
}

func TestFlowWorkspaceIsolationAndHooks(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	orgA, orgB := uuid.New(), uuid.New()
	ctx := context.Background()
	viewA, err := store.CreateView(app.WithTenantOrgID(ctx, orgA), "A", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := store.CreateView(app.WithTenantOrgID(ctx, orgB), "B", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hooks := &flowTestHooks{}
	svc := &FlowService{Store: NewAPIStore(store), Hooks: hooks}
	saved := map[uuid.UUID]*diagv1.Flow{}
	for _, scope := range []struct {
		org  uuid.UUID
		view int64
	}{{orgA, viewA.ID}, {orgB, viewB.ID}} {
		hooks.want = scope.org
		result, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{OrgId: scope.org.String(), ViewId: int32(scope.view), Content: &diagv1.FlowContent{Name: "Scoped"}}))
		if err != nil {
			t.Fatal(err)
		}
		saved[scope.org] = result.Msg.Flow
		listed, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: scope.org.String()}))
		if err != nil || len(listed.Msg.Flows) != 1 || listed.Msg.Flows[0].Id != result.Msg.Flow.Id {
			t.Fatalf("scope leaked: %v %v", listed, err)
		}
	}
	hooks.want = orgA
	for _, requested := range []string{"", orgA.String()} {
		if _, err := svc.ListFlows(WithWorkspaceID(ctx, orgA), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: requested})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ListFlows(WithWorkspaceID(ctx, orgA), connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgB.String()})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("accepted mismatched workspace: %v", err)
	}
	for _, requested := range []string{"malformed"} {
		if _, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: requested})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("accepted malformed workspace: %v", err)
		}
	}
	crossScope := flowSaveRequest(saved[orgB])
	crossScope.OrgId = orgA.String()
	crossScope.ViewId = int32(viewA.ID)
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(crossScope)); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("cross-workspace update: %v", err)
	}
	if _, err := svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{OrgId: orgA.String(), Id: saved[orgB].Id, ViewId: int32(viewA.ID), ExpectedRevision: 1})); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("cross-workspace delete: %v", err)
	}
	// Foreign IDs may be stored as unresolved soft references, but never expand the readable scope.
	own := flowSaveRequest(saved[orgA])
	own.OrgId = orgA.String()
	own.Content.Steps = []*diagv1.FlowStep{{Id: "foreign", Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: saved[orgB].Id}}}
	updated, err := svc.SaveFlow(ctx, connect.NewRequest(own))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgA.String()}))
	if err != nil || len(listed.Msg.Flows) != 1 {
		t.Fatalf("link exposed foreign flow: %v %v", listed, err)
	}
	if _, err := svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{OrgId: orgA.String(), Id: updated.Msg.Flow.Id, ViewId: int32(viewA.ID), ExpectedRevision: 2})); err != nil {
		t.Fatal(err)
	}
	if strings.Join(hooks.events, ",") != "create,create,update,delete" {
		t.Fatalf("unexpected success events: %v", hooks.events)
	}
	before := len(hooks.events)
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(own)); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("missing row: %v", err)
	}
	hooks.writeErr = connect.NewError(connect.CodePermissionDenied, errors.New("read only"))
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(own)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("write denial: %v", err)
	}
	if _, err := svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{OrgId: orgA.String()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("delete denial: %v", err)
	}
	hooks.readErr = connect.NewError(connect.CodePermissionDenied, errors.New("read denied"))
	if _, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: orgA.String()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("read denial: %v", err)
	}
	if len(hooks.events) != before {
		t.Fatal("failed requests emitted success events")
	}
}

func TestFlowMissingViewReturnsNotFound(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	svc := &FlowService{Store: NewAPIStore(store)}
	ctx := context.Background()
	_, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: 99999}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal(err)
	}
	_, err = svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{ViewId: 99999, Content: &diagv1.FlowContent{Name: "Missing"}}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal(err)
	}
	_, err = svc.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: "flow", ViewId: 99999, ExpectedRevision: 1}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal(err)
	}
	if connect.CodeOf(flowViewError("get", sql.ErrNoRows)) != connect.CodeNotFound {
		t.Fatal("missing view status")
	}
	_, err = svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: -1}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal(err)
	}
}

func TestFlowFromRecordUsesRowEnvelope(t *testing.T) {
	flow, err := flowFromRecord(app.FlowRecord{ID: "row", ViewID: 42, Revision: 7, Name: "Saved"})
	if err != nil || flow.Id != "row" || flow.ViewId != 42 || flow.Revision != 7 || flow.Content.Name != "Saved" {
		t.Fatalf("row envelope: %v %v", flow, err)
	}
	if _, err := flowFromRecord(app.FlowRecord{Steps: []app.FlowStep{{ID: "s", Branches: &app.FlowBranches{Mode: "unknown"}}}}); err == nil {
		t.Fatal("accepted invalid branch mode")
	}
}

func TestFlowListFailsClosedOnMixedCorruptRows(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	ctx := context.Background()
	view, err := store.CreateView(ctx, "Corrupt", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var corruptID string
	for _, name := range []string{"Valid", "Corrupt"} {
		row, err := store.SaveFlow(ctx, app.FlowRecord{ViewID: view.ID, Name: name, Steps: []app.FlowStep{{ID: "step"}}})
		if err != nil {
			t.Fatal(err)
		}
		if name == "Corrupt" {
			corruptID = row.ID
		}
	}
	if _, err := store.DB().ExecContext(ctx, `UPDATE flow_steps SET element_ids = '["bad-id"]' WHERE flow_id = ?`, corruptID); err != nil {
		t.Fatal(err)
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	if resp, err := svc.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{})); connect.CodeOf(err) != connect.CodeInternal || resp != nil {
		t.Fatalf("partial corrupt list: %v %v", resp, err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM flows").Scan(&count); err != nil || count != 2 {
		t.Fatalf("read changed rows: %d %v", count, err)
	}
}

func TestValidateFlow(t *testing.T) {
	valid := func(steps ...*diagv1.FlowStep) *diagv1.SaveFlowRequest {
		return &diagv1.SaveFlowRequest{ViewId: 1, Content: &diagv1.FlowContent{Name: "Flow", Steps: steps}}
	}
	for _, req := range []*diagv1.SaveFlowRequest{{}, {ViewId: 1}, {Content: &diagv1.FlowContent{Name: "Flow"}}, {ViewId: 1, Content: &diagv1.FlowContent{Name: " "}}, {ViewId: 1, ExpectedRevision: 1, Content: &diagv1.FlowContent{Name: "Flow"}}, {Id: "saved", ViewId: 1, Content: &diagv1.FlowContent{Name: "Flow"}}} {
		if err := validateFlow(req); err == nil {
			t.Fatalf("accepted invalid request: %v", req)
		}
	}
	for _, step := range []*diagv1.FlowStep{nil, {Id: " "}, {Id: "s", ElementIds: []int32{0}}, {Id: "s", ElementIds: []int32{1, 1}}, {Id: "s", ConnectorIds: []int32{-1}}, {Id: "s", ConnectorIds: []int32{1, 1}}, {Id: "s", Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: " "}}, {Id: "s", Control: flowBranches(diagv1.FlowBranchMode_FLOW_BRANCH_MODE_UNSPECIFIED)}, {Id: "s", Control: flowBranches(99)}, {Id: "s", Control: flowBranches(diagv1.FlowBranchMode_FLOW_BRANCH_MODE_ALTERNATE, nil, &diagv1.FlowBranch{Id: "b", Name: "B"})}} {
		if err := validateFlow(valid(step)); err == nil {
			t.Fatalf("accepted invalid step: %v", step)
		}
	}
	if err := validateFlow(valid(&diagv1.FlowStep{Id: "same"}, &diagv1.FlowStep{Id: "same"})); err == nil {
		t.Fatal("accepted duplicate step IDs")
	}
	for _, steps := range [][]*diagv1.FlowStep{nil, {{Id: "text"}}, {{Id: "mixed", ElementIds: []int32{99}, ConnectorIds: []int32{88}, Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: "missing"}}}} {
		if err := validateFlow(valid(steps...)); err != nil {
			t.Fatal(err)
		}
	}
	// oneof rejects conflicting controls at the wire boundary.
	if err := protojson.Unmarshal([]byte(`{"id":"s","branches":{},"goToFlowId":"other"}`), &diagv1.FlowStep{}); err == nil {
		t.Fatal("accepted conflicting controls")
	}
}

func TestFlowValidationBoundaries(t *testing.T) {
	valid := func() *diagv1.SaveFlowRequest {
		return &diagv1.SaveFlowRequest{ViewId: 1, Content: &diagv1.FlowContent{Name: "Flow", Steps: []*diagv1.FlowStep{{Id: "step"}}}}
	}
	fields := []struct {
		name  string
		limit int
		set   func(*diagv1.SaveFlowRequest, string)
	}{
		{"name", 200, func(r *diagv1.SaveFlowRequest, v string) { r.Content.Name = v }},
		{"description", 10000, func(r *diagv1.SaveFlowRequest, v string) { r.Content.Description = v }},
		{"step id", 100, func(r *diagv1.SaveFlowRequest, v string) { r.Content.Steps[0].Id = v }},
		{"step title", 500, func(r *diagv1.SaveFlowRequest, v string) { r.Content.Steps[0].Title = v }},
		{"step description", 10000, func(r *diagv1.SaveFlowRequest, v string) { r.Content.Steps[0].Description = v }},
		{"linked flow", 100, func(r *diagv1.SaveFlowRequest, v string) {
			r.Content.Steps[0].Control = &diagv1.FlowStep_GoToFlowId{GoToFlowId: v}
		}},
		{"path id", 100, func(r *diagv1.SaveFlowRequest, v string) {
			r.Content.Steps[0].Control = flowBranches(1, &diagv1.FlowBranch{Id: v, Name: "A"}, &diagv1.FlowBranch{Id: "b", Name: "B"})
		}},
		{"path name", 200, func(r *diagv1.SaveFlowRequest, v string) {
			r.Content.Steps[0].Control = flowBranches(1, &diagv1.FlowBranch{Id: "a", Name: v}, &diagv1.FlowBranch{Id: "b", Name: "B"})
		}},
	}
	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				req := valid()
				size := f.limit + extra
				f.set(req, strings.Repeat("界", size/3)+strings.Repeat("x", size%3))
				err := validateFlow(req)
				if (err != nil) != (extra == 1) {
					t.Fatalf("%d bytes: %v", size, err)
				}
			}
		})
	}
	for _, connector := range []bool{false, true} {
		req := valid()
		ids := make([]int32, 500)
		for i := range ids {
			ids[i] = int32(i + 1)
		}
		if connector {
			req.Content.Steps[0].ConnectorIds = ids
		} else {
			req.Content.Steps[0].ElementIds = ids
		}
		if err := validateFlow(req); err != nil {
			t.Fatal(err)
		}
		if connector {
			req.Content.Steps[0].ConnectorIds = append(ids, 501)
		} else {
			req.Content.Steps[0].ElementIds = append(ids, 501)
		}
		if err := validateFlow(req); err == nil {
			t.Fatal("accepted too many highlights")
		}
	}
	req := valid()
	req.Content.Steps = nil
	for i := 0; i < maxFlowSteps; i++ {
		req.Content.Steps = append(req.Content.Steps, &diagv1.FlowStep{Id: fmt.Sprintf("s%d", i)})
	}
	if err := validateFlow(req); err != nil {
		t.Fatal(err)
	}
	req.Content.Steps = append(req.Content.Steps, &diagv1.FlowStep{Id: "extra"})
	if err := validateFlow(req); err == nil {
		t.Fatal("accepted too many steps")
	}
	branch := &diagv1.FlowBranches{Mode: 1}
	req = valid()
	req.Content.Steps[0].Control = &diagv1.FlowStep_Branches{Branches: branch}
	for i := 0; i < maxFlowPaths; i++ {
		branch.Paths = append(branch.Paths, &diagv1.FlowBranch{Id: fmt.Sprintf("p%d", i), Name: "Path"})
	}
	if err := validateFlow(req); err != nil {
		t.Fatal(err)
	}
	branch.Paths = append(branch.Paths, &diagv1.FlowBranch{Id: "extra", Name: "Extra"})
	if err := validateFlow(req); err == nil {
		t.Fatal("accepted too many paths")
	}
	steps := []*diagv1.FlowStep{{Id: "leaf"}}
	for i := 0; i <= maxFlowBranchDepth; i++ {
		steps = []*diagv1.FlowStep{{Id: fmt.Sprintf("s%d", i), Control: flowBranches(1, &diagv1.FlowBranch{Id: fmt.Sprintf("a%d", i), Name: "A", Steps: steps}, &diagv1.FlowBranch{Id: fmt.Sprintf("b%d", i), Name: "B"})}}
		req.Content.Steps = steps
		err := validateFlow(req)
		if (err != nil) != (i == maxFlowBranchDepth) {
			t.Fatalf("nesting %d: %v", i, err)
		}
	}
}

func TestFlowContentSerializedSizeBoundary(t *testing.T) {
	store, _ := newSQLiteWorkspaceClientWithStore(t)
	ctx := context.Background()
	view, err := store.CreateView(ctx, "Limit", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := &diagv1.FlowContent{Name: "Limit"}
	for i := 0; i < 104; i++ {
		content.Steps = append(content.Steps, &diagv1.FlowStep{Id: fmt.Sprintf("s%d", i), Description: strings.Repeat("x", 10000)})
	}
	last := &diagv1.FlowStep{Id: "last", Description: "x"}
	content.Steps = append(content.Steps, last)
	encoded, err := protojson.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	last.Description += strings.Repeat("x", (1<<20)-len(encoded))
	encoded, err = protojson.Marshal(content)
	if err != nil || len(encoded) != 1<<20 {
		t.Fatalf("invalid exact-limit fixture: %d %v", len(encoded), err)
	}
	svc := &FlowService{Store: NewAPIStore(store)}
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{ViewId: int32(view.ID), Content: proto.Clone(content).(*diagv1.FlowContent)})); err != nil {
		t.Fatalf("rejected 1 MiB content: %v", err)
	}
	last.Description += "x"
	if _, err := svc.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{ViewId: int32(view.ID), Content: content})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("accepted oversize content: %v", err)
	}
}

func TestFlowServiceTreeCRUDWire(t *testing.T) {
	for _, jsonWire := range []bool{false, true} {
		t.Run(map[bool]string{false: "protobuf", true: "JSON"}[jsonWire], func(t *testing.T) {
			store, _ := newSQLiteWorkspaceClientWithStore(t)
			ctx := t.Context()
			view, err := store.CreateView(ctx, "Wire CRUD", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			hooks := &flowTestHooks{}
			_, handler := diagv1connect.NewFlowServiceHandler(&FlowService{Store: NewAPIStore(store), Hooks: hooks})
			server := httptest.NewServer(handler)
			defer server.Close()
			var opts []connect.ClientOption
			if jsonWire {
				opts = append(opts, connect.WithProtoJSON())
			}
			client := diagv1connect.NewFlowServiceClient(server.Client(), server.URL, opts...)
			content := &diagv1.FlowContent{Name: "Wire flow", Description: "Preserve description", Steps: []*diagv1.FlowStep{
				{Id: "first", Title: "First", ElementIds: []int32{101, 102}, ConnectorIds: []int32{201, 202}},
				{Id: "branch", Control: flowBranches(diagv1.FlowBranchMode_FLOW_BRANCH_MODE_ALTERNATE,
					&diagv1.FlowBranch{Id: "empty", Name: "Empty"},
					&diagv1.FlowBranch{Id: "path", Name: "Work", Steps: []*diagv1.FlowStep{{Id: "linked", Control: &diagv1.FlowStep_GoToFlowId{GoToFlowId: "missing-target"}}}})},
				{Id: "last", Description: "Text only"},
			}}
			created, err := client.SaveFlow(ctx, connect.NewRequest(&diagv1.SaveFlowRequest{ViewId: int32(view.ID), Content: content}))
			if err != nil {
				t.Fatal(err)
			}
			check := func(want *diagv1.Flow) {
				t.Helper()
				listed, err := client.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: int32(view.ID)}))
				if err != nil || len(listed.Msg.Flows) != 1 || !proto.Equal(listed.Msg.Flows[0], want) {
					t.Fatalf("wire list differs: %v %v", listed, err)
				}
			}
			check(created.Msg.Flow)
			changed := proto.Clone(created.Msg.Flow).(*diagv1.Flow)
			changed.Content.Name = "Updated"
			changed.Content.Description = "Changed"
			changed.Content.Steps[0], changed.Content.Steps[2] = changed.Content.Steps[2], changed.Content.Steps[0]
			changed.Content.Steps[2].ConnectorIds = []int32{999}
			changed.Content.Steps[2].ElementIds = nil
			branch := changed.Content.Steps[1].GetBranches()
			branch.Mode = diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL
			branch.Paths[0], branch.Paths[1] = branch.Paths[1], branch.Paths[0]
			branch.Paths[0].Name = "Renamed"
			branch.Paths[1].Steps = []*diagv1.FlowStep{{Id: "new-child", Title: "New nested step"}}
			changed.Content.Steps = append(changed.Content.Steps, &diagv1.FlowStep{Id: "added", Title: "New root"})
			updated, err := client.SaveFlow(ctx, connect.NewRequest(flowSaveRequest(changed)))
			if err != nil {
				t.Fatal(err)
			}
			changed.Revision = 2
			if !proto.Equal(updated.Msg.Flow, changed) {
				t.Fatalf("updated response differs: %v", updated)
			}
			check(changed)
			// A path write error must roll back the envelope, removed old tree and all
			// newly inserted steps, and must not emit an AfterWrite success event.
			if _, err := store.DB().ExecContext(ctx, `CREATE TRIGGER reject_api_path BEFORE INSERT ON flow_paths WHEN NEW.name = 'reject' BEGIN SELECT RAISE(ABORT, 'injected path failure'); END`); err != nil {
				t.Fatal(err)
			}
			rejected := proto.Clone(updated.Msg.Flow).(*diagv1.Flow)
			rejected.Content.Name = "Must roll back"
			rejected.Content.Steps[1].GetBranches().Paths[0].Name = "reject"
			before := len(hooks.events)
			if _, err := client.SaveFlow(ctx, connect.NewRequest(flowSaveRequest(rejected))); connect.CodeOf(err) != connect.CodeInternal {
				t.Fatalf("write failure status: %v", err)
			}
			if len(hooks.events) != before {
				t.Fatal("failed save emitted success event")
			}
			check(changed)
			request := flowSaveRequest(rejected)
			request.Id = ""
			request.ExpectedRevision = 0
			if _, err := client.SaveFlow(ctx, connect.NewRequest(request)); connect.CodeOf(err) != connect.CodeInternal {
				t.Fatalf("create failure status: %v", err)
			}
			check(changed)
			if _, err := client.SaveFlow(ctx, connect.NewRequest(flowSaveRequest(created.Msg.Flow))); connect.CodeOf(err) != connect.CodeAborted {
				t.Fatalf("stale update: %v", err)
			}
			if _, err := client.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: changed.Id, ViewId: changed.ViewId, ExpectedRevision: 1})); connect.CodeOf(err) != connect.CodeAborted {
				t.Fatalf("stale delete: %v", err)
			}
			// Clearing and restoring the tree exercises removal and recreation of all
			// nested rows without changing the flow identity.
			empty := proto.Clone(changed).(*diagv1.Flow)
			empty.Content.Steps = nil
			cleared, err := client.SaveFlow(ctx, connect.NewRequest(flowSaveRequest(empty)))
			if err != nil {
				t.Fatal(err)
			}
			empty.Revision = 3
			check(empty)
			restore := flowSaveRequest(cleared.Msg.Flow)
			restore.Content = content
			restored, err := client.SaveFlow(ctx, connect.NewRequest(restore))
			if err != nil {
				t.Fatal(err)
			}
			check(restored.Msg.Flow)
			if _, err := client.DeleteFlow(ctx, connect.NewRequest(&diagv1.DeleteFlowRequest{Id: restored.Msg.Flow.Id, ViewId: int32(view.ID), ExpectedRevision: 4})); err != nil {
				t.Fatal(err)
			}
			listed, err := client.ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{}))
			if err != nil || len(listed.Msg.Flows) != 0 {
				t.Fatalf("deleted flow remains: %v %v", listed, err)
			}
			for _, table := range []string{"flows", "flow_steps", "flow_paths"} {
				var count int
				if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("%s after delete: %d %v", table, count, err)
				}
			}
		})
	}
}
