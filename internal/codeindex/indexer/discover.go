package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/parser"
)

func Discover(ctx context.Context, root string, overrides, excludes []string) ([]*pb.Project, map[string]*graph.Source, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", root)
	}
	var projects []*pb.Project
	sources := map[string]*graph.Source{}
	ignore := func(rel string) bool {
		c := exec.CommandContext(ctx, "git", "check-ignore", "-q", "--", rel)
		c.Dir = root
		return c.Run() == nil
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if parser.SkipDirs[d.Name()] || excluded(rel, excludes) || ignore(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if excluded(rel, excludes) || ignore(rel) || isTestSource(rel) {
			return nil
		}
		if lang := projectLanguage(d.Name()); lang != "" {
			projects = append(projects, &pb.Project{Root: filepath.ToSlash(filepath.Dir(rel)), Language: lang, ConfigPath: rel})
		}
		lang := sourceLanguage(rel)
		if lang == "" {
			if !parser.IsInfraSource(d.Name(), rel) {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !utf8.Valid(b) || len(b) > 2<<20 {
				return nil
			}
			sources[rel] = &graph.Source{Path: rel, Language: "", Text: b, Hash: graph.Hash(b)}
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Files that cannot be represented as UTF-8 or that exceed the size
		// budget are skipped rather than failing the whole repository: a single
		// stray binary asset should not block indexing.
		if !utf8.Valid(b) || len(b) > 2<<20 {
			return nil
		}
		sources[rel] = &graph.Source{Path: rel, Language: lang, Text: b, Hash: graph.Hash(b)}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	for _, override := range overrides {
		p := filepath.Join(root, override)
		config := ""
		lang := ""
		for _, name := range []string{"go.mod", "go.work", "tsconfig.json", "package.json", "pyproject.toml",
			"setup.py", "composer.json", "Cargo.toml", "pubspec.yaml", "pom.xml", "build.gradle",
			"build.gradle.kts", "settings.gradle", "settings.gradle.kts", "build.sbt", "Gemfile",
			"CMakeLists.txt", "compile_commands.json"} {
			if _, err := os.Stat(filepath.Join(p, name)); err == nil {
				lang = projectLanguage(name)
				config = filepath.ToSlash(filepath.Join(override, name))
				break
			}
		}
		if lang == "" {
			return nil, nil, fmt.Errorf("project root %s lacks a recognized project file", override)
		}
		projects = append(projects, &pb.Project{Root: filepath.ToSlash(override), Language: lang, ConfigPath: config})
	}
	// One project per (indexer family, root). A TypeScript project supersedes a
	// bare JavaScript one at the same root; all other families collapse to a
	// single indexer invocation for that root.
	best := map[string]*pb.Project{}
	for _, p := range projects {
		key := languageFamily(p.Language) + ":" + p.Root
		existing := best[key]
		if existing == nil || (existing.Language == "javascript" && p.Language == "typescript") {
			best[key] = p
		}
	}
	projects = projects[:0]
	for _, p := range best {
		projects = append(projects, p)
	}
	// Whole-workspace indexers (a Gradle/Maven build, a .NET solution, a C/C++
	// compilation database, a Rust workspace) already cover nested modules, so
	// a project root nested inside another same-family root is not indexed
	// separately.
	workspaceFamilies := map[string]bool{familyJVM: true, familyDotnet: true, familyClang: true, familyRust: true}
	roots := map[string][]string{}
	for _, p := range projects {
		if workspaceFamilies[languageFamily(p.Language)] {
			roots[languageFamily(p.Language)] = append(roots[languageFamily(p.Language)], p.Root)
		}
	}
	kept := projects[:0]
	for _, p := range projects {
		family := languageFamily(p.Language)
		if !workspaceFamilies[family] || !nestedRoot(p.Root, roots[family]) {
			kept = append(kept, p)
		}
	}
	projects = kept
	sort.Slice(projects, func(i, j int) bool { return projects[i].ConfigPath < projects[j].ConfigPath })
	if len(projects) == 0 {
		if len(sources) == 0 {
			return nil, nil, fmt.Errorf("no supported project found (Go, TypeScript/JavaScript, Python, C/C++, C#/Visual Basic, Dart, Java/Scala/Kotlin, PHP, Ruby, or Rust)")
		}
		// A repository with configuration or sources but no indexable project
		// still publishes its infra facts and sources; code declarations are
		// simply absent from the snapshot.
		return projects, sources, nil
	}
	return projects, sources, nil
}

// nestedRoot reports whether root is strictly contained by another root in
// roots. The repository root is represented by ".".
func nestedRoot(root string, roots []string) bool {
	for _, other := range roots {
		if other == root {
			continue
		}
		if other == "." || other == "" || strings.HasPrefix(root, strings.TrimSuffix(other, "/")+"/") {
			return true
		}
	}
	return false
}

var pythonTestNames = map[string]bool{
	"conftest.py": true, "setup.py": true, "noop.py": true,
}

func isTestSource(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "test" || part == "tests" || part == "testdata" || part == "__tests__" || part == "fixtures" {
			return true
		}
	}
	name := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_test.ts") || strings.HasSuffix(name, "_test.tsx") {
		return true
	}
	for _, suffix := range []string{".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx",
		".test.js", ".spec.js", ".test.jsx", ".spec.jsx"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	if pythonTestNames[name] {
		return true
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test")
}
func excluded(rel string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.Trim(p, "/")
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if matched, _ := filepath.Match(p, rel); matched {
			return true
		}
	}
	return false
}
