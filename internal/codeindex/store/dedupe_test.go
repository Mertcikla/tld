package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"google.golang.org/protobuf/proto"
)

func TestExactRowReuse(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testExactRowReuse(t, st)
}

func testExactRowReuse(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	var stable strings.Builder
	stable.WriteString("package sample\n")
	for i := range 100 {
		fmt.Fprintf(&stable, "func Stable%d() { Stable%d() }\n", i, (i+1)%100)
	}
	if err := os.WriteFile(filepath.Join(root, "stable.go"), []byte(stable.String()), 0600); err != nil {
		t.Fatal(err)
	}
	pipeline := indexer.Pipeline{Config: config.Default()}
	build := func(prefix, body string) (*pb.Snapshot, *graph.Graph) {
		t.Helper()
		text := prefix + "package sample\nfunc Changed() { " + body + " }\n"
		if err := os.WriteFile(filepath.Join(root, "changed.go"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		// Cold extraction deliberately supplies no prior graph or cache IDs.
		snap, g, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: root}, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Supply a resolved call without requiring an external SCIP executable.
		var from, to *pb.CodeFact
		target := strings.TrimSuffix(body, "()")
		for _, f := range g.Facts {
			if f.Name == "Changed" {
				from = f
			}
			if f.Name == target {
				to = f
			}
		}
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, from.Id, to.Id, "", from.Anchor)
		if err := st.Publish(ctx, root, snap, g); err != nil {
			t.Fatal(err)
		}
		return snap, g
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := st.bun.NewRaw(`SELECT COUNT(*) FROM `+table).Scan(ctx, &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	first, g1 := build("", "Stable0()")
	facts, edges := count("codeindex_facts"), count("codeindex_edges")
	second, g2 := build("", "Stable1()")
	changedFacts := 0
	for _, f := range g2.Facts {
		if f.GetAnchor().GetPath() == "changed.go" {
			changedFacts++
		} else if g1.Facts[f.Id] == nil {
			t.Fatalf("unchanged fact %s was copied", f.Name)
		}
	}
	if added := count("codeindex_facts") - facts; added != changedFacts {
		t.Fatalf("inserted %d facts, want %d changed-file facts", added, changedFacts)
	}
	if added := count("codeindex_edges") - edges; added != 1 {
		t.Fatalf("inserted %d edges, want only the changed call", added)
	}
	beforeFacts, beforeEdges := count("codeindex_facts"), count("codeindex_edges")
	third, g3 := build("", "Stable1()")
	if count("codeindex_facts") != beforeFacts || count("codeindex_edges") != beforeEdges {
		t.Fatal("identical cold extraction inserted entities")
	}
	// A position-only change must create new fact rows and retain exact anchors.
	fourth, g4 := build("\n", "Stable1()")
	if count("codeindex_facts") != beforeFacts+changedFacts {
		t.Fatal("different source positions were merged")
	}
	for _, pair := range []struct {
		snap *pb.Snapshot
		g    *graph.Graph
	}{{first, g1}, {second, g2}, {third, g3}, {fourth, g4}} {
		loaded, err := st.LoadGraph(ctx, pair.snap.Id)
		if err != nil {
			t.Fatal(err)
		}
		for id, want := range pair.g.Facts {
			if !proto.Equal(loaded.Facts[id], want) {
				t.Fatalf("snapshot %s did not preserve fact %s", pair.snap.Id, want.Name)
			}
		}
	}
	if err := st.DeleteSnapshot(ctx, first.Id); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadGraph(ctx, third.Id)
	if err != nil || len(loaded.Facts) != len(g3.Facts) || len(loaded.EdgeFacts) != len(g3.EdgeFacts) {
		t.Fatalf("deleting an origin lost shared entities: %v", err)
	}
	if err := st.DeleteRepository(ctx, third.RepositoryId); err != nil {
		t.Fatal(err)
	}
	if count("codeindex_facts")+count("codeindex_edges") != 0 {
		t.Fatal("repository deletion retained shared entities")
	}
}

func TestExactEdgeReusePreservesRepeatedObservations(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testExactEdgeReusePreservesRepeatedObservations(t, st)
}

func testExactEdgeReusePreservesRepeatedObservations(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	for i, observations := range []int{2, 3, 2} {
		snapshot := fmt.Sprint(100 + i*100)
		g := graph.NewGraph("10", snapshot)
		for j := range observations {
			id := fmt.Sprint(101 + i*100 + j)
			g.EdgeFacts[id] = &pb.EdgeFact{Id: id, RepositoryId: "10", SnapshotId: snapshot,
				Kind: pb.EdgeKind_EDGE_KIND_REFERENCES, TargetSymbolKey: "external", Weight: 1,
				LogicalKey: graph.EdgeLogicalKey(pb.EdgeKind_EDGE_KIND_REFERENCES, "from", "external")}
		}
		if err := st.Publish(ctx, "/duplicates", &pb.Snapshot{Id: snapshot, RepositoryId: "10"}, g); err != nil {
			t.Fatal(err)
		}
		edges, err := st.EdgeFacts(ctx, snapshot, 0, "", "", 100)
		if err != nil || len(edges) != observations {
			t.Fatalf("observation count = %d, want %d: %v", len(edges), observations, err)
		}
	}
	var physical int
	if err := st.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_edges`).Scan(ctx, &physical); err != nil || physical != 3 {
		t.Fatalf("physical observations = %d, want 3: %v", physical, err)
	}
	diff, err := st.Diff(ctx, "100", "200", false)
	if err != nil || len(diff.GetEdgeFacts().GetModified()) != 1 {
		t.Fatalf("repeated observations lost their delta: %v %v", diff, err)
	}
}

func TestRowReuseUpgradePreservesSnapshots(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testRowReuseUpgradePreservesSnapshots(t, st, "migrations")
}

func testRowReuseUpgradePreservesSnapshots(t *testing.T, st *Store, migrations string) {
	t.Helper()
	ctx := context.Background()
	g := graph.NewGraph("10", "100")
	g.Facts["101"] = &pb.CodeFact{Id: "101", RepositoryId: "10", SnapshotId: "100", Name: "legacy", LogicalKey: "legacy"}
	if err := st.Publish(ctx, "/legacy", &pb.Snapshot{Id: "100", RepositoryId: "10"}, g); err != nil {
		t.Fatal(err)
	}
	// Recreate the immediately preceding schema with a real published snapshot.
	for _, table := range []string{"facts", "edges"} {
		for _, query := range []string{
			`DROP INDEX idx_codeindex_` + table + `_content`,
			`ALTER TABLE codeindex_` + table + ` DROP COLUMN content_hash`,
		} {
			if _, err := st.bun.ExecContext(ctx, query); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := st.bun.ExecContext(ctx, `ALTER TABLE codeindex_snapshots DROP COLUMN symbol_versions_json`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.bun.ExecContext(ctx, `DELETE FROM bun_migrations WHERE name IN ('20261014000500','20261015000100')`); err != nil {
		t.Fatal(err)
	}
	if err := dbrepo.ApplyEmbeddedMigrations(ctx, st.bun, assets.FS, migrations); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadGraph(ctx, "100")
	if err != nil || !proto.Equal(loaded.Facts["101"], g.Facts["101"]) {
		t.Fatalf("upgrade changed the published fact: %v", err)
	}
	// An old row has no fingerprint. Retrying its snapshot still retains it.
	if err := st.Publish(ctx, "/legacy", &pb.Snapshot{Id: "100", RepositoryId: "10"}, loaded); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSnapshot(ctx, "100"); err != nil {
		t.Fatal(err)
	}
}
