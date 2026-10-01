package indexer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func marshalIndex(t *testing.T, idx *scip.Index) []byte {
	t.Helper()
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// position returns the zero-based line and character bounds of the nth
// occurrence of needle in src. It assumes ASCII source, which keeps character
// and byte columns aligned.
func position(t *testing.T, src []byte, needle string, nth int) (line, start, end int) {
	t.Helper()
	at := -1
	for i := 0; i <= nth; i++ {
		next := bytes.Index(src[at+1:], []byte(needle))
		if next < 0 {
			t.Fatalf("%q occurrence %d not found", needle, nth)
		}
		at += 1 + next
	}
	prefix := src[:at]
	line = bytes.Count(prefix, []byte("\n"))
	lineStart := bytes.LastIndexByte(prefix, '\n') + 1
	start = at - lineStart
	return line, start, start + len(needle)
}

func testSource(t *testing.T, path, language, text string) (*graph.Graph, *graph.Source) {
	t.Helper()
	src := &graph.Source{Path: path, Language: language, Text: []byte(text), Hash: graph.Hash([]byte(text))}
	g := graph.NewGraph("repo", "snap")
	g.Sources[src.Path] = src
	return g, src
}

// TestSCIPSynthesizesDefinitionFact verifies that a definition occurrence with
// an enclosing range and a symbol kind becomes a code Fact with captured code
// and chunks, without any tree-sitter grammar for the language.
func TestSCIPSynthesizesDefinitionFact(t *testing.T) {
	text := "fn greet(name: &str) -> String {\n    format!(\"hi {}\", name)\n}\n"
	g, s := testSource(t, "src/lib.rs", "rust", text)
	symbol := "scip-rust cargo lib 0.1.0 src/lib.rs/greet()."
	line, start, end := position(t, s.Text, "greet", 0)
	idx := &scip.Index{
		Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "scip-rust", Version: "1"}},
		Documents: []*scip.Document{{
			RelativePath: "src/lib.rs",
			Text:         text,
			Occurrences: []*scip.Occurrence{{
				Range:       []int32{int32(line), int32(start), int32(end)},
				Symbol:      symbol,
				SymbolRoles: int32(scip.SymbolRole_Definition),
				TypedEnclosingRange: &scip.Occurrence_MultiLineEnclosingRange{
					MultiLineEnclosingRange: &scip.MultiLineRange{StartLine: 0, StartCharacter: 0, EndLine: 2, EndCharacter: 1},
				},
			}},
			Symbols: []*scip.SymbolInformation{{
				Symbol:                 symbol,
				Kind:                   scip.SymbolInformation_Function,
				DisplayName:            "greet",
				Documentation:          []string{"Say hello."},
				SignatureDocumentation: &scip.Signature{Text: "fn greet(name: &str) -> String"},
			}},
		}},
	}
	table := newSymbols()
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(marshalIndex(t, idx)), nil, false, true, table); err != nil {
		t.Fatal(err)
	}
	if len(g.Facts) != 1 {
		t.Fatalf("facts: %d", len(g.Facts))
	}
	for _, f := range g.Facts {
		if f.Kind != pb.FactKind_FACT_KIND_FUNCTION || f.Name != "greet" || f.Language != "rust" {
			t.Fatalf("fact: %+v", f)
		}
		if !strings.Contains(f.Code, "format!") {
			t.Fatalf("captured code does not span the declaration: %q", f.Code)
		}
		if f.SymbolKey != symbol || f.Documentation != "Say hello." {
			t.Fatalf("symbol metadata missing: %+v", f)
		}
		if f.Signature != "fn greet(name: &str) -> String" {
			t.Fatalf("signature: %q", f.Signature)
		}
	}
	if len(g.Chunks) == 0 {
		t.Fatal("synthesized fact produced no chunks")
	}
	if table.definitions[symbol] == "" {
		t.Fatal("definition not registered")
	}
}

// TestSCIPSkipsNonDeclarationKinds verifies that symbol kinds outside the
// code-bearing declaration set are not synthesized as Facts.
func TestSCIPSkipsNonDeclarationKinds(t *testing.T) {
	text := "let counter = 0;\n"
	g, s := testSource(t, "src/main.rs", "rust", text)
	symbol := "scip-rust cargo lib 0.1.0 src/main.rs/counter."
	line, start, end := position(t, s.Text, "counter", 0)
	idx := &scip.Index{
		Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "scip-rust", Version: "1"}},
		Documents: []*scip.Document{{
			RelativePath: "src/main.rs",
			Text:         text,
			Occurrences: []*scip.Occurrence{{
				Range:       []int32{int32(line), int32(start), int32(end)},
				Symbol:      symbol,
				SymbolRoles: int32(scip.SymbolRole_Definition),
			}},
			Symbols: []*scip.SymbolInformation{{Symbol: symbol, Kind: scip.SymbolInformation_StaticVariable, DisplayName: "counter"}},
		}},
	}
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(marshalIndex(t, idx)), nil, false, true, newSymbols()); err != nil {
		t.Fatal(err)
	}
	if len(g.Facts) != 0 {
		t.Fatalf("non-declaration kind synthesized: %d facts", len(g.Facts))
	}
}

