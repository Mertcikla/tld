package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func TestAnalysisPersistence(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	root := "/repo"
	repoID := graph.RepositoryID(root)
	snap := &pb.Snapshot{Id: "snap-1", RepositoryId: repoID, CreatedUnix: 100}
	g := graph.NewGraph(repoID, snap.Id)
	g.Facts["f1"] = &pb.CodeFact{Id: "f1", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Name: "A", Language: "go", Anchor: &pb.SourceAnchor{Path: "src/a.go"}}
	g.Facts["f2"] = &pb.CodeFact{Id: "f2", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, QualifiedName: "B", Language: "go", Anchor: &pb.SourceAnchor{Path: "src/b.go"}}
	g.Facts["f3"] = &pb.CodeFact{Id: "f3", RepositoryId: repoID, SnapshotId: snap.Id, Kind: pb.FactKind_FACT_KIND_FILE, Language: "go", Anchor: &pb.SourceAnchor{Path: "src/deep/c.go"}}
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	run := AnalysisRun{
		ID:           "run-1",
		RepositoryID: repoID,
		SnapshotID:   snap.Id,
		Algorithm:    "mapper",
		Params:       map[string]string{"neighbors": "10"},
		Groups: []AnalysisGroup{
			{ID: "g1", Label: "src", Kind: pb.GroupKind_GROUP_KIND_CLUSTER, Profile: "p1", Members: []string{"f1", "f2"}},
			{ID: "g2", Label: "src/deep", Kind: pb.GroupKind_GROUP_KIND_CLUSTER, Profile: "p2", Members: []string{"f3"}},
		},
	}
	if err := st.SaveAnalysis(ctx, run); err != nil {
		t.Fatalf("save analysis: %v", err)
	}
	if err := st.SaveAnalysis(ctx, run); err != nil {
		t.Fatalf("re-save analysis: %v", err)
	}

	var runs, groups, members int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_analysis_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_groups`).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM codeindex_group_members`).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || groups != 2 || members != 3 {
		t.Fatalf("runs=%d groups=%d members=%d", runs, groups, members)
	}
}
