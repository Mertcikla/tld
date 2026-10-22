package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestPlanLifecycleLiveEditsAndRetry(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	notesDir := t.TempDir()
	svc := &api.PlanService{Store: st, NotesDir: t.TempDir(), Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx, notesDir) }}
	created, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Name: "Feature", Resume: true}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.Plan.Id
	resumed, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Resume: true}))
	if err != nil || !resumed.Msg.Resumed || resumed.Msg.Plan.Id != id {
		t.Fatal("did not resume draft", err)
	}
	req := &pb.AppendPlanOperationsRequest{PlanId: id, RequestId: "retry-me", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Create{Create: &pb.PlanResource{Value: &pb.PlanResource_Element{Element: &pb.Element{Name: "Planned API"}}}}}}}
	saved, err := svc.AppendOperations(ctx, connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.AppendOperations(ctx, connect.NewRequest(req))
	if err != nil || again.Msg.Plan.Revision != saved.Msg.Plan.Revision {
		t.Fatal("retry duplicated operation", err)
	}
	elements, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(elements) != 1 || elements[0].Name != "Planned API" {
		t.Fatal("live edit missing or duplicated", elements)
	}
	proj, err := svc.GetProjection(ctx, connect.NewRequest(&pb.PlanProjectionRequest{PlanId: id}))
	if err != nil || len(proj.Msg.Resources) != 1 {
		t.Fatal("missing staged resource", err)
	}
	prompt, err := svc.GetPrompt(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if err != nil || prompt.Msg.Revision != 1 {
		t.Fatal("prompt revision", err)
	}
	_, err = svc.UpdatePlan(ctx, connect.NewRequest(&pb.UpdatePlanRequest{PlanId: id, ExpectedRevision: 0, Name: proto.String("Stale")}))
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Fatal("stale write accepted", err)
	}
	_, err = svc.UpdatePlan(ctx, connect.NewRequest(&pb.UpdatePlanRequest{PlanId: id, ExpectedRevision: 1, Status: proto.String("ready")}))
	if err != nil {
		t.Fatal(err)
	}
	next, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Resume: true}))
	if err != nil || next.Msg.Resumed || next.Msg.Plan.Id == id {
		t.Fatal("ready plan resumed", err)
	}
	other := api.WithWorkspaceID(context.Background(), uuid.New())
	_, err = svc.GetPlan(other, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal("plan leaked across workspaces", err)
	}
}

func TestPlanWorkspaceMovesNotesAndAtomicFailure(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	notesDir := t.TempDir()
	svc := &api.PlanService{Store: st, NotesDir: t.TempDir(), Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx, notesDir) }}
	created, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.Plan.Id
	call := func(method string, in map[string]any) map[string]any {
		t.Helper()
		out, e := svc.PlanWorkspace(ctx, id, method, in)
		if e != nil {
			t.Fatalf("%s: %v", method, e)
		}
		return out
	}
	v := call("CreateView", map[string]any{"name": "Draft view"})["view"].(map[string]any)
	e := call("CreateElement", map[string]any{"name": "Draft element"})["element"].(map[string]any)
	call("CreatePlacement", map[string]any{"view_id": v["id"], "element_id": e["id"], "position_x": float64(1)})
	call("UpdatePlacementPosition", map[string]any{"view_id": v["id"], "element_id": e["id"], "position_x": float64(99), "position_y": float64(42)})
	placements := call("ListPlacements", map[string]any{"view_id": v["id"]})["placements"].([]any)
	if len(placements) != 1 || placements[0].(map[string]any)["position_x"] != float64(99) {
		t.Fatal("move removed placement or lost position", placements)
	}
	call("CreateViewMarkdown", map[string]any{"view_id": v["id"], "initial_content": "# Staged schema", "target_kind": "PRIVATE_APP"})
	note := call("GetViewMarkdown", map[string]any{"view_id": v["id"]})
	if note["content"] != "# Staged schema" {
		t.Fatal(note)
	}
	before, err := svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: id, ExpectedRevision: before.Msg.Plan.Revision, RequestId: "atomic", Operations: []*pb.PlanOperation{
		{Action: &pb.PlanOperation_Create{Create: &pb.PlanResource{Value: &pb.PlanResource_Element{Element: &pb.Element{Name: "Must roll back"}}}}},
		{Action: &pb.PlanOperation_Include{Include: &pb.PlanMember{ResourceKey: "element:missing", Role: "preserve", Explicit: true}}},
	}}))
	if err == nil {
		t.Fatal("invalid batch accepted")
	}
	after, err := svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if err != nil {
		t.Fatal(err)
	}
	if after.Msg.Plan.Revision != before.Msg.Plan.Revision {
		t.Fatal("failed batch advanced revision")
	}
	live, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range live {
		if element.Name == "Must roll back" {
			t.Fatal("plan wrote live resource")
		}
	}
}

