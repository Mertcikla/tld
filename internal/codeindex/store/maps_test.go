package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func TestCompletedMapsSurviveNewMappingsAndDeleteWithRepository(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	for _, id := range []string{"base", "head"} {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", Provenance: "commit", ContentFingerprint: "fp-" + id}
		if err := idx.Publish(ctx, "/repo", snap, graph.NewGraph("repo", id)); err != nil {
			t.Fatal(err)
		}
		if err := idx.SaveCompletedMap(ctx, "repo", &pb.CompletedMap{Result: &pb.MapResult{RunId: "run-" + id, SnapshotId: id, ViewId: 9, Facts: 3}, Profile: "p1", ConfigHash: "cfg", CompletedUnix: 10}); err != nil {
			t.Fatal(err)
		}
	}
	maps, err := idx.CompletedMaps(ctx, "repo")
	if err != nil || len(maps) != 2 {
		t.Fatalf("maps: %+v: %v", maps, err)
	}
	snap, err := idx.Snapshot(ctx, "base")
	if err != nil || snap.Provenance != "commit" || snap.ContentFingerprint != "fp-base" {
		t.Fatalf("provenance roundtrip: %+v: %v", snap, err)
	}
	if err := idx.DeleteRepository(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	maps, err = idx.CompletedMaps(ctx, "repo")
	if err != nil || len(maps) != 0 {
		t.Fatalf("records remain: %v", err)
	}
}

func TestSnapshotCaptureOrderSurvivesTimestampTiesAndRetries(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	for _, id := range []string{"z-first", "a-second", "z-first"} {
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", CreatedUnix: 100}
		if err := idx.PublishHistorical(ctx, "/repo", snap, graph.NewGraph("repo", id)); err != nil {
			t.Fatal(err)
		}
	}
	snapshots, err := idx.Snapshots(ctx, "repo")
	if err != nil || len(snapshots) != 2 || snapshots[0].Id != "z-first" || snapshots[1].Id != "a-second" {
		t.Fatalf("snapshot capture order: %+v: %v", snapshots, err)
	}
}
