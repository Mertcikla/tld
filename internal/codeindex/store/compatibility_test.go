package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
)

func TestSnapshotCompatibilityIsComputedOnEveryRead(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	for _, id := range []string{"current", "legacy", "old", "malformed", "incomplete"} {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions()}
		switch id {
		case "legacy":
			snap.ToolVersions = nil
		case "old":
			snap.ToolVersions["relationshipVersion"] = "0"
		case "malformed":
			snap.ToolVersions["syntaxCacheVersion"] = "invalid"
		case "incomplete":
			snap.IngestionStatus = "staging"
		}
		if err := st.Publish(ctx, "/repo", snap, graph.NewGraph("repo", id)); err != nil {
			t.Fatal(err)
		}
		loaded, err := st.Snapshot(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		assertSnapshotCompatibility(t, loaded)
	}
	for _, list := range []func(context.Context, string) ([]*pb.Snapshot, error){st.Snapshots, st.SnapshotRefs} {
		snaps, err := list(ctx, "repo")
		if err != nil || len(snaps) != 5 {
			t.Fatalf("snapshots=%d err=%v", len(snaps), err)
		}
		for _, snap := range snaps {
			assertSnapshotCompatibility(t, snap)
		}
	}
}

func assertSnapshotCompatibility(t *testing.T, snap *pb.Snapshot) {
	t.Helper()
	want := snap.Id == "current"
	if snap.Compatible != want || (snap.IncompatibilityReason == "") != want {
		t.Fatalf("snapshot %s: compatible=%v reason=%q", snap.Id, snap.Compatible, snap.IncompatibilityReason)
	}
	if want && indexer.CheckSnapshot(snap) != nil {
		t.Fatal("version stamps did not survive persistence")
	}
}