func TestPlanMetadataRevisionsAndIdempotencyPayload(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	notesDir := t.TempDir()
	svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx, notesDir) }}
	r, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Objective: "Original objective"}))
	if err != nil {
		t.Fatal(err)
	}
	id := r.Msg.Plan.Id
	_, err = svc.UpdatePlan(ctx, connect.NewRequest(&pb.UpdatePlanRequest{PlanId: id, ExpectedRevision: 0, Objective: proto.String("New objective")}))
	if err != nil {
		t.Fatal(err)
	}
	old, err := svc.GetPrompt(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id, Revision: proto.Uint32(0)}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(old.Msg.Text, "Original objective") || strings.Contains(old.Msg.Text, "New objective") {
		t.Fatal("historical prompt used current metadata")
	}
	request := &pb.AppendPlanOperationsRequest{PlanId: id, ExpectedRevision: 1, RequestId: "same-key", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Checklist{Checklist: &pb.PlanChecklistItem{Text: "First claim", VerificationKind: "manual"}}}}}
	if _, err = svc.AppendOperations(ctx, connect.NewRequest(request)); err != nil {
		t.Fatal(err)
	}
	request.Operations[0].GetChecklist().Text = "Different claim"
	if _, err = svc.AppendOperations(ctx, connect.NewRequest(request)); err == nil {
		t.Fatal("idempotency key reused with different contents")
	}
}

func TestPlanAppendContinuesAfterDeletedTarget(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	live := store.NewAPIAdapter(st)
	element, err := live.CreateElement(ctx, uuid.Nil, api.ElementInput{Name: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	notesDir := t.TempDir()
	svc := &api.PlanService{Store: st, NotesDir: t.TempDir(), Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx, notesDir) }}
	created, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Name: "Drift"}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.Plan.Id
	_, err = svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: id, RequestId: "include", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Include{Include: &pb.PlanMember{ResourceKey: fmt.Sprintf("element:%d", element.Id), Role: "change", Explicit: true}}}}}))
	if err != nil {
		t.Fatal(err)
	}
	if err = live.DeleteElement(ctx, element.Id, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, connect.NewRequest(&pb.PlanProjectionRequest{PlanId: id}))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Msg.Operations) != 1 || preview.Msg.Operations[0].Status != "recorded" || preview.Msg.WorkspaceHash != "" {
		t.Fatal(preview.Msg)
	}
	saved, err := svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: id, ExpectedRevision: 1, RequestId: "continue", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Checklist{Checklist: &pb.PlanChecklistItem{Text: "Keep working", VerificationKind: "manual"}}}}}))
	if err != nil || saved.Msg.Plan.Revision != 2 {
		t.Fatal("drift prevented appending", err)
	}
}

