package app

import (
	"context"
	"errors"
	assets "github.com/mertcikla/tld/v2"
	"path/filepath"
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
	saved, err := store.SaveFlow(ctxA, FlowRecord{ViewID: view.ID, Name: "Checkout"})
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

func TestFlowInheritsHomeViewScope(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()
	owner, unrelated := uuid.New(), uuid.New()
	view, err := store.CreateView(WithTenantOrgID(ctx, owner), "Scoped", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// An unscoped administrative caller must not create a globally scoped flow
	// or override the home view's scope using the input record.
	saved, err := store.SaveFlow(ctx, FlowRecord{ViewID: view.ID, OrgID: &unrelated, Name: "Scoped flow"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.OrgID == nil || *saved.OrgID != owner {
		t.Fatalf("flow did not inherit view scope: %+v", saved)
	}
	rows, err := store.ListFlows(WithTenantOrgID(ctx, owner), view.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("owner cannot read flow: %v %v", rows, err)
	}
	rows, err = store.ListFlows(WithTenantOrgID(ctx, unrelated), 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("input scope leaked: %v %v", rows, err)
	}
	local, err := store.CreateView(ctx, "Local", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err = store.SaveFlow(ctx, FlowRecord{ViewID: local.ID, OrgID: &unrelated, Name: "Local flow"})
	if err != nil || saved.OrgID != nil {
		t.Fatalf("local flow inherited input scope: %+v %v", saved, err)
	}
}

func TestFlowTreeSurvivesDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flows.db")
	s, err := OpenStore(path, assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	view, err := s.CreateView(ctx, "Durable flows", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveFlow(ctx, FlowRecord{ViewID: view.ID, Name: "Durable", Steps: flowTreeFixture()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path, assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	rows, err := s.ListFlows(ctx, view.ID)
	if err != nil || len(rows) != 1 || rows[0].ID != saved.ID || rows[0].Revision != saved.Revision || !equalFlowTrees(rows[0].Steps, saved.Steps) {
		t.Fatalf("reopened flow differs: %v %v", rows, err)
	}
	updated, err := s.SaveFlow(ctx, rows[0])
	if err != nil || updated.Revision != 2 {
		t.Fatalf("reopened update: %v %v", updated, err)
	}
}
