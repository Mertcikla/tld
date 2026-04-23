package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNewService_ExtractPath_UsesGoTreeSitter(t *testing.T) {
	dir := t.TempDir()
	fooPath := filepath.Join(dir, "foo.go")
	barPath := filepath.Join(dir, "bar.go")
	writeAnalyzerTestFile(t, fooPath, "package main\nfunc Foo() {}\n")
	writeAnalyzerTestFile(t, barPath, "package main\nfunc Bar() { Foo() }\n")

	result, err := NewService().ExtractPath(context.Background(), dir, nil, nil)
	if err != nil {
		t.Fatalf("ExtractPath: %v", err)
	}
	if len(result.Symbols) != 2 {
		t.Fatalf("symbols = %d, want 2: %+v", len(result.Symbols), result.Symbols)
	}
	if len(result.Refs) != 1 {
		t.Fatalf("refs = %d, want 1: %+v", len(result.Refs), result.Refs)
	}

	symbolsByName := make(map[string]Symbol, len(result.Symbols))
	for _, sym := range result.Symbols {
		symbolsByName[sym.Name] = sym
	}
	if got := symbolsByName["Foo"].Kind; got != "function" {
		t.Fatalf("Foo kind = %q", got)
	}
	if got := symbolsByName["Bar"].Kind; got != "function" {
		t.Fatalf("Bar kind = %q", got)
	}
	if filepath.Base(symbolsByName["Foo"].FilePath) != "foo.go" {
		t.Fatalf("Foo file path = %q", symbolsByName["Foo"].FilePath)
	}
	if got := result.Refs[0].Name; got != "Foo" {
		t.Fatalf("ref name = %q", got)
	}
	if filepath.Base(result.Refs[0].FilePath) != "bar.go" {
		t.Fatalf("ref file path = %q", result.Refs[0].FilePath)
	}
	if got := result.Refs[0].Kind; got != "call" {
		t.Fatalf("ref kind = %q", got)
	}
}

func TestNewService_ExtractPath_CollectsGoImports(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	writeAnalyzerTestFile(t, filePath, "package main\n\nimport \"github.com/example/demo/internal/service\"\n\nfunc main() {\n    service.Run()\n}\n")

	result, err := NewService().ExtractPath(context.Background(), filePath, nil, nil)
	if err != nil {
		t.Fatalf("ExtractPath go imports: %v", err)
	}
	for _, ref := range result.Refs {
		if ref.Kind != "import" {
			continue
		}
		if ref.Name != "service" {
			t.Fatalf("import ref name = %q, want service", ref.Name)
		}
		if ref.TargetPath != "github.com/example/demo/internal/service" {
			t.Fatalf("import ref target_path = %q", ref.TargetPath)
		}
		return
	}
	t.Fatalf("expected import ref, got %+v", result.Refs)
}

func TestNewService_HasSymbol_UsesGoTreeSitter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "service.go")
	writeAnalyzerTestFile(t, filePath, "package main\ntype Service struct{}\n")

	found, err := NewService().HasSymbol(context.Background(), filePath, "Service")
	if err != nil {
		t.Fatalf("HasSymbol: %v", err)
	}
	if !found {
		t.Fatal("expected Service symbol to be found")
	}

	notFound, err := NewService().HasSymbol(context.Background(), filePath, "Missing")
	if err != nil {
		t.Fatalf("HasSymbol missing: %v", err)
	}
	if notFound {
		t.Fatal("did not expect Missing symbol to be found")
	}
}

func TestNewService_ExtractPath_UsesPythonTreeSitter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "service.py")
	writeAnalyzerTestFile(t, filePath, "class Service:\n    pass\n\ndef get_columns():\n    helper()\n    client.fetch()\n")

	result, err := NewService().ExtractPath(context.Background(), filePath, nil, nil)
	if err != nil {
		t.Fatalf("ExtractPath python: %v", err)
	}
	if len(result.Symbols) != 2 {
		t.Fatalf("symbols = %d, want 2: %+v", len(result.Symbols), result.Symbols)
	}
	if len(result.Refs) != 2 {
		t.Fatalf("refs = %d, want 2: %+v", len(result.Refs), result.Refs)
	}
	symbolKinds := make(map[string]string, len(result.Symbols))
	for _, sym := range result.Symbols {
		symbolKinds[sym.Name] = sym.Kind
	}
	if symbolKinds["Service"] != "class" {
		t.Fatalf("Service kind = %q", symbolKinds["Service"])
	}
	if symbolKinds["get_columns"] != "function" {
		t.Fatalf("get_columns kind = %q", symbolKinds["get_columns"])
	}
	if result.Refs[0].Name != "helper" || result.Refs[1].Name != "fetch" {
		t.Fatalf("unexpected refs: %+v", result.Refs)
	}
}

