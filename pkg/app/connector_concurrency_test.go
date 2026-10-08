package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/uptrace/bun"
)

// blockConnectorUpdateHook blocks the first UPDATE on the connectors table so a
// test can deterministically interleave two UpdateConnector calls between one
// caller's read and its write.
type blockConnectorUpdateHook struct {
	mu      sync.Mutex
	fired   bool
	reached chan struct{}
	release chan struct{}
}

func (h *blockConnectorUpdateHook) BeforeQuery(ctx context.Context, ev *bun.QueryEvent) context.Context {
	if ev.Operation() != "UPDATE" || !strings.Contains(ev.Query, "connectors") {
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

func (h *blockConnectorUpdateHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func stringPtr(s string) *string { return &s }

func TestUpdateConnectorDoesNotClobberConcurrentFieldUpdates(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	view, err := store.CreateView(ctx, "Race View", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateElement(ctx, LibraryElement{Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateElement(ctx, LibraryElement{Name: "Target"})
	if err != nil {
		t.Fatal(err)
	}

	connector, err := store.CreateConnector(ctx, Connector{
		ViewID:          view.ID,
		SourceElementID: source.ID,
		TargetElementID: target.ID,
		Label:           stringPtr("label-0"),
		Description:     stringPtr("description-0"),
		Direction:       "forward",
		Style:           "bezier",
	})
	if err != nil {
		t.Fatal(err)
	}

	hook := &blockConnectorUpdateHook{reached: make(chan struct{}), release: make(chan struct{})}
	store.BunDB().AddQueryHook(hook)

	labelErr := make(chan error, 1)
	go func() {
		// Reads the row (label-0, description-0), then blocks just before its UPDATE.
		_, err := store.UpdateConnector(ctx, connector.ID, Connector{Label: stringPtr("label-1")})
		labelErr <- err
	}()

	<-hook.reached

	// A concurrent update to a different field completes while the label update
	// is paused between its read and write.
	if _, err := store.UpdateConnector(ctx, connector.ID, Connector{Description: stringPtr("description-1")}); err != nil {
		t.Fatal(err)
	}

	close(hook.release)
	if err := <-labelErr; err != nil {
		t.Fatal(err)
	}

	final, err := store.ConnectorByID(ctx, connector.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Label == nil || *final.Label != "label-1" {
		t.Fatalf("label = %v, want label-1", final.Label)
	}
	if final.Description == nil || *final.Description != "description-1" {
		t.Fatalf("description = %v, want description-1 (concurrent update was clobbered)", final.Description)
	}
}
