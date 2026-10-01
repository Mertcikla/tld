package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func TestSCIPUTF16AndDirectEdgeFacts(t *testing.T) {
	source := []byte("const emoji = \"🚀\"; function answer(){}; function use(){ answer(); }\n")
	s := &graph.Source{Path: "a.ts", Language: "typescript", Text: source, Hash: graph.Hash(source)}
	g := graph.NewGraph("repo", "snapshot")
	g.Sources[s.Path] = s
	if _, err := treeFacts(context.Background(), g, s); err != nil {
		t.Fatal(err)
	}
	definition := strings.Index(string(source), "answer")
	reference := strings.LastIndex(string(source), "answer")
	utf16Column := func(offset int) int32 { return int32(len(utf16.Encode([]rune(string(source[:offset]))))) }
	symbol := "scip-typescript npm x 1 answer."
	idx := &scip.Index{Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "fixture", Version: "1"}}, Documents: []*scip.Document{{RelativePath: "a.ts", PositionEncoding: scip.PositionEncoding_UTF16CodeUnitOffsetFromLineStart, Text: string(source), Occurrences: []*scip.Occurrence{{Range: []int32{0, utf16Column(definition), utf16Column(definition + 6)}, Symbol: symbol, SymbolRoles: int32(scip.SymbolRole_Definition)}, {Range: []int32{0, utf16Column(reference), utf16Column(reference + 6)}, Symbol: symbol}}, Symbols: []*scip.SymbolInformation{{Symbol: symbol, DisplayName: "answer", Documentation: []string{"answer docs"}}}}}}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	table := newSymbols()
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(data), nil, true, false, table); err != nil {
		t.Fatal(err)
	}
	table.apply(g)
	if len(g.Facts) != 2 {
		t.Fatalf("Facts: %d", len(g.Facts))
	}
	for _, f := range g.Facts {
		if f.Name == "answer" && (f.SymbolKey != symbol || f.Documentation != "answer docs") {
			t.Fatalf("Fact: %+v", f)
		}
	}
	if len(g.EdgeFacts) != 1 {
		t.Fatalf("EdgeFacts: %d", len(g.EdgeFacts))
	}
	for _, e := range g.EdgeFacts {
		if e.Anchor.StartByte != uint32(reference) || e.ToFactId == "" {
			t.Fatalf("reference: %+v", e)
		}
	}
}

func TestSCIPSymbolRelationships(t *testing.T) {
	source := []byte("interface Animal {}\nclass Dog implements Animal {}\nfunction adopt(): Dog { return new Dog() }\n")
	s := &graph.Source{Path: "a.ts", Language: "typescript", Text: source, Hash: graph.Hash(source)}
	g := graph.NewGraph("repo", "snapshot")
	g.Sources[s.Path] = s
	if _, err := treeFacts(context.Background(), g, s); err != nil {
		t.Fatal(err)
	}
	animal := "scip-typescript npm fixture 1 Animal#"
	dog := "scip-typescript npm fixture 1 Dog#"
	adopt := "scip-typescript npm fixture 1 adopt()."
	external := "scip-typescript npm external 1 Base#"
	definition := func(name, symbol string) *scip.Occurrence {
		start := strings.Index(string(source), name)
		return &scip.Occurrence{Range: []int32{int32(strings.Count(string(source[:start]), "\n")), int32(start - strings.LastIndex(string(source[:start]), "\n") - 1), int32(start - strings.LastIndex(string(source[:start]), "\n") - 1 + len(name))}, Symbol: symbol, SymbolRoles: int32(scip.SymbolRole_Definition)}
	}
	idx := &scip.Index{Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "fixture", Version: "1"}}, Documents: []*scip.Document{{RelativePath: "a.ts", Text: string(source), Occurrences: []*scip.Occurrence{definition("Animal", animal), definition("Dog", dog), definition("adopt", adopt)}, Symbols: []*scip.SymbolInformation{
		{Symbol: animal},
		{Symbol: dog, Relationships: []*scip.Relationship{{Symbol: animal, IsImplementation: true}, {Symbol: external, IsImplementation: true}}},
		{Symbol: adopt, Relationships: []*scip.Relationship{{Symbol: dog, IsTypeDefinition: true}}},
	}}}}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	table := newSymbols()
	if err := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(data), nil, true, false, table); err != nil {
		t.Fatal(err)
	}
	table.apply(g)
	if len(g.EdgeFacts) != 3 {
		t.Fatalf("EdgeFacts: %d; definitions: %+v", len(g.EdgeFacts), table.definitions)
	}
	var internalImplementation, externalImplementation, typeDefinition bool
	for _, edge := range g.EdgeFacts {
		if len(edge.Evidence) != 1 || edge.Evidence[0].Producer != "scip" || edge.Evidence[0].Version != "1" {
			t.Fatalf("missing SCIP evidence: %+v", edge)
		}
		switch {
		case edge.Kind == pb.EdgeKind_EDGE_KIND_IMPLEMENTS && edge.FromFactId == table.definitions[dog] && edge.ToFactId == table.definitions[animal]:
			internalImplementation = true
		case edge.Kind == pb.EdgeKind_EDGE_KIND_IMPLEMENTS && edge.FromFactId == table.definitions[dog] && edge.TargetSymbolKey == external:
			externalImplementation = true
		case edge.Kind == pb.EdgeKind_EDGE_KIND_TYPE_DEFINITION && edge.FromFactId == table.definitions[adopt] && edge.ToFactId == table.definitions[dog]:
			typeDefinition = true
		default:
			t.Fatalf("unexpected relationship: %+v", edge)
		}
	}
	if !internalImplementation || !externalImplementation || !typeDefinition {
		t.Fatal("missing relationship kind or target")
	}
}

