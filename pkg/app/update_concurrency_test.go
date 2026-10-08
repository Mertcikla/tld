package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/uptrace/bun"
)

// firstStatementHook blocks the first query matching an operation and substring
// so tests can interleave two callers between a read and a write.
type firstStatementHook struct {
	mu      sync.Mutex
	fired   bool
	op      string
	substr  string
	reached chan struct{}
	release chan struct{}
}

func newFirstStatementHook(op, substr string) *firstStatementHook {
	return &firstStatementHook{op: op, substr: substr, reached: make(chan struct{}), release: make(chan struct{})}
}

func (h *firstStatementHook) BeforeQuery(ctx context.Context, ev *bun.QueryEvent) context.Context {
	if ev.Operation() != h.op || !strings.Contains(ev.Query, h.substr) {
		return ctx
	}
	h.mu.Lock()
	first := !h.fired
	h.fired = true
	h.mu.Unlock()
	if !first {
		return ctx
	}
	close(h.reached)
	<-h.release
	return ctx
}

func (h *firstStatementHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestUpdateViewDoesNotClobberConcurrentFieldUpdates(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	view, err := store.CreateView(ctx, "View", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateView(ctx, view.ID, nil, stringPtr("description-0"), nil, nil); err != nil {
		t.Fatal(err)
	}

	hook := newFirstStatementHook("UPDATE", "views")
	store.BunDB().AddQueryHook(hook)

	nameErr := make(chan error, 1)
	go func() {
		_, err := store.UpdateView(ctx, view.ID, stringPtr("name-1"), nil, nil, nil)
		nameErr <- err
	}()
	<-hook.reached

	if _, err := store.UpdateView(ctx, view.ID, nil, stringPtr("description-1"), nil, nil); err != nil {
		t.Fatal(err)
	}
	close(hook.release)
	if err := <-nameErr; err != nil {
		t.Fatal(err)
	}

	final, err := store.ViewByID(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Name != "name-1" {
		t.Fatalf("name = %q, want name-1", final.Name)
	}
	if final.Description == nil || *final.Description != "description-1" {
		t.Fatalf("description = %v, want description-1 (concurrent update was clobbered)", final.Description)
	}
}

func TestUpdateLayerDoesNotClobberConcurrentFieldUpdates(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	view, err := store.CreateView(ctx, "View", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := store.CreateLayer(ctx, view.ID, "layer-0", nil, stringPtr("#000"))
	if err != nil {
		t.Fatal(err)
	}

	hook := newFirstStatementHook("UPDATE", "view_layers")
	store.BunDB().AddQueryHook(hook)

	nameErr := make(chan error, 1)
	go func() {
		_, err := store.UpdateLayer(ctx, layer.ID, ViewLayer{Name: "layer-1"})
		nameErr <- err
	}()
	<-hook.reached

	if _, err := store.UpdateLayer(ctx, layer.ID, ViewLayer{Color: stringPtr("#fff")}); err != nil {
		t.Fatal(err)
	}
	close(hook.release)
	if err := <-nameErr; err != nil {
		t.Fatal(err)
	}

	final, err := store.LayerByID(ctx, layer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Name != "layer-1" {
		t.Fatalf("name = %q, want layer-1", final.Name)
	}
	if final.Color == nil || *final.Color != "#fff" {
		t.Fatalf("color = %v, want #fff (concurrent update was clobbered)", final.Color)
	}
}

func TestAdjustVisibilityOverrideDoesNotLoseConcurrentIncrements(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	view, err := store.CreateView(ctx, "View", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetVisibilityOverride(ctx, view.ID, "element", 1, 0); err != nil {
		t.Fatal(err)
	}

	hook := newFirstStatementHook("INSERT", "view_visibility_overrides")
	store.BunDB().AddQueryHook(hook)

	firstErr := make(chan error, 1)
	go func() {
		_, err := store.AdjustVisibilityOverride(ctx, view.ID, "element", 1, 1)
		firstErr <- err
	}()
	<-hook.reached

	if _, err := store.AdjustVisibilityOverride(ctx, view.ID, "element", 1, 1); err != nil {
		t.Fatal(err)
	}
	close(hook.release)
	if err := <-firstErr; err != nil {
		t.Fatal(err)
	}

	final, err := store.visibilityOverride(ctx, view.ID, "element", 1)
	if err != nil {
		t.Fatal(err)
	}
	if final.LevelDelta != 2 {
		t.Fatalf("level_delta = %d, want 2 (one increment was lost)", final.LevelDelta)
	}
}
