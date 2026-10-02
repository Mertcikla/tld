package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func TestFactEmbeddingsAndAnalysisPersistence(t *testing.T) {
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

	embeddings := []*pb.Embedding{
		{Id: "e1", FactId: "f1", SnapshotId: snap.Id, Profile: "p1", Dimensions: 3, Vector: []float32{1, 0, 0}},
		{Id: "e2", FactId: "f2", SnapshotId: snap.Id, Profile: "p1", Dimensions: 3, Vector: []float32{0, 1, 0}},
		{Id: "e3", FactId: "f3", SnapshotId: snap.Id, Profile: "p2", Dimensions: 2, Vector: []float32{1, 1}},
	}
	for _, embedding := range embeddings {
		if err := st.SaveFactEmbedding(ctx, embedding); err != nil {
			t.Fatalf("save fact embedding %s: %v", embedding.Id, err)
		}
	}

	majority, err := st.MajorityProfile(ctx, snap.Id)
	if err != nil {
		t.Fatalf("majority profile: %v", err)
	}
	if majority != "p1" {
		t.Fatalf("majority profile = %q, want p1", majority)
	}

	rows, err := st.FactEmbeddings(ctx, snap.Id, "p1", pb.FactKind_FACT_KIND_FILE)
	if err != nil {
		t.Fatalf("fact embeddings: %v", err)
	}
	if len(rows) != 2 || rows[0].Fact.Id != "f1" || rows[1].Fact.Id != "f2" {
		t.Fatalf("fact embeddings = %+v", rows)
	}
	if got := rows[0].Fact.Name; got != "A" {
		t.Fatalf("f1 name = %q", got)
	}

	empty, err := st.FactEmbeddings(ctx, snap.Id, "missing", pb.FactKind_FACT_KIND_FILE)
	if err != nil {
		t.Fatalf("missing profile: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("missing profile rows = %d", len(empty))
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
