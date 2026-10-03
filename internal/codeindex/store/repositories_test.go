package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
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
		if err := st.bun.NewRaw("SELECT COUNT(*) FROM " + table).Scan(ctx, &count); err != nil {
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

func TestDeleteRepositoryRejectsEmptyID(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	if err := st.DeleteRepository(context.Background(), "  "); err == nil {
		t.Fatal("expected an error for an empty repository id")
	}
}
