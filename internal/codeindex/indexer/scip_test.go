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

func TestSCIPSynthesizedLogicalKeysIncludeScope(t *testing.T) {
	var previous map[string]bool
	for _, version := range []string{"0.1.0", "0.2.0"} {
		g, source := testSource(t, "lib.rs", "rust", "fn run() {}\nfn run() {}\n")
		doc := &scip.Document{RelativePath: source.Path, Text: string(source.Text)}
		for i, scope := range []string{"A", "B"} {
			symbol := "scip-rust cargo fixture " + version + " " + scope + "#run()."
			doc.Symbols = append(doc.Symbols, &scip.SymbolInformation{Symbol: symbol, DisplayName: "run", Kind: scip.SymbolInformation_Method})
			doc.Occurrences = append(doc.Occurrences, &scip.Occurrence{
				Range: []int32{int32(i), 3, 6}, Symbol: symbol, SymbolRoles: int32(scip.SymbolRole_Definition),
			})
		}
		idx := &scip.Index{Documents: []*scip.Document{doc}}
		if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(marshalIndex(t, idx)), nil, false, true, newSymbols()); err != nil {
			t.Fatal(err)
		}
		keys := map[string]bool{}
		for _, fact := range g.Facts {
			keys[fact.LogicalKey] = true
		}
		if len(keys) != 2 {
			t.Fatalf("SCIP scopes collided: %v", keys)
		}
		if previous != nil {
			for key := range keys {
				if !previous[key] {
					t.Fatalf("package version changed declaration identity: %s", key)
				}
			}
		}
		previous = keys
	}
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