func TestDiscoverExcludesTestSources(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":               "module example.com/fixture\n\ngo 1.26\n",
		"tsconfig.json":        "{}",
		"pyproject.toml":       "[project]\nname = \"fixture\"\n",
		"main.go":              "package fixture\nfunc Main(){}\n",
		"main_test.go":         "package fixture\nfunc TestMain(){}\n",
		"widget.ts":            "export function widget() {}\n",
		"widget.test.ts":       "export function testWidget() {}\n",
		"widget.spec.tsx":      "export function specWidget() {}\n",
		"widget_test.ts":       "export function oldStyleTest() {}\n",
		"app.py":               "def main():\n    pass\n",
		"conftest.py":          "def helper():\n    pass\n",
		"test/index_test.ts":   "export function testWidget() {}\n",
		"tests/helper.go":      "package fixture\nfunc Helper(){}\n",
		"fixtures/helper.ts":   "export function fixture() {}\n",
		"testdata/helper.go":   "package fixture\nfunc Helper(){}\n",
		"__tests__/helper.tsx": "export function helper() {}\n",
	}
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, sources, err := Discover(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"main.go", "widget.ts", "app.py"} {
		if sources[want] == nil {
			t.Fatalf("code source %s missing: %v", want, sources)
		}
	}
	for _, excluded := range []string{"main_test.go", "widget.test.ts", "widget.spec.tsx", "widget_test.ts", "conftest.py", "test/index_test.ts", "tests/helper.go", "fixtures/helper.ts", "testdata/helper.go", "__tests__/helper.tsx"} {
		if sources[excluded] != nil {
			t.Fatalf("test source %s was captured", excluded)
		}
	}
}

func TestTreeFactsCaptureImports(t *testing.T) {
	source := []byte("package a\n\nimport (\n\t\"database/sql\"\n\t_ \"github.com/lib/pq\"\n)\n\nfunc Run() { _ = sql.ErrNoRows }\n")
	src := &graph.Source{Path: "a.go", Language: "go", Text: source, Hash: graph.Hash(source)}
	g := graph.NewGraph("repo", "snap")
	g.Sources[src.Path] = src
	if _, err := treeFacts(context.Background(), g, src); err != nil {
		t.Fatal(err)
	}
	if len(g.Facts) != 1 {
		t.Fatalf("facts: %d", len(g.Facts))
	}
	for _, fact := range g.Facts {
		got := strings.Join(fact.Imports, ",")
		if got != "database/sql,github.com/lib/pq" {
			t.Fatalf("imports: %q", got)
		}
	}
}

