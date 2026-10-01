package indexer

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/config"
)

func TestSourceLanguageAndFamily(t *testing.T) {
	cases := map[string]string{
		"a.go": "go", "a.ts": "typescript", "a.tsx": "tsx", "a.js": "javascript", "a.py": "python",
		"A.cs": "csharp", "A.vb": "visualbasic", "A.c": "c", "A.cpp": "cpp", "A.hpp": "cpp",
		"A.dart": "dart", "A.java": "java", "A.scala": "scala", "A.kt": "kotlin", "A.php": "php",
		"A.rb": "ruby", "A.rs": "rust",
	}
	for path, want := range cases {
		if got := sourceLanguage(path); got != want {
			t.Fatalf("sourceLanguage(%s) = %q, want %q", path, got, want)
		}
	}
	families := map[string]string{
		"go": familyGo, "typescript": familyWeb, "javascript": familyWeb, "python": familyPython,
		"csharp": familyDotnet, "visualbasic": familyDotnet, "c": familyClang, "cpp": familyClang,
		"java": familyJVM, "scala": familyJVM, "kotlin": familyJVM, "dart": familyDart,
		"php": familyPHP, "ruby": familyRuby, "rust": familyRust,
	}
	for language, want := range families {
		if got := languageFamily(language); got != want {
			t.Fatalf("languageFamily(%s) = %q, want %q", language, got, want)
		}
	}
	for _, syntaxFamily := range []string{familyGo, familyWeb, familyPython} {
		if !isSyntaxFamily(syntaxFamily) {
			t.Fatalf("%s should be tree-sitter backed", syntaxFamily)
		}
	}
	for _, scipFamily := range []string{familyDotnet, familyClang, familyJVM, familyDart, familyPHP, familyRuby, familyRust} {
		if isSyntaxFamily(scipFamily) {
			t.Fatalf("%s should be SCIP backed", scipFamily)
		}
	}
}

func TestDiscoverFindsNewLanguageFamilies(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"dotnet/App.csproj":  "<Project/>",
		"dotnet/Program.cs":  "class Program { static void Main() {} }\n",
		"vb/Library.vbproj":  "<Project/>",
		"vb/Module1.vb":      "Module Module1\nEnd Module\n",
		"cpp/CMakeLists.txt": "project(fixture)\n",
		"cpp/main.cpp":       "int main() { return 0; }\n",
		"dart/pubspec.yaml":  "name: fixture\n",
		"dart/main.dart":     "void main() {}\n",
		"jvm/pom.xml":        "<project/>",
		"jvm/Main.java":      "class Main {}\n",
		"jvm/App.kt":         "fun main() {}\n",
		"php/composer.json":  "{}",
		"php/index.php":      "<?php function run() {}\n",
		"ruby/Gemfile":       "source 'https://rubygems.org'\n",
		"ruby/app.rb":        "def run\nend\n",
		"rust/Cargo.toml":    "[package]\nname = \"fixture\"\n",
		"rust/lib.rs":        "pub fn run() {}\n",
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
	projects, sources, err := Discover(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantLanguages := map[string]string{
		"dotnet/Program.cs": "csharp", "vb/Module1.vb": "visualbasic",
		"cpp/main.cpp": "cpp", "dart/main.dart": "dart",
		"jvm/Main.java": "java", "jvm/App.kt": "kotlin",
		"php/index.php": "php", "ruby/app.rb": "ruby", "rust/lib.rs": "rust",
	}
	for path, language := range wantLanguages {
		if sources[path] == nil || sources[path].Language != language {
			t.Fatalf("source %s: got %+v, want language %s", path, sources[path], language)
		}
	}
	families := map[string]int{}
	for _, p := range projects {
		families[languageFamily(p.Language)]++
	}
	// Java and Kotlin share one jvm root and collapse to a single indexer
	// invocation; the two .NET project roots stay separate.
	wantCounts := map[string]int{
		familyDotnet: 2, familyClang: 1, familyJVM: 1, familyDart: 1,
		familyPHP: 1, familyRuby: 1, familyRust: 1,
	}
	for family, want := range wantCounts {
		if families[family] != want {
			t.Fatalf("family %s projects: %d, want %d (all: %v)", family, families[family], want, families)
		}
	}
}

func TestIndexerSpecs(t *testing.T) {
	cfg := config.Default()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	c := indexerContext{root: "/repo", projectDir: dir, artifact: "/tmp/x.scip", name: "proj", configPath: "proj.csproj"}
	families := []string{familyGo, familyWeb, familyPython, familyDotnet, familyClang, familyJVM, familyDart, familyPHP, familyRuby, familyRust}
	for _, family := range families {
		spec, err := indexerForFamily(family, cfg)
		if err != nil {
			t.Fatalf("%s: %v", family, err)
		}
		if spec.name == "" || spec.executable(cfg, c) == "" {
			t.Fatalf("%s: incomplete spec", family)
		}
		argv, explicit, err := spec.args(cfg, c)
		if err != nil {
			t.Fatalf("%s args: %v", family, err)
		}
		if explicit {
			found := false
			for _, a := range argv {
				if a == c.artifact {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s declared explicit output but argv lacks artifact: %v", family, argv)
			}
		}
		if family == familyClang {
			joined := strings.Join(argv, " ")
			if !strings.Contains(joined, "compile_commands.json") {
				t.Fatalf("clang argv lacks compilation database: %v", argv)
			}
		}
	}
	clang, _ := indexerForFamily(familyClang, cfg)
	if _, _, err := clang.args(cfg, indexerContext{projectDir: t.TempDir(), artifact: "x"}); err == nil {
		t.Fatal("clang accepted a project without compile_commands.json")
	}
}

func TestWebIndexerInfersTsconfig(t *testing.T) {
	cfg := config.Default()
	spec, err := indexerForFamily(familyWeb, cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := indexerContext{projectDir: dir, artifact: "/tmp/out.scip"}
	argv, _, err := spec.args(cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(argv, "--infer-tsconfig") {
		t.Fatalf("javascript project without tsconfig should infer one: %v", argv)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	argv, _, err = spec.args(cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(argv, "--infer-tsconfig") {
		t.Fatalf("tsconfig project should not infer: %v", argv)
	}
}

func TestNestedRoot(t *testing.T) {
	if !nestedRoot("a/b/c", []string{"a", "a/b"}) {
		t.Fatal("descendant root not detected")
	}
	if nestedRoot("ab", []string{"a"}) {
		t.Fatal("sibling prefix misdetected as nested")
	}
	if !nestedRoot("plugin", []string{"."}) {
		t.Fatal("repository root should contain every other root")
	}
	if nestedRoot(".", []string{"plugin"}) {
		t.Fatal("repository root must not be nested")
	}
}
