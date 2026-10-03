package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWatchControlLifecycle(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	if _, ok, err := idx.WatchState(ctx, "repo"); err != nil || ok {
		t.Fatalf("unexpected initial state: ok=%v err=%v", ok, err)
	}
	claim := WatchState{RepositoryID: "repo", OwnerKind: "cli", OwnerID: "owner-a", OwnerPID: 111, State: "watching", RepoRoot: "/repo"}
	if err := idx.ClaimWatch(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err := idx.ClaimWatch(ctx, WatchState{RepositoryID: "repo", OwnerKind: "server", OwnerID: "owner-b"}); !errors.Is(err, ErrWatchActive) {
		t.Fatalf("second watcher should be rejected: %v", err)
	}
	if err := idx.ClaimWatch(ctx, claim); err != nil {
		t.Fatalf("same owner should be able to re-claim: %v", err)
	}
	state, ok, err := idx.WatchState(ctx, "repo")
	if err != nil || !ok {
		t.Fatalf("state: ok=%v err=%v", ok, err)
	}
	if !state.Fresh(time.Now()) || state.State != "watching" || state.OwnerPID != 111 {
		t.Fatalf("state: %+v", state)
	}
	if stale, err := idx.WatchStopRequested(ctx, "repo"); err != nil || stale {
		t.Fatalf("stop requested prematurely: %v %v", stale, err)
	}
	if err := idx.RequestWatchStop(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	requested, err := idx.WatchStopRequested(ctx, "repo")
	if err != nil || !requested {
		t.Fatalf("stop requested: %v %v", requested, err)
	}
	// A stop request lets another watcher take over.
	if err := idx.ClaimWatch(ctx, WatchState{RepositoryID: "repo", OwnerKind: "server", OwnerID: "owner-b"}); err != nil {
		t.Fatalf("claim after stop: %v", err)
	}
	// Releasing a different owner must not clobber the new watcher.
	if err := idx.ReleaseWatch(ctx, "repo", "owner-a"); err != nil {
		t.Fatal(err)
	}
	state, ok, err = idx.WatchState(ctx, "repo")
	if err != nil || !ok || state.OwnerID != "owner-b" {
		t.Fatalf("release clobbered owner: %+v %v", state, err)
	}
	if err := idx.ReleaseWatch(ctx, "repo", "owner-b"); err != nil {
		t.Fatal(err)
	}
	state, _, _ = idx.WatchState(ctx, "repo")
	if state.Fresh(time.Now()) || state.State != "stopped" || state.StopRequested {
		t.Fatalf("released state: %+v", state)
	}
}
