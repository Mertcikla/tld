package store

import (
	"context"
	"fmt"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

func TestPackageVersionReuse(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testPackageVersionReuse(t, st)
}

func testPackageVersionReuse(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	var snapshots []*pb.Snapshot
	var graphs []*graph.Graph
	for _, version := range []string{"validation-base", "validation-edit-1", "revision-sha"} {
		snap := &pb.Snapshot{Id: graph.ID(version), RepositoryId: "10"}
		g := graph.NewGraph(snap.RepositoryId, snap.Id)
		source := &graph.Source{Path: "a.go", Language: "go", Text: []byte("func A(){}\nfunc B(){}\n"), Hash: "source-hash"}
		g.Sources[source.Path] = source
		snap.Sources = []*pb.SourceFile{{Path: source.Path, Hash: source.Hash}}
		var parent *pb.CodeFact
		for i, name := range []string{"A", "B"} {
			symbol := fmt.Sprintf("scip-go gomod fixture %s pkg/%s().", version, name)
			f := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, name, "go", source.Anchor(i*11, i*11+10), name, "",
				&pb.Evidence{Producer: "scip", OriginalId: symbol, Version: "indexer-1"})
			f.SymbolKey, f.QualifiedName = symbol, symbol
			if parent == nil {
				parent = f
			} else {
				f.ParentFactId = parent.Id
				g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, f.Id, parent.Id, "", f.Anchor)
			}
		}
		if err := st.Publish(ctx, "/versions", snap, g); err != nil {
			t.Fatal(err)
		}
		snapshots, graphs = append(snapshots, snap), append(graphs, g)
	}
	var facts, edges int
	if err := st.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_facts`).Scan(ctx, &facts); err != nil {
		t.Fatal(err)
	}
	if err := st.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_edges`).Scan(ctx, &edges); err != nil {
		t.Fatal(err)
	}
	if facts != 2 || edges != 1 {
		t.Fatalf("package versions copied facts/edges: %d/%d", facts, edges)
	}
	check := func(i int) {
		t.Helper()
		loaded, err := st.LoadGraph(ctx, snapshots[i].Id)
		if err != nil {
			t.Fatal(err)
		}
		for id, want := range graphs[i].Facts {
			if !proto.Equal(loaded.Facts[id], want) {
				t.Fatalf("snapshot %s lost its original metadata: %v / %v", snapshots[i].Id, loaded.Facts[id], want)
			}
		}
	}
	for i := range snapshots {
		check(i)
	}
	if err := st.DeleteSnapshot(ctx, snapshots[0].Id); err != nil {
		t.Fatal(err)
	}
	check(1)
	check(2)
}