func TestPlanWorkspaceRecordsLiveWritesAndRetry(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	plan, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Name: "Live"}))
	if err != nil {
		t.Fatal(err)
	}
	handler := svc.PlanMiddleware(http.NotFoundHandler())
	call := func(method, body, key string) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/diag.v1.WorkspaceService/"+method, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tld-Plan", plan.Msg.Plan.Id)
		req.Header.Set("X-Tld-Request-Id", key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		out := map[string]any{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := call("CreateElement", `{"name":"Live service","tags":["plan:create","runtime"]}`, "create-once")
	again := call("CreateElement", `{"name":"Live service","tags":["plan:create","runtime"]}`, "create-once")
	if fmt.Sprint(first) != fmt.Sprint(again) {
		t.Fatal("retry changed response")
	}
	live, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil || len(live) != 1 {
		t.Fatal("write missing or duplicated", err, live)
	}
	ops, err := svc.ListOperations(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: plan.Msg.Plan.Id}))
	if err != nil || len(ops.Msg.Operations) != 1 {
		t.Fatal("record missing or duplicated", err)
	}
	if ops.Msg.Operations[0].GetCreate().GetElement().Id <= 0 {
		t.Fatal("plan-local alias instead of live ID")
	}
	if strings.Contains(fmt.Sprint(live[0].Tags), "plan:") {
		t.Fatal("derived tags persisted")
	}

	_, err = svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: plan.Msg.Plan.Id, ExpectedRevision: 1, RequestId: "metadata", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Update{Update: &pb.PlanPatch{Resource: &pb.PlanResource{Key: fmt.Sprintf("element:%d", live[0].ID), Value: &pb.PlanResource_Element{Element: &pb.Element{Technology: proto.String("Go")}}}, Fields: &fieldmaskpb.FieldMask{Paths: []string{"technology"}}}}}}}))
	if err != nil {
		t.Fatal(err)
	}
	updated, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil || len(updated) != 1 || updated[0].Name != "Live service" || fmt.Sprint(updated[0].Tags) != fmt.Sprint(live[0].Tags) {
		t.Fatal("metadata edit lost existing fields", err, updated)
	}
}

type planReadbackHooks struct {
	api.NopWorkspaceHooks
	store *app.Store
	calls int
	err   error
	count int
}

func (h *planReadbackHooks) AfterWrite(ctx context.Context, _ uuid.UUID, _, _, _ string, _ map[string]any, _ any) {
	h.calls++
	rows, _, err := h.store.Elements(ctx, 0, 0, "")
	h.err = err
	h.count = len(rows)
}
func TestPlanWriteHooksRunAfterCommit(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx, cancel := context.WithTimeout(api.WithWorkspaceID(context.Background(), uuid.Nil), 5*time.Second)
	defer cancel()
	hooks := &planReadbackHooks{store: st}
	svc := &api.PlanService{Store: st, Hooks: hooks, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	created, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.Plan.Id
	if _, err = svc.PlanWorkspace(ctx, id, "CreateElement", map[string]any{"name": "Live"}); err != nil {
		t.Fatal(err)
	}
	if hooks.err != nil || hooks.calls != 1 || hooks.count != 1 {
		t.Fatal("hook could not read committed write", hooks.err, hooks.calls, hooks.count)
	}
	_, err = svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: id, ExpectedRevision: 1, RequestId: "append", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Create{Create: &pb.PlanResource{Value: &pb.PlanResource_Element{Element: &pb.Element{Name: "Another"}}}}}}}))
	if err != nil || hooks.err != nil || hooks.calls != 2 || hooks.count != 2 {
		t.Fatal("append hook ran before commit", err, hooks.err, hooks.calls, hooks.count)
	}
	_, err = svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: id, ExpectedRevision: 2, RequestId: "rollback", Operations: []*pb.PlanOperation{
		{Action: &pb.PlanOperation_Create{Create: &pb.PlanResource{Value: &pb.PlanResource_Element{Element: &pb.Element{Name: "Rolled back"}}}}},
		{Action: &pb.PlanOperation_Include{Include: &pb.PlanMember{ResourceKey: "element:missing", Role: "preserve"}}},
	}}))
	if err == nil || hooks.calls != 2 {
		t.Fatal("failed transaction published a notification", err, hooks.calls)
	}
}
