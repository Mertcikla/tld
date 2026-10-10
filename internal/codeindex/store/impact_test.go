package store

import (
	"context"
	"errors"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
)

func TestLeaseAndImpactDeletion(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	if err := idx.Publish(ctx, "/repo", &pb.Snapshot{Id: "snap", RepositoryId: "repo"}, graph.NewGraph("repo", "snap")); err != nil {
		t.Fatal(err)
	}
	_, release, err := idx.AcquireLease(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	second := NewStoreFromHandle(handle)
	if _, _, err := second.AcquireLease(ctx, "repo"); !errors.Is(err, ErrBusy) {
		t.Fatalf("overlapping process lease allowed: %v", err)
	}
	release()
	_, release, err = second.AcquireLease(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err := idx.SaveImpact(ctx, &pb.ImpactDiagram{RepositoryId: "repo", ComparisonKey: "live", ViewId: 5}); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpsertWatchState(ctx, WatchState{
		RepositoryID: "repo", OwnerKind: "cli", OwnerID: "owner", State: "watching",
		Error: "retry", GitBranch: "main", GitRevision: "abc",
	}); err != nil {
		t.Fatal(err)
	}
	live, err := idx.LiveImpact(ctx, "repo")
	if err != nil || !live.Watching || live.Error != "retry" || live.Diagram.ViewId != 5 {
		t.Fatalf("live state: %+v %v", live, err)
	}
	if err := idx.DeleteRepository(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Impact(ctx, "repo", "live"); err == nil {
		t.Fatal("impact survives repository deletion")
	}
}
