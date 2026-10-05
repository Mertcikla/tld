package indexer

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// TestDeclarationIndexParsesCSharp verifies the grammar fallback still captures
// full declarations while the parse runs under the abandonment deadline.
func TestDeclarationIndexParsesCSharp(t *testing.T) {
	text := "class Widget {\n    string Render() => \"hi\";\n}\n"
	src := &graph.Source{Path: "src/Express.cs", Language: "csharp", Text: []byte(text)}
	d := newDeclarationIndex(src)
	if d == nil {
		t.Skip("no csharp grammar available")
	}
	defer d.Release()

	at := strings.Index(text, "Render")
	start, end, ok := d.span(at, at+len("Render"))
	if !ok {
		t.Fatal("expected an enclosing declaration for Render")
	}
	if got := text[start:end]; !strings.Contains(got, "=> \"hi\"") {
		t.Fatalf("span = %q, want the method body", got)
	}
}

// TestParseDeclarationTreeReturnsValidTree covers the deadline-wrapped parser
// used by the declaration fallback.
func TestParseDeclarationTreeReturnsValidTree(t *testing.T) {
	lang := parserLanguage("rust")
	if lang == nil {
		t.Skip("no rust grammar available")
	}
	tree := parseDeclarationTree(lang, []byte("fn run() {}\n"))
	if tree == nil {
		t.Fatal("expected a parsed tree")
	}
	tree.Release()
}
