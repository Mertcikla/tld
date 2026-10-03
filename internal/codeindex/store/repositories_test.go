package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"google.golang.org/protobuf/proto"
)

func TestDeleteRepositoryRemovesAllRepositoryRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	pipeline := indexer.Pipeline{Config: config.Default()}
	snap, g, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: dir}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	if err := st.Publish(ctx, dir, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}
	var factID string
	for id := range g.Facts {
		factID = id
		break
	}
	if factID == "" {
		t.Fatal("indexer produced no facts")
	}
	if err := st.SaveMappings(ctx, []ResourceMapping{
		{LogicalKey: "map|view|" + snap.RepositoryId, Kind: MappingView, ResourceID: 11, RepositoryID: snap.RepositoryId, SnapshotID: snap.Id},
		{LogicalKey: "map|element|" + snap.RepositoryId, Kind: MappingElement, ResourceID: 12, RepositoryID: snap.RepositoryId, SnapshotID: snap.Id},
	}); err != nil {
		t.Fatalf("save mappings: %v", err)
	}
	if err := st.SaveAnalysis(ctx, AnalysisRun{
		ID:           "run-1",
		RepositoryID: snap.RepositoryId,
		SnapshotID:   snap.Id,
		Algorithm:    "mapper",
		Groups:       []AnalysisGroup{{ID: "run-1:0", Label: "core", Members: []string{factID}}},
	}); err != nil {
		t.Fatalf("save analysis: %v", err)
	}

	if err := st.DeleteRepository(ctx, snap.RepositoryId); err != nil {
		t.Fatalf("delete repository: %v", err)
	}

	repos, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("repositories remain: %+v", repos)
	}
	snapshots, err := st.Snapshots(ctx, snap.RepositoryId)
	if err != nil {
		t.Fatalf("snapshots: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("snapshots remain: %d", len(snapshots))
	}
	facts, err := st.Facts(ctx, snap.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 100)
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("facts remain: %d", len(facts))
	}
	mappings, err := st.MappingsByRepository(ctx, snap.RepositoryId)
	if err != nil {
		t.Fatalf("mappings: %v", err)
	}
	if len(mappings) != 0 {
		t.Fatalf("mappings remain: %d", len(mappings))
	}
	for table, want := range map[string]int{
		"codeindex_analysis_runs": 0,
		"codeindex_groups":        0,
		"codeindex_group_members": 0,
	} {
		var count int
		if err := st.bun.NewRaw("SELECT COUNT(*) FROM "+table).Scan(ctx, &count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s rows = %d, want %d", table, count, want)
		}
	}
	if _, err := st.Repository(ctx, snap.RepositoryId); err == nil {
		t.Fatal("repository still resolves after delete")
	}
}

func TestListingsReportMembershipStatsWithoutSourceManifests(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	pipeline := indexer.Pipeline{Config: config.Default()}
	snap, g, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: dir}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	if err := st.Publish(ctx, dir, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	snapshots, err := st.Snapshots(ctx, snap.RepositoryId)
	if err != nil {
		t.Fatalf("snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	listed := snapshots[0]
	if len(listed.Sources) != 0 {
		t.Fatalf("listing returned %d source manifests, want 0", len(listed.Sources))
	}
	if listed.Statistics == nil {
		t.Fatal("listing omitted statistics")
	}
	if got, want := listed.Statistics.Facts, uint32(len(g.Facts)); got != want {
		t.Fatalf("statistics facts = %d, want %d", got, want)
	}
	if got, want := listed.Statistics.Edges, uint32(len(g.EdgeFacts)); got != want {
		t.Fatalf("statistics edges = %d, want %d", got, want)
	}
	if got, want := listed.Statistics.Chunks, uint32(len(g.Chunks)); got != want {
		t.Fatalf("statistics chunks = %d, want %d", got, want)
	}
	if got, want := listed.Statistics.Sources, uint32(len(snap.Sources)); got != want {
		t.Fatalf("statistics sources = %d, want %d", got, want)
	}

	repos, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("repositories = %d, want 1", len(repos))
	}
	summary := repos[0]
	if summary.Facts != uint32(len(g.Facts)) || summary.Edges != uint32(len(g.EdgeFacts)) || summary.Sources != uint32(len(snap.Sources)) {
		t.Fatalf("summary counts = %d facts/%d edges/%d sources", summary.Facts, summary.Edges, summary.Sources)
	}
}

func TestDeleteSnapshotRetainsSharedFactsAndRepointsLatest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	pipeline := indexer.Pipeline{Config: config.Default()}
	snapA, g, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: dir}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	if err := st.Publish(ctx, dir, snapA, g); err != nil {
		t.Fatalf("publish a: %v", err)
	}
	snapB := proto.Clone(snapA).(*pb.Snapshot)
	snapB.Id = snapA.Id + "-b"
	snapB.CreatedUnix = snapA.CreatedUnix + 1
	if err := st.Publish(ctx, dir, snapB, g); err != nil {
		t.Fatalf("publish b: %v", err)
	}

	if got, err := st.Snapshots(ctx, snapA.RepositoryId); err != nil || len(got) != 2 {
		t.Fatalf("snapshots before delete = %d (%v), want 2", len(got), err)
	}

	if err := st.DeleteSnapshot(ctx, snapB.Id); err != nil {
		t.Fatalf("delete snapshot b: %v", err)
	}

	remaining, err := st.Snapshots(ctx, snapA.RepositoryId)
	if err != nil {
		t.Fatalf("snapshots: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Id != snapA.Id {
		t.Fatalf("remaining snapshots = %+v, want [%s]", remaining, snapA.Id)
	}
	facts, err := st.Facts(ctx, snapA.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 100)
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("shared facts were garbage collected while snapshot a still referenced them")
	}
	repo, err := st.Repository(ctx, snapA.RepositoryId)
	if err != nil {
		t.Fatalf("repository: %v", err)
	}
	if repo.LatestSnapshotId != snapA.Id {
		t.Fatalf("latest snapshot = %q, want %q", repo.LatestSnapshotId, snapA.Id)
	}

	if err := st.DeleteSnapshot(ctx, snapA.Id); err != nil {
		t.Fatalf("delete snapshot a: %v", err)
	}
	if got, err := st.Snapshots(ctx, snapA.RepositoryId); err != nil || len(got) != 0 {
		t.Fatalf("snapshots after final delete = %d (%v), want 0", len(got), err)
	}
	if facts, err := st.Facts(ctx, snapA.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 100); err != nil || len(facts) != 0 {
		t.Fatalf("facts after final delete = %d (%v), want 0", len(facts), err)
	}
	repo, err = st.Repository(ctx, snapA.RepositoryId)
	if err != nil {
		t.Fatalf("repository after final delete: %v", err)
	}
	if repo.LatestSnapshotId != "" {
		t.Fatalf("latest snapshot after final delete = %q, want empty", repo.LatestSnapshotId)
	}
}

func TestDeleteSnapshotRejectsUnknownAndEmptyID(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	if err := st.DeleteSnapshot(context.Background(), "  "); err == nil {
		t.Fatal("expected an error for an empty snapshot id")
	}
	if err := st.DeleteSnapshot(context.Background(), "missing"); err == nil {
		t.Fatal("expected an error for an unknown snapshot id")
	}
}

func TestDeleteRepositoryRejectsEmptyID(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	if err := st.DeleteRepository(context.Background(), "  "); err == nil {
		t.Fatal("expected an error for an empty repository id")
	}
}
