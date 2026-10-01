package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/parser"
)

func TestInfraFactsPublishedOnGraph(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:\n  api:\n    image: postgres:16\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newGraphForInfraTest(t, dir)
	if len(g.Facts) == 0 {
		t.Fatal("expected infra facts on graph")
	}
	for _, f := range g.Facts {
		if f.Anchor == nil || f.LogicalKey == "" {
			t.Fatalf("infra fact lacks anchor or logical key: %+v", f)
		}
	}
}

func newGraphForInfraTest(t *testing.T, dir string) *graph.Graph {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	src := &graph.Source{Path: "compose.yml", Text: body, Hash: graph.Hash(body)}
	sources := map[string]*graph.Source{"compose.yml": src}
	facts, err := parser.Scan(context.Background(), dir, sources)
	if err != nil {
		t.Fatal(err)
	}
	g := graph.NewGraph("repo", "snapshot")
	g.Sources["compose.yml"] = src
	for _, f := range facts {
		start, end := byteOffset(src.Text, f.Line, f.Column, f.EndLine, f.EndColumn)
		g.AddInfraFact(f.Kind, f.Subject, f.Object, src.Language, f.Extractor, src.Anchor(start, end), f.Text)
	}
	return g
}
