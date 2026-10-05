package indexer

import (
	"context"
	"encoding/json"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// TestSymbolLevelReuseOnEdit confirms that editing one declaration reuses the
// facts and chunks of unchanged siblings instead of rebuilding the whole file.
func TestSymbolLevelReuseOnEdit(t *testing.T) {
	ctx := context.Background()
	original := "package a\nfunc Alpha() int { return 1 }\nfunc Beta() int { return 2 }\n"

	first := &graph.Source{Path: "a.go", Language: "go", Text: []byte(original), Hash: graph.Hash([]byte(original))}
	g1 := graph.NewGraph("repo", "snap-1")
	if _, err := syntaxFacts(ctx, g1, first); err != nil {
		t.Fatal(err)
	}
	if first.SyntaxCache == "" {
		t.Fatal("no syntax cache produced")
	}
	cacheBefore := decodeCache(t, first.SyntaxCache)
	betaBefore := findDecl(cacheBefore, "Beta")
	alphaBefore := findDecl(cacheBefore, "Alpha")
	if betaBefore == nil || alphaBefore == nil {
		t.Fatalf("missing declarations in cache: %+v", cacheBefore.Decls)
	}

	// Change only Alpha, keeping its line count so Beta's span is stable.
	edited := "package a\nfunc Alpha() int { return 9 }\nfunc Beta() int { return 2 }\n"
	second := &graph.Source{Path: "a.go", Language: "go", Text: []byte(edited), Hash: graph.Hash([]byte(edited)), SyntaxCache: first.SyntaxCache}
	g2 := graph.NewGraph("repo", "snap-2")
	if _, err := syntaxFacts(ctx, g2, second); err != nil {
		t.Fatal(err)
	}
	cacheAfter := decodeCache(t, second.SyntaxCache)
	betaAfter := findDecl(cacheAfter, "Beta")
	alphaAfter := findDecl(cacheAfter, "Alpha")
	if betaAfter == nil || alphaAfter == nil {
		t.Fatalf("missing declarations after edit: %+v", cacheAfter.Decls)
	}
	if betaAfter.BodyHash != betaBefore.BodyHash {
		t.Fatal("unchanged sibling's body hash moved")
	}
	if alphaAfter.BodyHash == alphaBefore.BodyHash {
		t.Fatal("edited declaration kept its old body hash")
	}
	// Unchanged sibling content is preserved byte-for-byte.
	if betaAfter.Fact.Code != betaBefore.Fact.Code || betaAfter.Fact.Signature != betaBefore.Fact.Signature {
		t.Fatal("unchanged sibling was rebuilt")
	}
	if len(betaAfter.Chunks) != len(betaBefore.Chunks) || betaAfter.Chunks[0].Text != betaBefore.Chunks[0].Text {
		t.Fatal("unchanged sibling chunks were rebuilt")
	}
	if alphaAfter.Fact.Code == alphaBefore.Fact.Code {
		t.Fatal("edited declaration did not update its code")
	}
	if alphaAfter.Fact.Id == betaAfter.Fact.Id {
		t.Fatal("declarations share an id")
	}
}

// TestUnchangedFileAdoptsWithoutReserializing confirms an unchanged file's cache
// is reused verbatim and its facts adopt deterministically into a new snapshot.
func TestUnchangedFileAdoptsWithoutReserializing(t *testing.T) {
	ctx := context.Background()
	text := "package a\nfunc Stable() int { return 1 }\n"
	src := &graph.Source{Path: "a.go", Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
	g1 := graph.NewGraph("repo", "snap-1")
	if _, err := syntaxFacts(ctx, g1, src); err != nil {
		t.Fatal(err)
	}
	cached := src.SyntaxCache
	src2 := &graph.Source{Path: "a.go", Language: "go", Text: []byte(text), Hash: src.Hash, SyntaxCache: cached}
	g2 := graph.NewGraph("repo", "snap-2")
	if _, err := syntaxFacts(ctx, g2, src2); err != nil {
		t.Fatal(err)
	}
	if src2.SyntaxCache != cached {
		t.Fatal("unchanged cache was reserialized")
	}
	var stable *pb.CodeFact
	for _, fact := range g2.Facts {
		if fact.Name == "Stable" {
			stable = fact
		}
	}
	if stable == nil {
		t.Fatal("adopted fact missing")
	}
	if stable.SnapshotId != "snap-2" || len(g2.Chunks) == 0 {
		t.Fatalf("fact/chunks not adopted into new snapshot: %+v", stable)
	}
}

func decodeCache(t *testing.T, raw string) syntaxCache {
	t.Helper()
	var cache syntaxCache
	if err := json.Unmarshal([]byte(raw), &cache); err != nil {
		t.Fatal(err)
	}
	return cache
}

func findDecl(cache syntaxCache, name string) *declCacheEntry {
	for i := range cache.Decls {
		if cache.Decls[i].Name == name {
			return &cache.Decls[i]
		}
	}
	return nil
}

func TestSyntaxCacheRebuildsLegacyLogicalKeys(t *testing.T) {
	text := []byte("class A { run() {} }\nclass B { run() {} }\n")
	src := &graph.Source{Path: "a.ts", Language: "typescript", Text: text, Hash: graph.Hash(text)}
	g := graph.NewGraph("repo", "before")
	if _, err := syntaxFacts(context.Background(), g, src); err != nil {
		t.Fatal(err)
	}
	cache := decodeCache(t, src.SyntaxCache)
	cache.Version = 3
	for _, entry := range cache.Decls {
		entry.Fact.LogicalKey = graph.LogicalFactKey(entry.Fact.Kind, entry.Name, src.Path)
	}
	raw, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	src.SyntaxCache = string(raw)
	next := graph.NewGraph("repo", "after")
	if _, err := syntaxFacts(context.Background(), next, src); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, fact := range next.Facts {
		if fact.Name == "run" {
			keys[fact.LogicalKey] = true
		}
	}
	if len(keys) != 2 || decodeCache(t, src.SyntaxCache).Version != syntaxCacheVersion {
		t.Fatal("legacy cache reused colliding logical keys")
	}
}
