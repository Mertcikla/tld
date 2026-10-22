package api_test

import (
	"context"
	"fmt"
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

func TestPlanHandoffSurvivesWorkspaceChangesAndRestart(t *testing.T) {
	db := filepath.Join(t.TempDir(), "handoff.db")
	notes := t.TempDir()
	open := func() (*app.Store, *api.PlanService) {
		st, err := app.OpenStore(db, assets.FS)
		if err != nil {
			t.Fatal(err)
		}
		svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx, notes) }}
		return st, svc
	}
	st, svc := open()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	p, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo", Name: "Feature", Objective: "Add bounded retries"}))
	if err != nil {
		t.Fatal(err)
	}
	id := p.Msg.Plan.Id
	call := func(method string, body map[string]any) map[string]any {
		t.Helper()
		out, e := svc.PlanWorkspace(ctx, id, method, body)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	element := call("CreateElement", map[string]any{"name": "Original API"})["element"].(map[string]any)
	view := call("CreateView", map[string]any{"name": "Architecture"})["view"].(map[string]any)
	call("CreateViewMarkdown", map[string]any{"view_id": view["id"], "initial_content": "Original requirement", "target_kind": "PRIVATE_APP"})
	p, err = svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if err != nil {
		t.Fatal(err)
	}
	locked, err := svc.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: id, ExpectedRevision: p.Msg.Plan.Revision, GitRevision: "baseline-sha"}))
	if err != nil {
		t.Fatal(err)
	}
	p, err = svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if err != nil {
		t.Fatal(err)
	}
	revision := p.Msg.Plan.ReadyRevision
	if p.Msg.Plan.Status != "ready" || revision != p.Msg.Plan.Revision {
		t.Fatal("handoff not automatically ready", p.Msg.Plan)
	}
	original, err := svc.GetPrompt(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id, Revision: &revision}))
	if err != nil {
		t.Fatal(err)
	}
	adapter := store.NewAPIAdapter(st, notes)
	if _, err = adapter.UpdateElement(ctx, int32(element["id"].(float64)), uuid.Nil, api.ElementInput{Name: "Changed outside the plan"}); err != nil {
		t.Fatal(err)
	}
	viewID := int32(view["id"].(float64))
	if _, err = adapter.SaveViewMarkdown(ctx, viewID, uuid.Nil, "Changed requirement", nil, true); err != nil {
		t.Fatal(err)
	}
	// An unavailable checkout must not trap the workspace in its lock.
	svc.RepositoryRevision = func(context.Context, string) (string, error) { return "", fmt.Errorf("checkout removed") }
	current, err := svc.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{}))
	if err != nil || !current.Msg.Locked {
		t.Fatal("recovery unavailable", err)
	}
	_, err = svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: locked.Msg.Revision, Action: "finish"}))
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	st, svc = open()
	defer func() { _ = st.Close() }()
	restored, err := svc.GetPrompt(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id, Revision: &revision}))
	if err != nil || restored.Msg.Text != original.Msg.Text {
		t.Fatal("frozen prompt changed", err)
	}
	if !strings.Contains(restored.Msg.Text, "baseline-sha") || !strings.Contains(restored.Msg.Text, "Original API") || !strings.Contains(restored.Msg.Text, "--revision") {
		t.Fatal(restored.Msg.Text)
	}
	note, err := svc.GetNote(ctx, connect.NewRequest(&pb.PlanNoteRequest{PlanId: id, ViewKey: fmt.Sprintf("view:%d", viewID), Revision: &revision}))
	if err != nil || note.Msg.Note.Content != "Original requirement" {
		t.Fatal("note was not frozen", note, err)
	}
	p, err = svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: id}))
	if err != nil || p.Msg.Plan.Status != "completed" {
		t.Fatal("plan did not finish", p, err)
	}
	completed, err := svc.GetProjection(ctx, connect.NewRequest(&pb.PlanProjectionRequest{PlanId: id, FullWorkspace: true}))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range completed.Msg.Resources {
		if e := r.Resource.GetElement(); e != nil && e.Id == int32(element["id"].(float64)) && e.Name != "Original API" {
			t.Fatal("completed plan lost its frozen specification", e)
		}
	}
}

func TestImplementationPreservedScopeAndFinishRecovery(t *testing.T) {
	st, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	p, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	element, err := svc.PlanWorkspace(ctx, p.Msg.Plan.Id, "CreateElement", map[string]any{"name": "Preserved API"})
	if err != nil {
		t.Fatal(err)
	}
	elementID := int32(element["element"].(map[string]any)["id"].(float64))
	p, err = svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: p.Msg.Plan.Id}))
	if err != nil {
		t.Fatal(err)
	}
	p, err = svc.AppendOperations(ctx, connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: p.Msg.Plan.Id, ExpectedRevision: p.Msg.Plan.Revision, RequestId: "preserve", Operations: []*pb.PlanOperation{{Action: &pb.PlanOperation_Include{Include: &pb.PlanMember{ResourceKey: fmt.Sprintf("element:%d", elementID), Role: "preserve", Explicit: true}}}}}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id, ExpectedRevision: p.Msg.Plan.Revision}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.PlanWorkspace(ctx, "", "UpdateElement", map[string]any{"element_id": float64(elementID), "name": "Changed API"}); err != nil {
		t.Fatal(err)
	}
	state, err := svc.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: state.Msg.Revision, Action: "finish"}))
	if err != nil || !blocked.Msg.Locked || blocked.Msg.Edits[0].Status != "warning" || !strings.Contains(blocked.Msg.Edits[0].Reason, "preserve") {
		t.Fatal("preserve constraint bypassed", blocked, err)
	}
	// A stale reviewer cannot discard a newer ledger.
	_, err = svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: state.Msg.Revision, Action: "cancel"}))
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Fatal("stale cancellation accepted", err)
	}
	ended, err := svc.ReviewImplementation(ctx, connect.NewRequest(&pb.ReviewImplementationRequest{RepositoryId: "repo", ExpectedRevision: blocked.Msg.Revision, Action: "cancel"}))
	if err != nil || ended.Msg.Locked || ended.Msg.Edits[0].Status != "discarded" {
		t.Fatal("could not recover", ended, err)
	}
	live, err := store.NewAPIAdapter(st).GetElement(ctx, elementID, uuid.Nil)
	if err != nil || live.Name != "Preserved API" {
		t.Fatal("discard changed live state", live, err)
	}
	p, err = svc.GetPlan(ctx, connect.NewRequest(&pb.GetPlanRequest{PlanId: p.Msg.Plan.Id}))
	if err != nil || p.Msg.Plan.Status != "draft" {
		t.Fatal("cancel did not reopen draft", err)
	}
	// An intentional new implementation creates a fresh revision without replacing the old handoff.
	_, err = svc.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id, ExpectedRevision: p.Msg.Plan.Revision}))
	if err != nil {
		t.Fatal(err)
	}
}
