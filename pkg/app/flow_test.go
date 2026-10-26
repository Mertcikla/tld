package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestFlowsRevisionIsolationAndCascade(t *testing.T) {
	store := openAppStore(t)
	ctxA := WithTenantOrgID(context.Background(), uuid.New())
	ctxB := WithTenantOrgID(context.Background(), uuid.New())
	view, err := store.CreateView(ctxA, "Checkout", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.SaveFlow(ctxA, FlowRecord{ViewID: view.ID, Document: `{"name":"Checkout"}`})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.Revision != 1 {
		t.Fatalf("invalid saved flow: %+v", saved)
	}
	rows, err := store.ListFlows(ctxB, 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("other scope can read flows: %v, %v", rows, err)
	}
	if _, err := store.SaveFlow(ctxB, saved); err == nil {
		t.Fatal("other scope updated flow")
	}
	if err := store.DeleteFlow(ctxB, saved.ID, view.ID, saved.Revision); err == nil {
		t.Fatal("other scope deleted flow")
	}
	updated, err := store.SaveFlow(ctxA, saved)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if _, err := store.SaveFlow(ctxA, saved); !errors.Is(err, ErrFlowConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if err := store.DeleteFlow(ctxA, saved.ID, view.ID, saved.Revision); !errors.Is(err, ErrFlowConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err := store.DeleteView(ctxA, view.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = store.ListFlows(ctxA, 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("cascade: %v, %v", rows, err)
	}
}
