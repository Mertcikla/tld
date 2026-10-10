package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
)

func TestSnapshotLineCounts(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	for _, test := range []struct {
		name, before, after string
		added, removed      uint32
	}{
		{"added", "", "first\nsecond", 2, 0},
		{"removed", "first\nsecond\n", "", 0, 2},
		{"modified", "keep\nold\nlast\n", "keep\nnew\nextra\nlast\n", 2, 1},
		{"newline", "same", "same\n", 1, 1},
		{"empty", "", "", 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			publish := func(suffix, text string) *pb.Snapshot {
				snap := &pb.Snapshot{Id: test.name + suffix, RepositoryId: test.name}
				g := graph.NewGraph(snap.RepositoryId, snap.Id)
				if text != "" {
					src := &graph.Source{Path: "file.go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
					g.Sources[src.Path] = src
					snap.Sources = []*pb.SourceFile{{Path: src.Path, Hash: src.Hash}}
				}
				if err := st.Publish(ctx, "/"+test.name, snap, g); err != nil {
					t.Fatal(err)
				}
				return snap
			}
			before, after := publish("base", test.before), publish("head", test.after)
			diff, err := st.Diff(ctx, before.Id, after.Id, true)
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "empty" {
				if len(diff.Sources) != 0 {
					t.Fatal("identical snapshots report changed sources")
				}
				return
			}
			if len(diff.Sources) != 1 || diff.Sources[0].GetLinesAdded() != test.added || diff.Sources[0].GetLinesRemoved() != test.removed {
				t.Fatalf("line counts: %+v, want +%d -%d", diff.Sources, test.added, test.removed)
			}
			// Persisted diagrams from before the fields existed are hydrated too.
			diff.Sources[0].LinesAdded, diff.Sources[0].LinesRemoved = nil, nil
			if err := st.SaveImpact(ctx, &pb.ImpactDiagram{RepositoryId: test.name, ComparisonKey: "live", Diff: diff}); err != nil {
				t.Fatal(err)
			}
			saved, err := st.Impact(ctx, test.name, "live")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Diff.Sources[0].GetLinesAdded() != test.added || saved.Diff.Sources[0].GetLinesRemoved() != test.removed {
				t.Fatal("legacy impact line counts were not hydrated")
			}
		})
	}
}
