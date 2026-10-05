package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
)

func TestDiffDistinguishesDeclarationScopes(t *testing.T) {
	for _, test := range []struct{ name, before, after string }{
		{"methods.ts", "class A { run() { return 1; } }\nclass B {}\n", "class A { run() { return 1; } }\nclass B { run() { return 2; } }\n"},
		// Insert the new receiver's method before the old one to also verify
		// identity is independent of other scopes' declaration order.
		{"methods.go", "package demo\ntype A struct{}\ntype B struct{}\nfunc (a A) run() int { return 1 }\n", "package demo\ntype A struct{}\ntype B struct{}\nfunc (b B) run() int { return 2 }\nfunc (a A) run() int { return 1 }\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			path := filepath.Join(root, test.name)
			if err := os.WriteFile(path, []byte(test.before), 0600); err != nil {
				t.Fatal(err)
			}
			pipeline := indexer.Pipeline{Config: config.Default()}
			req := &pb.IndexRequest{Directory: root}
			before, beforeGraph, err := pipeline.Build(ctx, req, nil)
			if err != nil {
				t.Fatal(err)
			}
			idx, handle := openTestStore(t)
			defer func() { _ = handle.Close() }()
			if err := idx.Publish(ctx, root, before, beforeGraph); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.after), 0600); err != nil {
				t.Fatal(err)
			}
			hashes := map[string]string{test.name: beforeGraph.Sources[test.name].Hash}
			after, afterGraph, _, err := pipeline.BuildIncremental(ctx, req, nil, &indexer.IncrementalBase{Snapshot: before, Graph: beforeGraph, Sources: hashes})
			if err != nil {
				t.Fatal(err)
			}
			if err := idx.Publish(ctx, root, after, afterGraph); err != nil {
				t.Fatal(err)
			}
			keys := map[string]bool{}
			for _, fact := range afterGraph.Facts {
				if fact.Name == "run" {
					keys[fact.LogicalKey] = true
				}
			}
			if len(keys) != 2 {
				t.Fatalf("same-named methods collided: %v", keys)
			}
			diff, err := idx.Diff(ctx, before.Id, after.Id, false)
			if err != nil {
				t.Fatal(err)
			}
			added := 0
			for _, fact := range diff.Facts.Added {
				if fact.Name == "run" {
					added++
				}
			}
			if added != 1 {
				t.Fatalf("added methods = %d, want 1; diff=%v", added, diff.Facts)
			}
			for _, fact := range diff.Facts.Removed {
				if fact.Name == "run" {
					t.Fatal("existing method was reported removed")
				}
			}
			for _, fact := range diff.Facts.Modified {
				if fact.Name == "run" {
					t.Fatal("existing method was reported modified")
				}
			}
		})
	}
}
