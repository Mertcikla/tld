package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

func TestSnapshotLineCountsBatchMultipleFiles(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	publish := func(id string, files map[string]string) *pb.Snapshot {
		snap := &pb.Snapshot{Id: id, RepositoryId: "batch"}
		g := graph.NewGraph(snap.RepositoryId, snap.Id)
		for path, text := range files {
			src := &graph.Source{Path: path, Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[src.Path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: src.Path, Hash: src.Hash})
		}
		if err := st.Publish(ctx, "/batch", snap, g); err != nil {
			t.Fatal(err)
		}
		return snap
	}
	before := publish("batch-base", map[string]string{
		"a/file one.go":     "one\ntwo\n",
		"z.go":              "alpha\nbeta\ngamma\n",
		"replace":           "old\n",
		"collapse/child.go": "old\nchild\n",
	})
	after := publish("batch-head", map[string]string{
		"a/file one.go":          "one\ntwo\nthree\n",
		"z.go":                   "alpha\n",
		"new.txt":                "x\ny\n",
		"replace/child {one}.go": "new\nchild\n",
		"collapse":               "new\n",
	})
	diff, err := st.Diff(ctx, before.Id, after.Id, true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]uint32{
		"a/file one.go":          {1, 0},
		"new.txt":                {2, 0},
		"z.go":                   {0, 2},
		"replace":                {0, 1},
		"replace/child {one}.go": {2, 0},
		"collapse/child.go":      {0, 2},
		"collapse":               {1, 0},
	}
	if len(diff.Sources) != len(want) {
		t.Fatalf("sources = %d, want %d", len(diff.Sources), len(want))
	}
	for _, change := range diff.Sources {
		expected, ok := want[change.Path]
		if !ok {
			t.Fatalf("unexpected changed path %q", change.Path)
		}
		if change.GetLinesAdded() != expected[0] || change.GetLinesRemoved() != expected[1] {
			t.Fatalf("%s: +%d -%d, want +%d -%d", change.Path, change.GetLinesAdded(), change.GetLinesRemoved(), expected[0], expected[1])
		}
	}
}

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
			// Unknown captured counts remain unknown on immutable cache reads.
			diff.Sources[0].LinesAdded, diff.Sources[0].LinesRemoved = nil, nil
			if err := st.SaveImpact(ctx, &model.Diagram{RepositoryId: test.name, ComparisonKey: "live", Diff: diff}); err != nil {
				t.Fatal(err)
			}
			saved, err := st.Impact(ctx, test.name, "live")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Diff.Sources[0].LinesAdded != nil || saved.Diff.Sources[0].LinesRemoved != nil {
				t.Fatal("unknown captured impact counts were reconstructed")
			}
		})
	}
}
