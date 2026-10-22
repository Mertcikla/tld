package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestImplementationLockStagesWithoutFlagAndCommitsIncrementally(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	p, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Name: "Implementation"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id, GitRevision: "first"}))
	if err != nil {
		t.Fatal(err)
	}
	handler := svc.PlanMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("locked request escaped middleware") }))
	call := func(method, body, requestID string) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/diag.v1.WorkspaceService/"+method, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tld-Request-Id", requestID)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("%s: %s", method, w.Body.String())
		}
		out := map[string]any{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := call("CreateElement", `{"name":"Agent API"}`, "create")
	id := out["element"].(map[string]any)["id"].(float64)
	if id < 1000000000 {
		t.Fatalf("no stable virtual ID: %v", out)
	}
	again := call("CreateElement", `{"name":"Agent API"}`, "create")
	if again["element"].(map[string]any)["id"] != id {
		t.Fatal("retry duplicated edit")
	}
	body, _ := json.Marshal(map[string]any{"element_id": id, "name": "Updated API"})
	call("UpdateElement", string(body), "update")
	elems, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil || len(elems) != 0 {
		t.Fatalf("live workspace changed: %v %v", elems, err)
	}
	state, err := svc.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{RepositoryId: "repo", GitRevision: "second"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Msg.Edits) != 2 || state.Msg.PendingCommit != "second" {
		t.Fatalf("queue or commit confirmation lost: %v", state.Msg)
	}
	result, err := svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: state.Msg.Revision, Action: "commit"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Msg.Edits[0].Status != "committed" || result.Msg.Edits[1].Status != "committed" {
		t.Fatalf("failed replay: %v", result.Msg)
	}
	elems, _, err = st.Elements(ctx, 0, 0, "")
	if err != nil || len(elems) != 1 || elems[0].Name != "Updated API" {
		t.Fatalf("wrong replay result: %v %v", elems, err)
	}
	if result.Msg.PendingCommit != "" {
		t.Fatal("confirmation not acknowledged")
	}
}

func TestImplementationConnectorEndpointDriftAndSkip(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	adapter := store.NewAPIAdapter(st)
	svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	a, err := adapter.CreateElement(ctx, uuid.Nil, api.ElementInput{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := adapter.CreateElement(ctx, uuid.Nil, api.ElementInput{Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	// Use a normal handler to create a view before locking.
	view, err := svc.PlanWorkspace(ctx, p.Msg.Plan.Id, "CreateView", map[string]any{"name": "System"})
	if err != nil {
		t.Fatal(err)
	}
	viewID := view["view"].(map[string]any)["id"]
	p, err = svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: p.Msg.Plan.Id}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id, ExpectedRevision: p.Msg.Plan.Revision}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.PlanWorkspace(ctx, p.Msg.Plan.Id, "CreateConnector", map[string]any{"view_id": viewID, "source_element_id": float64(a.Id), "target_element_id": float64(b.Id), "label": "uses"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.UpdateElement(ctx, a.Id, uuid.Nil, api.ElementInput{Name: "Changed A"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := svc.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: state.Msg.Revision, Action: "commit"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Msg.Edits[0].Status != "warning" {
		t.Fatalf("endpoint drift not detected: %v", result.Msg)
	}
	result, err = svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: result.Msg.Revision, Action: "ignore", OperationId: result.Msg.Edits[0].Id, ObservedHash: result.Msg.Edits[0].ObservedHash}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Msg.Edits[0].Status != "committed" {
		t.Fatalf("ignore did not proceed: %v", result.Msg)
	}
}

func TestImplementationPartialReplayFailureSkipAndDiscard(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	adapter := store.NewAPIAdapter(st)
	service := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	a, err := adapter.CreateElement(ctx, uuid.Nil, api.ElementInput{Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id, GitRevision: "before"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		method string
		body   map[string]any
	}{
		{"CreateElement", map[string]any{"name": "First success"}},
		{"UpdateElement", map[string]any{"element_id": float64(a.Id), "name": "Will fail"}},
		{"CreateElement", map[string]any{"name": "Last success"}},
	} {
		if _, err := service.PlanWorkspace(ctx, p.Msg.Plan.Id, input.method, input.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := adapter.DeleteElement(ctx, a.Id, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	get := func() *pb.ImplementationResponse {
		t.Helper()
		r, err := service.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{RepositoryId: "repo"}))
		if err != nil {
			t.Fatal(err)
		}
		return r.Msg
	}
	review := func(action string) *pb.ImplementationResponse {
		t.Helper()
		v := get()
		req := &pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: v.Revision, Action: action}
		for _, e := range v.Edits {
			if e.Status != "committed" && e.Status != "skipped" && e.Status != "discarded" {
				req.OperationId = e.Id
				req.ObservedHash = e.ObservedHash
				break
			}
		}
		r, err := service.ReviewImplementation(ctx, connect.NewRequest(req))
		if err != nil {
			t.Fatal(err)
		}
		return r.Msg
	}
	warned := review("commit")
	if warned.Edits[0].Status != "committed" || warned.Edits[1].Status != "warning" {
		t.Fatalf("earlier success lost: %v", warned)
	}
	failed := review("ignore")
	if failed.Edits[1].Status != "error" {
		t.Fatalf("missing endpoint should fail structurally: %v", failed)
	}
	review("skip")
	done := review("commit")
	if done.Edits[2].Status != "committed" {
		t.Fatalf("could not continue after skip: %v", done)
	}
	elems, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil || len(elems) != 2 {
		t.Fatalf("incorrect incremental replay: %v %v", elems, err)
	}
	_, err = service.PlanWorkspace(ctx, p.Msg.Plan.Id, "CreateElement", map[string]any{"name": "Discard me"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{RepositoryId: "repo", GitRevision: "after"}))
	if err != nil {
		t.Fatal(err)
	}
	if state.Msg.PendingCommit != "after" {
		t.Fatal("new commit did not request confirmation")
	}
	denied := review("discard")
	if denied.PendingCommit != "" || denied.GitRevision != "after" || denied.Edits[3].Status != "discarded" {
		t.Fatalf("denial did not discard staged work: %v", denied)
	}
	_, err = service.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: state.Msg.Revision, Action: "unlock"}))
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("stale review accepted: %v", err)
	}
}

func TestImplementationBatchReturnsVirtualIdentities(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	service := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	p, err := service.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.PlanWorkspace(ctx, p.Msg.Plan.Id, "ApplyWorkspaceBatch", map[string]any{"elements": []any{map[string]any{"ref": "worker", "name": "Worker"}}})
	if err != nil {
		t.Fatal(err)
	}
	result := out["element_results"].([]any)[0].(map[string]any)
	id := result["id"].(float64)
	if id < 1000000000 {
		t.Fatalf("batch leaked temporary database ID: %v", out)
	}
	_, err = service.PlanWorkspace(ctx, p.Msg.Plan.Id, "UpdateElement", map[string]any{"element_id": id, "name": "Updated worker"})
	if err != nil {
		t.Fatal(err)
	}
	live, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil || len(live) != 0 {
		t.Fatalf("batch escaped staging: %v %v", live, err)
	}
	state, err := service.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	done, err := service.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: state.Msg.Revision, Action: "commit"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range done.Msg.Edits {
		if e.Status != "committed" {
			t.Fatalf("batch replay failed: %v", done.Msg)
		}
	}
	live, _, err = st.Elements(ctx, 0, 0, "")
	if err != nil || len(live) != 1 || live[0].Name != "Updated worker" {
		t.Fatalf("batch replay lost identity: %v %v", live, err)
	}
}