func TestNewService_ExtractPath_UsesJavaTreeSitter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "Service.java")
	writeAnalyzerTestFile(t, filePath, "class Service {\n    void handle() {\n        helper();\n        client.fetch();\n    }\n\n    void helper() {}\n}\n")

	result, err := NewService().ExtractPath(context.Background(), filePath, nil, nil)
	if err != nil {
		t.Fatalf("ExtractPath java: %v", err)
	}
	if len(result.Symbols) != 3 {
		t.Fatalf("symbols = %d, want 3: %+v", len(result.Symbols), result.Symbols)
	}
	if len(result.Refs) != 2 {
		t.Fatalf("refs = %d, want 2: %+v", len(result.Refs), result.Refs)
	}
	symbolKinds := make(map[string]string, len(result.Symbols))
	parents := make(map[string]string, len(result.Symbols))
	for _, sym := range result.Symbols {
		symbolKinds[sym.Name] = sym.Kind
		parents[sym.Name] = sym.Parent
	}
	if symbolKinds["Service"] != "class" {
		t.Fatalf("Service kind = %q", symbolKinds["Service"])
	}
	if symbolKinds["handle"] != "method" || parents["handle"] != "Service" {
		t.Fatalf("handle = kind %q parent %q", symbolKinds["handle"], parents["handle"])
	}
	if symbolKinds["helper"] != "method" || parents["helper"] != "Service" {
		t.Fatalf("helper = kind %q parent %q", symbolKinds["helper"], parents["helper"])
	}
	if result.Refs[0].Name != "helper" || result.Refs[1].Name != "fetch" {
		t.Fatalf("unexpected refs: %+v", result.Refs)
	}
}

func TestNewService_ExtractPath_UsesCPPTreeSitter(t *testing.T) {
	dir := t.TempDir()
	headerPath := filepath.Join(dir, "service.hpp")
	sourcePath := filepath.Join(dir, "service.cpp")
	writeAnalyzerTestFile(t, headerPath, "class Service {\npublic:\n    void handle();\n};\n")
	writeAnalyzerTestFile(t, sourcePath, "#include \"service.hpp\"\n\nvoid helper() {}\n\nvoid Service::handle() {\n    helper();\n}\n\nint main() {\n    Service service;\n    service.handle();\n    return 0;\n}\n")

	result, err := NewService().ExtractPath(context.Background(), dir, nil, nil)
	if err != nil {
		t.Fatalf("ExtractPath cpp: %v", err)
	}
	if len(result.Symbols) < 4 {
		t.Fatalf("symbols = %d, want at least 4: %+v", len(result.Symbols), result.Symbols)
	}
	if len(result.Refs) < 2 {
		t.Fatalf("refs = %d, want at least 2: %+v", len(result.Refs), result.Refs)
	}
	symbolKinds := make(map[string]string, len(result.Symbols))
	parents := make(map[string]string, len(result.Symbols))
	for _, sym := range result.Symbols {
		symbolKinds[sym.Name] = sym.Kind
		if sym.Parent != "" {
			parents[sym.Name] = sym.Parent
		}
	}
	if symbolKinds["Service"] != "class" {
		t.Fatalf("Service kind = %q", symbolKinds["Service"])
	}
	if symbolKinds["helper"] != "function" {
		t.Fatalf("helper kind = %q", symbolKinds["helper"])
	}
	if kind := symbolKinds["handle"]; kind != "method" {
		t.Fatalf("handle kind = %q", kind)
	}
	if parents["handle"] != "Service" {
		t.Fatalf("handle parent = %q", parents["handle"])
	}
	if symbolKinds["main"] != "function" {
		t.Fatalf("main kind = %q", symbolKinds["main"])
	}
	refNames := make([]string, 0, len(result.Refs))
	for _, ref := range result.Refs {
		refNames = append(refNames, ref.Name)
	}
	if !containsString(refNames, "helper") || !containsString(refNames, "handle") {
		t.Fatalf("unexpected refs: %+v", result.Refs)
	}
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func writeAnalyzerTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