func TestImportedSCIPRejectsStaleSource(t *testing.T) {
	s := &graph.Source{Path: "a.go", Language: "go", Text: []byte("package a\n"), Hash: graph.Hash([]byte("package a\n"))}
	g := graph.NewGraph("r", "s")
	g.Sources[s.Path] = s
	idx := &scip.Index{Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "fixture"}}, Documents: []*scip.Document{{RelativePath: "a.go"}}}
	b, _ := proto.Marshal(idx)
	if e := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader(b), map[string]string{"a.go": "wrong"}, true, false, newSymbols()); e == nil {
		t.Fatal("stale manifest accepted")
	}
}
func TestMixedPipeline(t *testing.T) {
	for _, tool := range []string{"scip-go", "scip-typescript", "scip-python"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Skipf("%s unavailable", tool)
		}
	}
	root := "testdata/mixed"
	if _, e := os.Stat(filepath.Join(root, "ts/node_modules/typescript")); e != nil {
		t.Skip("run npm ci --prefix internal/indexer/testdata/mixed/ts")
	}
	snap, g, e := (Pipeline{Config: config.Default()}).Build(context.Background(), &pb.IndexRequest{Directory: root}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if snap.IngestionStatus != "complete" {
		t.Fatal(snap.IngestionStatus)
	}
	producers := map[string]bool{}
	for _, f := range g.Facts {
		for _, ev := range f.Evidence {
			producers[ev.Producer] = true
		}
	}
	for _, name := range []string{"scip", "tree-sitter"} {
		if !producers[name] {
			t.Fatalf("no %s evidence", name)
		}
	}
	calls := 0
	for _, e := range g.EdgeFacts {
		if e.Kind == pb.EdgeKind_EDGE_KIND_CALLS {
			calls++
		}
	}
	if calls == 0 {
		t.Fatal("no resolved calls")
	}
	languages := map[string]int{}
	for _, f := range g.Facts {
		languages[f.Language]++
	}
	for _, language := range []string{"go", "typescript", "javascript", "python"} {
		if languages[language] == 0 {
			t.Fatalf("language %s produced no facts: %v", language, languages)
		}
	}
}
func TestPythonRequiresSemanticIndexer(t *testing.T) {
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	source := "class Registry:\n    def add(self, name):\n        return render(name)\n\ndef render(name):\n    return str(name)\n"
	if e := os.WriteFile(filepath.Join(root, "service.py"), []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	cfg := config.Default()
	cfg.Tools.SCIPPython = "unavailable-scip-python"
	if _, _, e := (Pipeline{Config: cfg}).Build(context.Background(), &pb.IndexRequest{Directory: root}, nil); e == nil {
		t.Fatal("python indexing accepted without scip-python")
	}
}

func TestSourceDriftPreventsSnapshot(t *testing.T) {
	for _, tool := range []string{"scip-go"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Skipf("missing %s", tool)
		}
	}
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/drift\n\ngo 1.26\n"), 0600); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(root, "drift.go")
	if e := os.WriteFile(file, []byte("package drift\nfunc A(){}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	_, _, e := (Pipeline{Config: config.Default()}).Build(context.Background(), &pb.IndexRequest{Directory: root}, func(update Progress) {
		if update.Stage == "verify" {
			_ = os.WriteFile(file, []byte("package drift\nfunc B(){}\n"), 0600)
		}
	})
	if e == nil {
		t.Fatal("source drift accepted")
	}
}
func TestMissingToolAndMalformedSCIP(t *testing.T) {
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/missing\n\ngo 1.26\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "a.go"), []byte("package missing\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cfg := config.Default()
	cfg.Tools.SCIPGo = "unavailable-scip-go"
	if _, _, e := (Pipeline{Config: cfg}).Build(context.Background(), &pb.IndexRequest{Directory: root}, nil); e == nil {
		t.Fatal("missing tool accepted")
	}
	g := graph.NewGraph("r", "s")
	if e := importSCIPReader(context.Background(), g, &pb.Project{Root: "."}, bytes.NewReader([]byte("bad protobuf")), nil, true, false, newSymbols()); e == nil {
		t.Fatal("malformed SCIP accepted")
	}
}
func TestPrebuiltSCIPManifest(t *testing.T) {
	for _, tool := range []string{"scip-go"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Skipf("missing %s", tool)
		}
	}
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/prebuilt\n\ngo 1.26\n"), 0600); e != nil {
		t.Fatal(e)
	}
	source := []byte("package prebuilt\nfunc Welcome() string { return \"hi\" }\n")
	if e := os.WriteFile(filepath.Join(root, "a.go"), source, 0600); e != nil {
		t.Fatal(e)
	}
	artifact := filepath.Join(t.TempDir(), "index.scip")
	cmd := exec.Command("scip-go", "--output", artifact, "--project-root", root, "--module-root", root, "--repository-root", root)
	cmd.Dir = root
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("scip-go %v %s", e, output)
	}
	request := &pb.IndexRequest{Directory: root, ScipArtifacts: map[string]string{".": artifact}}
	pipeline := Pipeline{Config: config.Default()}
	if _, _, e := pipeline.Build(context.Background(), request, nil); e == nil {
		t.Fatal("unverified prebuilt SCIP accepted")
	}
	manifest, _ := json.Marshal(map[string]string{"a.go": graph.Hash(source)})
	if e := os.WriteFile(artifact+".manifest.json", manifest, 0600); e != nil {
		t.Fatal(e)
	}
	snap, g, e := pipeline.Build(context.Background(), request, nil)
	if e != nil {
		t.Fatal(e)
	}
	if snap.IngestionStatus != "complete" || len(g.Facts) == 0 {
		t.Fatal("prebuilt SCIP produced no facts")
	}
}
