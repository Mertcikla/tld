package indexer

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
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

func TestWebIndexerPassesAllTsconfigs(t *testing.T) {
	cfg := config.Default()
	spec, err := indexerForFamily(familyWeb, cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"tsconfig.json", "tsconfig.lib.json", "tsconfig.node.json", "unrelated.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c := indexerContext{projectDir: dir, artifact: "/tmp/out.scip", configPath: "frontend/tsconfig.json"}
	argv, _, err := spec.args(cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(argv, "--infer-tsconfig") {
		t.Fatalf("tsconfig project should not infer: %v", argv)
	}
	for _, want := range []string{"tsconfig.json", "tsconfig.lib.json", "tsconfig.node.json"} {
		if !slices.Contains(argv, want) {
			t.Fatalf("argv %v lacks %s", argv, want)
		}
	}
	if slices.Contains(argv, "unrelated.json") {
		t.Fatalf("argv %v contains a non-tsconfig file", argv)
	}

	fallback, _, err := spec.fallbackArgs(cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(fallback, "tsconfig.json") || slices.Contains(fallback, "tsconfig.lib.json") {
		t.Fatalf("fallback should use the primary config only: %v", fallback)
	}

	single := t.TempDir()
	if err := os.WriteFile(filepath.Join(single, "tsconfig.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if argv, ok, err := spec.fallbackArgs(cfg, indexerContext{projectDir: single, configPath: "tsconfig.json"}); err != nil || ok || len(argv) != 0 {
		t.Fatalf("single-config project needs no fallback: argv=%v ok=%v err=%v", argv, ok, err)
	}
}

func TestSymbolInputsTracksWebConfigs(t *testing.T) {
	root := t.TempDir()
	frontend := filepath.Join(root, "frontend")
	if err := os.MkdirAll(filepath.Join(frontend, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontend, "tsconfig.json"), []byte(`{"include":["src/main.tsx"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	libConfig := filepath.Join(frontend, "tsconfig.lib.json")
	if err := os.WriteFile(libConfig, []byte(`{"include":["src"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	projects := []*pb.Project{{Root: "frontend", Language: "typescript", ConfigPath: "frontend/tsconfig.json"}}
	sources := map[string]*graph.Source{"frontend/src/a.ts": {Path: "frontend/src/a.ts", Hash: "h1"}}

	first, err := symbolInputs(root, projects, sources, "cfg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libConfig, []byte(`{"include":["src","other"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := symbolInputs(root, projects, sources, "cfg")
	if err != nil {
		t.Fatal(err)
	}
	if first["web|frontend"] == second["web|frontend"] {
		t.Fatal("changing tsconfig.lib.json did not invalidate the project fingerprint")
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

func TestDiscoverProjectsMatchesFullDiscovery(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                 "module example.com/demo\n",
		"main.go":                "package main\n",
		"frontend/tsconfig.json": "{}",
		"frontend/src/app.ts":    "export const app = 1\n",
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
	light, err := DiscoverProjects(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(light) != len(projects) {
		t.Fatalf("DiscoverProjects returned %d projects, full discovery %d", len(light), len(projects))
	}
	for i, project := range light {
		if project.GetConfigPath() != projects[i].GetConfigPath() || project.GetLanguage() != projects[i].GetLanguage() {
			t.Fatalf("project %d = %+v, want %+v", i, project, projects[i])
		}
	}
	families := map[string]bool{}
	for _, project := range light {
		families[Family(project.GetLanguage())] = true
	}
	if !families[familyGo] || !families[familyWeb] {
		t.Fatalf("families = %v, want go and web", families)
	}
	if len(sources) == 0 {
		t.Fatal("full discovery should still collect sources")
	}
}

// TestDiscoverSetupPyOnlyProject ensures a legacy Python repository whose only
// project marker is setup.py still registers a project (setup.py must not be
// filtered out as a test source first).
func TestDiscoverSetupPyOnlyProject(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"setup.py":         "from setuptools import setup\nsetup(name='demo')\n",
		"demo/__init__.py": "def helper():\n    return 1\n",
		"demo/main.py":     "from demo import helper\n\ndef run():\n    return helper()\n",
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
	if len(projects) != 1 {
		t.Fatalf("projects = %+v, want 1 (setup.py)", projects)
	}
	if Family(projects[0].GetLanguage()) != familyPython || projects[0].GetConfigPath() != "setup.py" {
		t.Fatalf("project = %+v, want python via setup.py", projects[0])
	}
	if sources["demo/main.py"] == nil {
		t.Fatalf("sources = %v, want demo/main.py captured", sources)
	}
}

func TestDiscoverDotnetSolutionIndexesEachProject(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"App.slnx":        "<Solution></Solution>\n",
		"src/A/A.csproj":  "<Project/>\n",
		"src/A/AClass.cs": "namespace A { class AClass {} }\n",
		"src/B/B.csproj":  "<Project/>\n",
		"src/B/BClass.cs": "namespace B { class BClass {} }\n",
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
	projects, _, err := Discover(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The .slnx is ignored; each .csproj becomes its own project root so a
	// single broken or unsupported project cannot hide the others.
	if len(projects) != 2 {
		t.Fatalf("projects = %+v, want 2 csproj roots", projects)
	}
	roots := map[string]bool{}
	for _, project := range projects {
		if Family(project.GetLanguage()) != familyDotnet {
			t.Fatalf("project %+v is not dotnet", project)
		}
		roots[project.GetRoot()] = true
	}
	if !roots["src/A"] || !roots["src/B"] {
		t.Fatalf("roots = %v, want src/A and src/B", roots)
	}
}

func TestSynthesizeJSTsConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	js := filepath.Join(dir, "lib.js")
	ts := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(js, []byte("const x = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ts, []byte("export const y = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := indexerContext{projectDir: dir, artifact: filepath.Join(t.TempDir(), "0.scip"), sourceFiles: []string{js, ts}}

	path := synthesizeJSTsConfig(c, []string{"tsconfig.json"})
	if path == "" {
		t.Fatal("empty tsconfig with JS sources should synthesize a config")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"allowJs":true`) {
		t.Fatalf("synthesized config = %s, want allowJs", raw)
	}

	if err := os.WriteFile(filepath.Join(dir, "tsconfig.lib.json"), []byte(`{"include":["src"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := synthesizeJSTsConfig(c, []string{"tsconfig.lib.json"}); got != "" {
		t.Fatalf("explicit include should not synthesize, got %q", got)
	}

	c.sourceFiles = []string{ts}
	if got := synthesizeJSTsConfig(c, []string{"tsconfig.json"}); got != "" {
		t.Fatalf("project without JS sources should not synthesize, got %q", got)
	}
}

func TestDiscoverProjectsAllowsRepositoriesWithoutProjects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	projects, err := DiscoverProjects(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("projects = %+v, want none", projects)
	}
}
