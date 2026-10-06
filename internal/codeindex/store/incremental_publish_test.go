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

func TestIncrementalPublicationRetainsCurrentAnchors(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	put := func(name, code string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("a.go", "package a\nfunc Changed() int { return 1 }\nfunc Stable() {}\n")
	put("b.go", "package a\nfunc Other() {}\n")
	pipeline := indexer.Pipeline{Config: config.Default()}
	req := &pb.IndexRequest{Directory: root}
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	snap, built, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Publish(ctx, root, snap, built); err != nil {
		t.Fatal(err)
	}
	originalID := snap.Id
	original, err := idx.LoadGraph(ctx, originalID)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []struct{ name, code string }{
		{"b.go", "package a\nfunc Other() { println(1) }\n"},
		{"a.go", "package a\n\nfunc Changed() int { return 200 }\n\nfunc Stable() {}\n"},
	} {
		put(edit.name, edit.code)
		baseGraph, err := idx.LoadGraph(ctx, snap.Id)
		if err != nil {
			t.Fatal(err)
		}
		hashes := map[string]string{}
		for _, source := range snap.Sources {
			hashes[source.Path] = source.Hash
		}
		next, nextGraph, reused, err := pipeline.BuildIncremental(ctx, req, nil, &indexer.IncrementalBase{Snapshot: snap, Graph: baseGraph, Sources: hashes})
		if err != nil || reused {
			t.Fatalf("build incremental: %v reused=%v", err, reused)
		}
		for range 2 {
			if err := idx.Publish(ctx, root, next, nextGraph); err != nil {
				t.Fatal(err)
			}
			stored, err := idx.LoadGraph(ctx, next.Id)
			if err != nil {
				t.Fatal(err)
			}
			if len(stored.Facts) != len(nextGraph.Facts) {
				t.Fatalf("published counts: facts %d/%d", len(stored.Facts), len(nextGraph.Facts))
			}
			for id, want := range nextGraph.Facts {
				got := stored.Facts[id]
				if !proto.Equal(got.Anchor, want.Anchor) || got.ParentFactId != want.ParentFactId {
					t.Fatalf("stale fact metadata: %s", want.Name)
				}
				if got.Anchor.SourceHash != stored.Sources[got.Anchor.Path].Hash {
					t.Fatalf("stale source hash: %s", want.Name)
				}
				for i, evidence := range want.Evidence {
					if !proto.Equal(got.Evidence[i], evidence) {
						t.Fatalf("stale evidence: %s", want.Name)
					}
				}
			}
		}
		snap = next
	}
	old, err := idx.LoadGraph(ctx, originalID)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range original.Facts {
		if !proto.Equal(old.Facts[id], want) {
			t.Fatalf("old snapshot changed: %s", want.Name)
		}
	}
}
