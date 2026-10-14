package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"
	"sync"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
)

func TestIntegerIDsAllocateAtomicallyAndUse64Bits(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	const callers = 12
	ids := make(chan int64, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			id, err := st.AllocateIDs(ctx, 3)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		})
	}
	wg.Wait()
	close(ids)
	seen := map[int64]bool{}
	for first := range ids {
		for id := first; id < first+3; id++ {
			if seen[id] || id < 1 {
				t.Fatalf("duplicate or invalid ID: %d", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != callers*3 {
		t.Fatalf("allocated %d IDs", len(seen))
	}
	if _, err := st.AllocateIDs(ctx, 0); err == nil {
		t.Fatal("zero count accepted")
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE codeindex_id_sequence SET value=2147483647`); err != nil {
		t.Fatal(err)
	}
	if id, err := st.AllocateIDs(ctx, 1); err != nil || id != 2147483648 {
		t.Fatalf("64-bit allocation: %d %v", id, err)
	}
	if _, err := st.bun.NewRaw(`UPDATE codeindex_id_sequence SET value=?`, int64(math.MaxInt64)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AllocateIDs(ctx, 1); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestIntegerIDPublicationRemapsReferencesAndPreservesPayloads(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	root := t.TempDir()
	repository, err := st.ResolveRepository(ctx, root, "", false, scope(ctx).orgID)
	if err != nil {
		t.Fatal(err)
	}
	snap := &pb.Snapshot{Id: graph.ID("pending"), RepositoryId: repository.ID}
	g := graph.NewGraph(snap.RepositoryId, snap.Id)
	src := &graph.Source{Path: "a.go", Text: []byte("func A(){}\nfunc B(){}\n"), Hash: "source-hash"}
	g.Sources[src.Path] = src
	parent := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, 10), "A", "", nil)
	child := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", src.Anchor(11, 21), "B", "", nil)
	child.ParentFactId = parent.Id
	edge := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, child.Id, parent.Id, "symbol/A", src.Anchor(12, 13))
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{repository.ID, snap.Id, parent.Id, child.Id, edge.Id} {
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			t.Fatalf("ID %q: %v", id, err)
		}
	}
	loaded, err := st.LoadGraph(ctx, snap.Id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Facts[child.Id].ParentFactId != parent.Id || loaded.EdgeFacts[edge.Id].FromFactId != child.Id || loaded.EdgeFacts[edge.Id].ToFactId != parent.Id {
		t.Fatal("references were not remapped")
	}
	got := loaded.EdgeFacts[edge.Id]
	if got.Anchor != nil || len(got.Evidence) != 0 {
		t.Fatalf("edge retained provenance: %v", got)
	}
	first := snap.Id
	if err := st.Publish(ctx, root, snap, g); err != nil || snap.Id != first {
		t.Fatalf("retry changed snapshot: %s %v", snap.Id, err)
	}
	if _, err := st.Snapshot(ctx, graph.ID("old snapshot")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("legacy snapshot lookup=%v", err)
	}
}
