package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// TestSnapshotMembershipSharesReusedFacts verifies that an unchanged fact is
// published once and resolved through membership in every snapshot that
// contains it.
func TestSnapshotMembershipSharesReusedFacts(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	repo := "repo"

	// Snapshot one: two declarations.
	g1 := graph.NewGraph(repo, "snap-1")
	src := &graph.Source{Path: "a.go", Language: "go", Text: []byte("package a\nfunc A(){}\nfunc B(){}\n")}
	g1.Sources[src.Path] = src
	a1 := g1.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(9, 20), "func A(){}", "func A()", nil)
	_ = a1
	bFact := g1.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", src.Anchor(21, 32), "func B(){}", "func B()", nil)
	if err := idx.Publish(ctx, "/repo", snapFor(repo, "snap-1", src), g1); err != nil {
		t.Fatal(err)
	}

	// Snapshot two: A changes (new fact id), B is reused with its original id.
	g2 := graph.NewGraph(repo, "snap-2")
	g2.Sources[src.Path] = src
	a2 := g2.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(9, 22), "func A(){x}", "func A()", nil)
	adopted := g2.AdoptFactAnchored(bFact, bFact.Anchor, bFact.BodyHash, "func B()")
	if adopted.Id != bFact.Id {
		t.Fatal("reused fact did not keep its id")
	}
	if err := idx.Publish(ctx, "/repo", snapFor(repo, "snap-2", src), g2); err != nil {
		t.Fatal(err)
	}

	// Both snapshots resolve A and B, including the shared B row.
	facts1, err := idx.Facts(ctx, "snap-1", pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	facts2, err := idx.Facts(ctx, "snap-2", pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts1) != 2 || len(facts2) != 2 {
		t.Fatalf("membership facts: %d / %d", len(facts1), len(facts2))
	}
	byName := func(facts []*pb.CodeFact) map[string]*pb.CodeFact {
		out := map[string]*pb.CodeFact{}
		for _, f := range facts {
			out[f.Name] = f
		}
		return out
	}
	if byName(facts2)["B"].Id != bFact.Id || byName(facts1)["B"].Id != bFact.Id {
		t.Fatal("B identity changed across snapshots")
	}
	if byName(facts1)["A"].Id == byName(facts2)["A"].Id {
		t.Fatal("changed A kept its identity")
	}
	// The shared B row exists exactly once.
	var count int
	if err := idx.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_facts WHERE id = ?`, bFact.Id).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reused fact rows = %d, want 1", count)
	}
	// Snapshot stats use membership.
	snap2, err := idx.Snapshot(ctx, "snap-2")
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Statistics.Facts != 2 {
		t.Fatalf("snapshot statistics facts = %d", snap2.Statistics.Facts)
	}
	// The old incremental base round-trips through membership.
	loaded, err := idx.LoadGraph(ctx, "snap-2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Facts[bFact.Id] == nil || loaded.Facts[a2.Id] == nil {
		t.Fatal("LoadGraph did not resolve membership")
	}
}

func snapFor(repo, id string, src *graph.Source) *pb.Snapshot {
	return &pb.Snapshot{
		Id: id, RepositoryId: repo, IngestionStatus: "complete",
		Sources: []*pb.SourceFile{{Path: src.Path, Hash: src.Hash, Size: uint64(len(src.Text))}},
	}
}
