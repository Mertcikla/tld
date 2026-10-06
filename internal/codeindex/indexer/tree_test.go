package indexer

import (
	"context"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/odvcencio/gotreesitter"
)

func TestTreeFactsOnlyCodeDeclarations(t *testing.T) {
	tests := []struct {
		path, language, source string
		want                   map[string]pb.FactKind
	}{
		{"a.go", "go", "package a\ntype Item struct{}\nvar count = 1\nfunc Run(){}\nfunc (i Item) Act(){}\n", map[string]pb.FactKind{"Item": pb.FactKind_FACT_KIND_STRUCT, "Run": pb.FactKind_FACT_KIND_FUNCTION, "Act": pb.FactKind_FACT_KIND_METHOD}},
		{"a.ts", "typescript", "class Box { open() {} }\nconst make = () => 1;\nconst value = 2;\nfunction run(){ make(); }\n", map[string]pb.FactKind{"Box": pb.FactKind_FACT_KIND_CLASS, "open": pb.FactKind_FACT_KIND_METHOD, "make": pb.FactKind_FACT_KIND_FUNCTION, "run": pb.FactKind_FACT_KIND_FUNCTION}},
		{"a.js", "javascript", "class Box { open() {} }\nconst make = (x) => x;\nconst value = 2;\nfunction run(){ make(1); }\n", map[string]pb.FactKind{"Box": pb.FactKind_FACT_KIND_CLASS, "open": pb.FactKind_FACT_KIND_METHOD, "make": pb.FactKind_FACT_KIND_FUNCTION, "run": pb.FactKind_FACT_KIND_FUNCTION}},
		{"a.py", "python", "def top(): pass\nclass Box:\n    def open(self): pass\n    def __init__(self): pass\nhelper = 3\n", map[string]pb.FactKind{"top": pb.FactKind_FACT_KIND_FUNCTION, "Box": pb.FactKind_FACT_KIND_CLASS, "open": pb.FactKind_FACT_KIND_METHOD, "__init__": pb.FactKind_FACT_KIND_METHOD}},
		{"a.php", "php", "<?php\nfunction top() {}\nclass Box { public function open() {} }\n", map[string]pb.FactKind{"top": pb.FactKind_FACT_KIND_FUNCTION, "Box": pb.FactKind_FACT_KIND_CLASS, "open": pb.FactKind_FACT_KIND_METHOD}},
		{"a.cs", "csharp", "class Box { void Open() {} }\n", map[string]pb.FactKind{"Box": pb.FactKind_FACT_KIND_CLASS, "Open": pb.FactKind_FACT_KIND_METHOD}},
		{"a.dart", "dart", "void main() {}\nclass Box { void open() {} }\n", map[string]pb.FactKind{"main": pb.FactKind_FACT_KIND_FUNCTION, "Box": pb.FactKind_FACT_KIND_CLASS, "open": pb.FactKind_FACT_KIND_METHOD}},
		{"a.cpp", "cpp", "int run() { return 0; }\nclass Box { public: void open() {} };\n", map[string]pb.FactKind{"run": pb.FactKind_FACT_KIND_FUNCTION, "Box": pb.FactKind_FACT_KIND_CLASS, "open": pb.FactKind_FACT_KIND_METHOD}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			src := &graph.Source{Path: tt.path, Language: tt.language, Text: []byte(tt.source), Hash: graph.Hash([]byte(tt.source))}
			g := graph.NewGraph("repo", "snap")
			g.Sources[src.Path] = src
			if _, err := treeFacts(context.Background(), g, src); err != nil {
				t.Fatal(err)
			}
			if len(g.Facts) != len(tt.want) {
				t.Fatalf("facts=%d want=%d", len(g.Facts), len(tt.want))
			}
			for _, f := range g.Facts {
				if tt.want[f.Name] != f.Kind {
					t.Fatalf("unexpected fact %s: %s", f.Name, f.Kind)
				}
			}
		})
	}
}
func TestPythonAndJavaScriptImports(t *testing.T) {
	tests := []struct {
		path, language, source string
		want                   []string
	}{
		{"a.py", "python", "import os\nimport numpy as np\nfrom flask import Flask\n", []string{"os", "numpy", "flask"}},
		{"a.js", "javascript", "const fs = require('fs');\nimport x from 'pkg';\n", []string{"fs", "pkg"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			lang := parserLanguage(tt.language)
			if lang == nil {
				t.Fatalf("no grammar for %s", tt.language)
			}
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			got := fileImports(wrapNode(tree.RootNode(), lang), []byte(tt.source))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("imports: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestJavaScriptAndPythonCallsResolve(t *testing.T) {
	tests := []struct {
		path, language, source, caller, callee string
	}{
		{"a.js", "javascript", "function a(){ b(); }\nfunction b(){}\n", "a", "b"},
		{"a.py", "python", "def a():\n    b()\ndef b():\n    pass\n", "a", "b"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			src := &graph.Source{Path: tt.path, Language: tt.language, Text: []byte(tt.source), Hash: graph.Hash([]byte(tt.source))}
			g := graph.NewGraph("repo", "snap")
			g.Sources[src.Path] = src
			sites, err := treeFacts(context.Background(), g, src)
			if err != nil {
				t.Fatal(err)
			}
			var caller, callee *pb.CodeFact
			for _, f := range g.Facts {
				switch f.Name {
				case tt.caller:
					caller = f
				case tt.callee:
					callee = f
				}
			}
			if caller == nil || callee == nil {
				t.Fatalf("missing declarations: %+v", g.Facts)
			}
			callSite := strings.Index(tt.source, tt.callee+"()")
			table := newSymbols()
			table.definitions[tt.callee] = callee.Id
			table.references = []occurrence{{key: tt.callee, anchor: src.Anchor(callSite, callSite+len(tt.callee))}}
			table.apply(g)
			deriveCalls(g, sites, table)
			calls := 0
			for _, e := range g.EdgeFacts {
				if e.Kind == pb.EdgeKind_EDGE_KIND_CALLS && e.FromFactId == caller.Id && e.ToFactId == callee.Id {
					calls++
				}
			}
			if calls != 1 {
				t.Fatalf("resolved calls=%d want 1 (%+v)", calls, g.EdgeFacts)
			}
		})
	}
}

func TestCallsUseReferenceSitesWithoutOccurrenceFacts(t *testing.T) {
	source := []byte("package a\nfunc A(){ B(); B() }\nfunc B(){}\n")
	src := &graph.Source{Path: "a.go", Language: "go", Text: source, Hash: graph.Hash(source)}
	g := graph.NewGraph("repo", "snap")
	g.Sources[src.Path] = src
	sites, err := treeFacts(context.Background(), g, src)
	if err != nil {
		t.Fatal(err)
	}
	var a, b *pb.CodeFact
	for _, f := range g.Facts {
		if f.Name == "A" {
			a = f
		}
		if f.Name == "B" {
			b = f
		}
	}
	if a == nil || b == nil {
		t.Fatalf("missing declarations: %+v", g.Facts)
	}
	key := "B"
	table := newSymbols()
	table.definitions[key] = b.Id
	first := strings.Index(string(source), "B()")
	second := strings.LastIndex(string(source[:strings.Index(string(source), "func B")]), "B()")
	table.references = []occurrence{{key: key, anchor: src.Anchor(first, first+1)}, {key: key, anchor: src.Anchor(second, second+1)}}
	table.apply(g)
	deriveCalls(g, sites, table)
	if len(g.Facts) != 2 || len(g.EdgeFacts) != 4 {
		t.Fatalf("facts=%d edgefacts=%d", len(g.Facts), len(g.EdgeFacts))
	}
	calls := 0
	for _, e := range g.EdgeFacts {
		if e.Kind == pb.EdgeKind_EDGE_KIND_CALLS {
			calls++
			if e.FromFactId != a.Id || e.ToFactId != b.Id {
				t.Fatalf("call %+v", e)
			}
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRelationshipPolicyKeepsExternalCalls(t *testing.T) {
	source := []byte("package a\nfunc A(){ B(); fmt.Println(\"x\") }\nfunc B(){}\n")
	src := &graph.Source{Path: "a.go", Language: "go", Text: source, Hash: graph.Hash(source)}
	g := graph.NewGraph("repo", "snap")
	g.Sources[src.Path] = src
	sites, err := treeFacts(context.Background(), g, src)
	if err != nil {
		t.Fatal(err)
	}
	var a, b *pb.CodeFact
	for _, fact := range g.Facts {
		switch fact.Name {
		case "A":
			a = fact
		case "B":
			b = fact
		}
	}
	if a == nil || b == nil {
		t.Fatal("missing code Facts")
	}
	bCall := strings.Index(string(source), "B();")
	printCall := strings.Index(string(source), "Println")
	unknown := strings.Index(string(source), "fmt")
	table := newSymbols()
	table.definitions["B"] = b.Id
	table.references = []occurrence{
		{key: "B", anchor: src.Anchor(bCall, bCall+1)},
		{key: "B", anchor: src.Anchor(0, 1)}, // no enclosing code Fact
		{key: "external.Println", anchor: src.Anchor(printCall, printCall+7)},
		{key: "external.package", anchor: src.Anchor(unknown, unknown+3)},
	}
	table.apply(g)
	deriveCalls(g, sites, table)
	var references, calls, externalCalls int
	for _, edge := range g.EdgeFacts {
		if edge.FromFactId != a.Id {
			t.Fatalf("relationship has no code source: %+v", edge)
		}
		switch edge.Kind {
		case pb.EdgeKind_EDGE_KIND_REFERENCES:
			references++
			if edge.ToFactId != b.Id || edge.TargetSymbolKey != "" {
				t.Fatalf("non-code reference retained: %+v", edge)
			}
		case pb.EdgeKind_EDGE_KIND_CALLS:
			calls++
			if edge.TargetSymbolKey == "external.Println" {
				externalCalls++
			}
		}
	}
	if references != 1 || calls != 2 || externalCalls != 1 {
		t.Fatalf("references=%d calls=%d externalCalls=%d", references, calls, externalCalls)
	}
}
