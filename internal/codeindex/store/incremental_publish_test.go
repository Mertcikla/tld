package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/indexer"
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

func TestIncrementalPublicationIdenticalSourcesKeepOwnIdentity(t *testing.T) {
	ctx := context.Background()
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	type checkout struct {
		root     string
		snap     *pb.Snapshot
		pipeline indexer.Pipeline
	}
	var checkouts []checkout
	for _, repositoryID := range []string{"first", "second"} {
		root := t.TempDir()
		for name, text := range map[string]string{
			"one/a.go": "package same\nfunc Stable() {}\n",
			"two/a.go": "package same\nfunc Stable() {}\n",
			"other.go": "package other\nfunc Changed() {}\n",
		} {
			file := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		pipeline := indexer.Pipeline{Config: config.Default(), RepositoryID: repositoryID}
		snap, built, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: root}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := idx.Publish(ctx, root, snap, built); err != nil {
			t.Fatal(err)
		}
		checkouts = append(checkouts, checkout{root: root, snap: snap, pipeline: pipeline})
	}
	for _, checkout := range checkouts {
		t.Run(checkout.snap.RepositoryId, func(t *testing.T) {
			base, err := idx.LoadGraph(ctx, checkout.snap.Id)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(checkout.root, "other.go"), []byte("package other\nfunc Changed() { println(1) }\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			hashes := map[string]string{}
			for _, source := range checkout.snap.Sources {
				hashes[source.Path] = source.Hash
			}
			next, built, reused, err := checkout.pipeline.BuildIncremental(ctx, &pb.IndexRequest{Directory: checkout.root}, nil,
				&indexer.IncrementalBase{Snapshot: checkout.snap, Graph: base, Sources: hashes})
			if err != nil || reused {
				t.Fatalf("incremental build: reused=%t err=%v", reused, err)
			}
			for _, fact := range built.Facts {
				if !strings.HasPrefix(fact.LogicalKey, "fact|"+fact.GetAnchor().GetPath()+"|") {
					t.Fatalf("fact %s has another source's logical key: %s", fact.GetAnchor().GetPath(), fact.LogicalKey)
				}
				if fact.Kind == pb.FactKind_FACT_KIND_FILE && fact.Name != fact.GetAnchor().GetPath() {
					t.Fatalf("file %s has another source's name: %s", fact.GetAnchor().GetPath(), fact.Name)
				}
			}
			if err := idx.Publish(ctx, checkout.root, next, built); err != nil {
				t.Fatal(err)
			}
			diff, err := idx.Diff(ctx, checkout.snap.Id, next.Id, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, facts := range [][]*pb.CodeFact{diff.Facts.Added, diff.Facts.Modified, diff.Facts.Removed} {
				for _, fact := range facts {
					if fact.GetAnchor().GetPath() != "other.go" {
						t.Fatalf("unchanged identical file reported as changed: %s", fact.GetAnchor().GetPath())
					}
				}
			}
		})
	}
}
