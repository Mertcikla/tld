package store

import (
	"context"
	"errors"
	"os"
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
	claim := WatchState{RepositoryID: "repo", OwnerKind: "cli", OwnerID: "owner-a", OwnerPID: os.Getpid(), State: "watching", RepoRoot: "/repo"}
	if err := idx.ClaimWatch(ctx, claim); err != nil {
		t.Fatal(err)
	}
	// A live owner cannot be displaced even by a different owner.
	if err := idx.ClaimWatch(ctx, WatchState{RepositoryID: "repo", OwnerKind: "server", OwnerID: "owner-b"}); !errors.Is(err, ErrWatchActive) {
		t.Fatalf("second watcher should be rejected: %v", err)
	}
	// The same owner may re-claim its own record.
	if err := idx.ClaimWatch(ctx, claim); err != nil {
		t.Fatalf("same owner should be able to re-claim: %v", err)
	}
	state, ok, err := idx.WatchState(ctx, "repo")
	if err != nil || !ok {
		t.Fatalf("state: ok=%v err=%v", ok, err)
	}
	if !state.Live(time.Now()) || state.State != "watching" || state.OwnerPID != os.Getpid() {
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
	// A stop request alone does not let a second watcher start: the current
	// owner must exit or be reaped first, so two watchers never overlap.
	if err := idx.ClaimWatch(ctx, WatchState{RepositoryID: "repo", OwnerKind: "server", OwnerID: "owner-b", OwnerPID: os.Getpid()}); !errors.Is(err, ErrWatchActive) {
		t.Fatalf("live owner displaced by a stop request: %v", err)
	}
	// Releasing a different owner must not clobber the live watcher.
	if err := idx.ReleaseWatch(ctx, "repo", "owner-a"); err != nil {
		t.Fatal(err)
	}
	state, ok, err = idx.WatchState(ctx, "repo")
	if err != nil || !ok || state.OwnerID != "owner-a" {
		t.Fatalf("foreign release clobbered owner: %+v %v", state, err)
	}
	// A dead owner's stale row can be taken over immediately.
	if err := idx.ForceClearWatchState(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	if err := idx.ClaimWatch(ctx, WatchState{RepositoryID: "repo", OwnerKind: "server", OwnerID: "owner-b", OwnerPID: os.Getpid()}); err != nil {
		t.Fatalf("claim after clear: %v", err)
	}
	if err := idx.ReleaseWatch(ctx, "repo", "owner-b"); err != nil {
		t.Fatal(err)
	}
	state, _, _ = idx.WatchState(ctx, "repo")
	if state.Live(time.Now()) || state.State != "stopped" || state.StopRequested {
		t.Fatalf("released state: %+v", state)
	}
}

func TestWatchHeartbeatRequiresOwnership(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	base := WatchState{RepositoryID: "repo", OwnerKind: "cli", OwnerID: "owner-a", OwnerPID: os.Getpid(), State: "watching"}
	if err := idx.ClaimWatch(ctx, base); err != nil {
		t.Fatal(err)
	}
	base.State = "scanning"
	if err := idx.WatchHeartbeat(ctx, base); err != nil {
		t.Fatalf("owned heartbeat: %v", err)
	}
	// A heartbeat from a stale owner must be rejected, not silently applied.
	intruder := base
	intruder.OwnerID = "owner-b"
	if err := idx.WatchHeartbeat(ctx, intruder); !errors.Is(err, ErrWatchOwnershipLost) {
		t.Fatalf("foreign heartbeat should lose ownership: %v", err)
	}
	state, _, _ := idx.WatchState(ctx, "repo")
	if state.State != "scanning" || state.OwnerID != "owner-a" {
		t.Fatalf("foreign heartbeat mutated the row: %+v", state)
	}
}

func TestReapStaleWatchStates(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	// A watcher whose process no longer exists (pid 0 with a fresh heartbeat is
	// treated as live, so use an impossible high pid and a fresh heartbeat).
	st := WatchState{RepositoryID: "repo", OwnerKind: "cli", OwnerID: "owner", OwnerPID: 1 << 30, State: "scanning"}
	if err := idx.UpsertWatchState(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := idx.RequestWatchStop(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	if err := idx.ReapStaleWatchStates(ctx); err != nil {
		t.Fatal(err)
	}
	state, ok, err := idx.WatchState(ctx, "repo")
	if err != nil || !ok {
		t.Fatalf("state: %v %v", ok, err)
	}
	if state.Live(time.Now()) || state.StopRequested || state.State != "stopped" {
		t.Fatalf("stale watcher not reaped: %+v", state)
	}
}