// TestSCIPSynthesizesDeclarationBodies verifies that definitions whose indexer
// omits an enclosing range still capture the full declaration body via the
// tree-sitter declaration resolver, for the languages whose SCIP indexers do
// not populate enclosing ranges.
func TestSCIPSynthesizesDeclarationBodies(t *testing.T) {
	tests := []struct {
		name, path, language, source, symbol, symbolName, marker string
		kind                                                     scip.SymbolInformation_Kind
	}{
		{
			name: "php", path: "src/greet.php", language: "php",
			source:     "<?php\nfunction greet(): string {\n    return \"hi\";\n}\n",
			symbol:     "scip-php composer fixture 0.0.0 src/greet.php/greet().",
			symbolName: "greet", marker: "return \"hi\"",
			kind: scip.SymbolInformation_Function,
		},
		{
			name: "csharp", path: "src/Greeter.cs", language: "csharp",
			source:     "class Greeter {\n    string render() {\n        return \"hi\";\n    }\n}\n",
			symbol:     "scip-dotnet nuget fixture 0.0.0 src/Greeter.cs/Greeter#render().",
			symbolName: "render", marker: "return \"hi\"",
			kind: scip.SymbolInformation_Method,
		},
		{
			name: "dart function", path: "lib/greet.dart", language: "dart",
			source:     "String greet() {\n  return \"hi\";\n}\n",
			symbol:     "scip-dart pub fixture 0.0.0 lib/greet.dart/greet().",
			symbolName: "greet", marker: "return \"hi\"",
			kind: scip.SymbolInformation_Function,
		},
		{
			name: "dart method", path: "lib/greeter.dart", language: "dart",
			source:     "class Greeter {\n  String render() {\n    return \"hi\";\n  }\n}\n",
			symbol:     "scip-dart pub fixture 0.0.0 lib/greeter.dart/Greeter#render().",
			symbolName: "render", marker: "return \"hi\"",
			kind: scip.SymbolInformation_Method,
		},
		{
			name: "dart arrow", path: "lib/arrow.dart", language: "dart",
			source:     "String greet() => \"hi\";\n",
			symbol:     "scip-dart pub fixture 0.0.0 lib/arrow.dart/greet().",
			symbolName: "greet", marker: "=> \"hi\"",
			kind: scip.SymbolInformation_Function,
		},
		{
			name: "csharp expression body", path: "src/Express.cs", language: "csharp",
			source:     "class Widget {\n    string Render() => \"hi\";\n}\n",
			symbol:     "scip-dotnet nuget fixture 0.0.0 src/Express.cs/Widget#Render().",
			symbolName: "Render", marker: "=> \"hi\"",
			kind: scip.SymbolInformation_Method,
		},
		{
			name: "cpp", path: "src/greet.cpp", language: "cpp",
			source:     "int greet() {\n    return 1;\n}\n",
			symbol:     "scip-clang . . src/greet.cpp/greet().",
			symbolName: "greet", marker: "return 1",
			kind: scip.SymbolInformation_Function,
		},
		{
			name: "c", path: "src/greet.c", language: "c",
			source:     "int greet(void) {\n    return 1;\n}\n",
			symbol:     "scip-clang . . src/greet.c/greet().",
			symbolName: "greet", marker: "return 1",
			kind: scip.SymbolInformation_Function,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, s := testSource(t, tt.path, tt.language, tt.source)
			line, start, end := position(t, s.Text, tt.symbolName, 0)
			idx := &scip.Index{
				Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "fixture", Version: "1"}},
				Documents: []*scip.Document{{
					RelativePath: tt.path,
					Text:         tt.source,
					Occurrences: []*scip.Occurrence{{
						Range:       []int32{int32(line), int32(start), int32(end)},
						Symbol:      tt.symbol,
						SymbolRoles: int32(scip.SymbolRole_Definition),
					}},
					Symbols: []*scip.SymbolInformation{{
						Symbol:      tt.symbol,
						Kind:        tt.kind,
						DisplayName: tt.symbolName,
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
				if f.Name != tt.symbolName {
					t.Fatalf("name: %q", f.Name)
				}
				if strings.TrimSpace(f.Code) == tt.symbolName {
					t.Fatalf("captured only the symbol name: %q", f.Code)
				}
				if !strings.Contains(f.Code, tt.marker) {
					t.Fatalf("captured code does not span the declaration body: %q", f.Code)
				}
			}
			if len(g.Chunks) == 0 {
				t.Fatal("synthesized fact produced no chunks")
			}
		})
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

// TestApplyRustImplsFromSource covers rust-analyzer's missing
// is_implementation relationships: tree-sitter finds the impl block and the
// SCIP occurrences resolve the trait and type identifiers to facts.
func TestApplyRustImplsFromSource(t *testing.T) {
	text := "trait Greeter {}\nstruct English;\nimpl Greeter for English {}\n"
	g, s := testSource(t, "src/lib.rs", "rust", text)
	prefix := "rust-analyzer cargo fixture 0.1.0 "
	english := g.AddFact(pb.FactKind_FACT_KIND_STRUCT, "English", "rust", &pb.SourceAnchor{Path: s.Path, StartByte: 17, EndByte: 24}, "", "", nil)
	greeter := g.AddFact(pb.FactKind_FACT_KIND_INTERFACE, "Greeter", "rust", &pb.SourceAnchor{Path: s.Path, StartByte: 0, EndByte: 16}, "", "", nil)
	traitAt := strings.LastIndex(text, "Greeter")
	typeAt := strings.LastIndex(text, "English")

	table := newSymbols()
	table.definitions[prefix+"English#"] = english.Id
	table.definitions[prefix+"Greeter#"] = greeter.Id
	table.references = []occurrence{
		{key: prefix + "Greeter#", anchor: &pb.SourceAnchor{Path: s.Path, StartByte: uint32(traitAt), EndByte: uint32(traitAt + len("Greeter"))}, scipBacked: true},
		{key: prefix + "English#", anchor: &pb.SourceAnchor{Path: s.Path, StartByte: uint32(typeAt), EndByte: uint32(typeAt + len("English"))}, scipBacked: true},
	}
	table.applyRustImpls(g)

	for _, e := range g.EdgeFacts {
		if e.Kind == pb.EdgeKind_EDGE_KIND_IMPLEMENTS && e.FromFactId == english.Id && e.ToFactId == greeter.Id {
			return
		}
	}
	t.Fatal("no IMPLEMENTS edge derived from the impl block")
}

// TestApplyRustImplsExternalTrait covers a trait from another crate: the type is
// local but the trait is unresolved, so the edge keeps the external symbol key.
func TestApplyRustImplsExternalTrait(t *testing.T) {
	text := "struct English;\nimpl Display for English {}\n"
	g, s := testSource(t, "src/lib.rs", "rust", text)
	prefix := "rust-analyzer cargo fixture 0.1.0 "
	english := g.AddFact(pb.FactKind_FACT_KIND_STRUCT, "English", "rust", &pb.SourceAnchor{Path: s.Path, StartByte: 0, EndByte: 15}, "", "", nil)
	traitAt := strings.LastIndex(text, "Display")
	typeAt := strings.LastIndex(text, "English")

	table := newSymbols()
	table.definitions[prefix+"English#"] = english.Id
	table.references = []occurrence{
		{key: prefix + "Display#", anchor: &pb.SourceAnchor{Path: s.Path, StartByte: uint32(traitAt), EndByte: uint32(traitAt + len("Display"))}, scipBacked: true},
		{key: prefix + "English#", anchor: &pb.SourceAnchor{Path: s.Path, StartByte: uint32(typeAt), EndByte: uint32(typeAt + len("English"))}, scipBacked: true},
	}
	table.applyRustImpls(g)

	for _, e := range g.EdgeFacts {
		if e.Kind == pb.EdgeKind_EDGE_KIND_IMPLEMENTS && e.FromFactId == english.Id && e.ToFactId == "" && e.TargetSymbolKey == prefix+"Display#" {
			return
		}
	}
	t.Fatal("no external IMPLEMENTS edge derived")
}

// TestSCIPApplyInfersCallsWithoutSymbolKind covers indexers such as scip-ruby
// and scip-clang that omit SymbolInformation.Kind: a reference to a method fact
// must still produce a CALLS edge.
func TestSCIPApplyInfersCallsWithoutSymbolKind(t *testing.T) {
	text := "def caller\n  helper\nend\n"
	g, s := testSource(t, "a.rb", "ruby", text)
	owner := g.AddFact(pb.FactKind_FACT_KIND_METHOD, "caller", "ruby", &pb.SourceAnchor{Path: s.Path, StartByte: 0, EndByte: 24}, text, "", nil)
	def := g.AddFact(pb.FactKind_FACT_KIND_METHOD, "helper", "ruby", &pb.SourceAnchor{Path: s.Path, StartByte: 20, EndByte: 26}, "", "", nil)
	sym := "scip-ruby . a.rb/helper()."
	table := newSymbols()
	table.definitions[sym] = def.Id
	table.references = []occurrence{{
		key:        sym,
		anchor:     &pb.SourceAnchor{Path: s.Path, StartByte: 13, EndByte: 19},
		version:    "1",
		scipBacked: true,
	}}
	table.apply(g)

	calls := 0
	for _, e := range g.EdgeFacts {
		if e.Kind == pb.EdgeKind_EDGE_KIND_CALLS && e.FromFactId == owner.Id && e.ToFactId == def.Id {
			calls++
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 inferred from definition kind", calls)
	}
}