// TestSCIPFallsBackToDescriptorSuffix verifies synthesis when an indexer omits
// SymbolInformation.Kind and only the SCIP descriptor suffix identifies a
// callable declaration.
func TestSCIPFallsBackToDescriptorSuffix(t *testing.T) {
	text := "fn compute() {}\n"
	g, s := testSource(t, "src/main.rs", "rust", text)
	symbol := "scip-rust cargo lib 0.1.0 src/main.rs/compute()."
	line, start, end := position(t, s.Text, "compute", 0)
	idx := &scip.Index{
		Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "scip-rust", Version: "1"}},
		Documents: []*scip.Document{{
			RelativePath: "src/main.rs",
			Text:         text,
			Occurrences: []*scip.Occurrence{{
				Range:       []int32{int32(line), int32(start), int32(end)},
				Symbol:      symbol,
				SymbolRoles: int32(scip.SymbolRole_Definition),
			}},
		}},
	}
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(marshalIndex(t, idx)), nil, false, true, newSymbols()); err != nil {
		t.Fatal(err)
	}
	if len(g.Facts) != 1 {
		t.Fatalf("facts: %d", len(g.Facts))
	}
	for _, f := range g.Facts {
		if f.Kind != pb.FactKind_FACT_KIND_FUNCTION {
			t.Fatalf("descriptor fallback kind: %s", f.Kind)
		}
	}
}

// TestSCIPSkipsMalformedOccurrence verifies that an occurrence with an
// out-of-bounds range is skipped without failing the document.
func TestSCIPSkipsMalformedOccurrence(t *testing.T) {
	text := "fn greet() {}\n"
	g, s := testSource(t, "src/main.rs", "rust", text)
	good := "scip-rust cargo lib 0.1.0 src/main.rs/greet()."
	bad := "scip-rust cargo lib 0.1.0 src/main.rs/broken()."
	line, start, end := position(t, s.Text, "greet", 0)
	idx := &scip.Index{
		Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "scip-rust", Version: "1"}},
		Documents: []*scip.Document{{
			RelativePath: "src/main.rs",
			Text:         text,
			Occurrences: []*scip.Occurrence{
				{Range: []int32{int32(line), int32(start), int32(end)}, Symbol: good, SymbolRoles: int32(scip.SymbolRole_Definition)},
				{Range: []int32{int32(line), 0, 9999}, Symbol: bad, SymbolRoles: int32(scip.SymbolRole_Definition)},
			},
			Symbols: []*scip.SymbolInformation{
				{Symbol: good, Kind: scip.SymbolInformation_Function, DisplayName: "greet"},
				{Symbol: bad, Kind: scip.SymbolInformation_Function, DisplayName: "broken"},
			},
		}},
	}
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(marshalIndex(t, idx)), nil, false, true, newSymbols()); err != nil {
		t.Fatal(err)
	}
	if len(g.Facts) != 1 {
		t.Fatalf("malformed occurrence not skipped: %d facts", len(g.Facts))
	}
	for _, f := range g.Facts {
		if f.Name != "greet" {
			t.Fatalf("unexpected fact: %s", f.Name)
		}
	}
}
func TestSCIPCallableReferenceEmitsCalls(t *testing.T) {
	text := "fn helper() {}\nfn caller() { helper(); }\n"
	g, s := testSource(t, "src/main.rs", "rust", text)
	helper := "scip-rust cargo lib 0.1.0 src/main.rs/helper()."
	caller := "scip-rust cargo lib 0.1.0 src/main.rs/caller()."
	hl, hs, he := position(t, s.Text, "helper", 0)
	cl, cs, ce := position(t, s.Text, "caller", 0)
	rl, rs, re := position(t, s.Text, "helper", 1)
	enclosing := func(line int32, endChar int32) *scip.Occurrence_MultiLineEnclosingRange {
		return &scip.Occurrence_MultiLineEnclosingRange{MultiLineEnclosingRange: &scip.MultiLineRange{StartLine: line, StartCharacter: 0, EndLine: line, EndCharacter: endChar}}
	}
	idx := &scip.Index{
		Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "scip-rust", Version: "1"}},
		Documents: []*scip.Document{{
			RelativePath: "src/main.rs",
			Text:         text,
			Occurrences: []*scip.Occurrence{
				{Range: []int32{int32(hl), int32(hs), int32(he)}, Symbol: helper, SymbolRoles: int32(scip.SymbolRole_Definition), TypedEnclosingRange: enclosing(0, 14)},
				{Range: []int32{int32(cl), int32(cs), int32(ce)}, Symbol: caller, SymbolRoles: int32(scip.SymbolRole_Definition), TypedEnclosingRange: enclosing(1, 25)},
				{Range: []int32{int32(rl), int32(rs), int32(re)}, Symbol: helper},
			},
			Symbols: []*scip.SymbolInformation{
				{Symbol: helper, Kind: scip.SymbolInformation_Function, DisplayName: "helper"},
				{Symbol: caller, Kind: scip.SymbolInformation_Function, DisplayName: "caller"},
			},
		}},
	}
	table := newSymbols()
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(marshalIndex(t, idx)), nil, false, true, table); err != nil {
		t.Fatal(err)
	}
	table.apply(g)
	calls, references := 0, 0
	for _, e := range g.EdgeFacts {
		switch e.Kind {
		case pb.EdgeKind_EDGE_KIND_CALLS:
			calls++
			if e.FromFactId != table.definitions[caller] || e.ToFactId != table.definitions[helper] {
				t.Fatalf("unexpected call edge: %+v", e)
			}
		case pb.EdgeKind_EDGE_KIND_REFERENCES:
			references++
		}
	}
	if calls != 1 || references != 1 {
		t.Fatalf("calls=%d references=%d", calls, references)
	}
}
