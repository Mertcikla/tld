package indexer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/codeindex/config"
)

// Indexer families group the languages that a single SCIP indexer invocation
// covers. Tree-sitter languages form their own families and are enriched, not
// replaced, by SCIP. Every other family is SCIP-backed: code Facts, references
// and calls are produced directly from the SCIP index.
const (
	familyGo     = "go"
	familyWeb    = "web"
	familyPython = "python"
	familyDotnet = "dotnet"
	familyClang  = "clang"
	familyJVM    = "jvm"
	familyDart   = "dart"
	familyPHP    = "php"
	familyRuby   = "ruby"
	familyRust   = "rust"
)

// languageFamily returns the indexer family for a source language.
func languageFamily(language string) string {
	switch language {
	case "go":
		return familyGo
	case "typescript", "tsx", "javascript", "jsx":
		return familyWeb
	case "python":
		return familyPython
	case "csharp", "visualbasic":
		return familyDotnet
	case "c", "cpp":
		return familyClang
	case "java", "scala", "kotlin":
		return familyJVM
	case "dart":
		return familyDart
	case "php":
		return familyPHP
	case "ruby":
		return familyRuby
	case "rust":
		return familyRust
	}
	return language
}

// isSyntaxFamily reports whether a family's Facts come from the tree-sitter
// declaration walker. SCIP enriches those Facts with symbol metadata and
// references; it does not synthesize them.
func isSyntaxFamily(family string) bool {
	switch family {
	case familyGo, familyWeb, familyPython:
		return true
	}
	return false
}

// sourceLanguage returns the language id for a file extension, or "" when the
// extension is not indexed.
func sourceLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".cs":
		return "csharp"
	case ".vb":
		return "visualbasic"
	case ".c":
		return "c"
	case ".h", ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx":
		return "cpp"
	case ".dart":
		return "dart"
	case ".java":
		return "java"
	case ".scala":
		return "scala"
	case ".kt", ".kts":
		return "kotlin"
	case ".php":
		return "php"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	}
	return ""
}

// projectLanguage returns the language implied by a project marker file, or ""
// when the file is not a recognized project marker.
func projectLanguage(name string) string {
	switch name {
	case "go.mod", "go.work":
		return "go"
	case "tsconfig.json":
		return "typescript"
	case "package.json":
		return "javascript"
	case "pyproject.toml", "setup.py", "setup.cfg":
		return "python"
	case "composer.json":
		return "php"
	case "Cargo.toml":
		return "rust"
	case "pubspec.yaml":
		return "dart"
	case "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "build.sbt":
		return "java"
	case "Gemfile":
		return "ruby"
	case "CMakeLists.txt", "meson.build", "compile_commands.json":
		return "cpp"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sln", ".slnx", ".csproj":
		return "csharp"
	case ".vbproj":
		return "visualbasic"
	case ".gemspec":
		return "ruby"
	}
	return ""
}

// indexerContext carries the paths a family needs to configure its indexer.
type indexerContext struct {
	root       string
	projectDir string
	artifact   string
	name       string
	configPath string
}

// indexerSpec describes how to invoke one family's SCIP indexer.
type indexerSpec struct {
	name        string
	versionArgs []string
	// executable resolves the tool binary, which may depend on the project
	// layout (for example a project-local PHP vendor binary).
	executable func(cfg config.Config, c indexerContext) string
	// args builds the indexer argv. explicitOutput reports whether argv already
	// contains the artifact path; when false the tool writes index.scip into
	// the working directory and the caller resolves it there.
	args func(cfg config.Config, c indexerContext) (argv []string, explicitOutput bool, err error)
	// fallbackArgs, when set, rebuilds a narrower argv the caller retries with
	// after args fails, so an optional extra project cannot break indexing.
	fallbackArgs func(cfg config.Config, c indexerContext) (argv []string, explicitOutput bool, err error)
}

// indexerForFamily returns the indexer spec for an indexer family.
func indexerForFamily(family string, cfg config.Config) (indexerSpec, error) {
	switch family {
	case familyGo:
		return indexerSpec{
			name:        "scip-go",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPGo },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				return []string{"--output", c.artifact, "--project-root", c.projectDir, "--module-root", c.projectDir, "--repository-root", c.root}, true, nil
			},
		}, nil
	case familyWeb:
		return indexerSpec{
			name:        "scip-typescript",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPTypeScript },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				return webIndexArgs(c, webProjectConfigs(c.projectDir)), true, nil
			},
			fallbackArgs: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				configs := webProjectConfigs(c.projectDir)
				if len(configs) <= 1 {
					return nil, false, nil
				}
				primary := filepath.Base(c.configPath)
				if !strings.HasPrefix(primary, "tsconfig") || !strings.HasSuffix(primary, ".json") {
					primary = "tsconfig.json"
				}
				return webIndexArgs(c, []string{primary}), true, nil
			},
		}, nil
	case familyPython:
		return indexerSpec{
			name:        "scip-python",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPPython },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				return []string{"index", ".", "--project-name", c.name, "--output", c.artifact}, true, nil
			},
		}, nil
	case familyDotnet:
		return indexerSpec{
			name:        "scip-dotnet",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPDotnet },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				argv := []string{"index"}
				if base := filepath.Base(c.configPath); base != "." && base != "/" && base != "" {
					argv = append(argv, base)
				}
				argv = append(argv, "--working-directory", c.projectDir, "--output", c.artifact)
				return argv, true, nil
			},
		}, nil
	case familyClang:
		return indexerSpec{
			name:        "scip-clang",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPClang },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				compdb, err := findCompilationDatabase(c.projectDir)
				if err != nil {
					return nil, false, err
				}
				return []string{"--compdb-path=" + compdb}, false, nil
			},
		}, nil
	case familyJVM:
		return indexerSpec{
			name:        "scip-java",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPJava },
			args: func(_ config.Config, _ indexerContext) ([]string, bool, error) {
				return []string{"index"}, false, nil
			},
		}, nil
	case familyDart:
		return indexerSpec{
			name:        "scip-dart",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPDart },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				return []string{".", "--output", c.artifact}, true, nil
			},
		}, nil
	case familyPHP:
		return indexerSpec{
			name:        "scip-php",
			versionArgs: []string{"--help"},
			executable: func(cfg config.Config, c indexerContext) string {
				local := filepath.Join(c.projectDir, "vendor", "bin", "scip-php")
				if _, err := os.Stat(local); err == nil {
					return local
				}
				return cfg.Tools.SCIPPhp
			},
			args: func(_ config.Config, _ indexerContext) ([]string, bool, error) {
				return nil, false, nil
			},
		}, nil
	case familyRuby:
		return indexerSpec{
			name:        "scip-ruby",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.SCIPRuby },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				return []string{".", "--gem-metadata", c.name + "@0.0.0", "--index-file", c.artifact}, true, nil
			},
		}, nil
	case familyRust:
		return indexerSpec{
			name:        "rust-analyzer",
			versionArgs: []string{"--version"},
			executable:  func(cfg config.Config, _ indexerContext) string { return cfg.Tools.RustAnalyzer },
			args: func(_ config.Config, c indexerContext) ([]string, bool, error) {
				return []string{"scip", ".", "--output", c.artifact}, true, nil
			},
		}, nil
	}
	return indexerSpec{}, fmt.Errorf("no SCIP indexer configured for project family %q", family)
}

// webIndexArgs builds a scip-typescript invocation. Passing every sibling
// tsconfig unions file coverage: a project's primary tsconfig may deliberately
// include only entry points and .d.ts files, with the application sources
// covered by a second config such as tsconfig.lib.json.
func webIndexArgs(c indexerContext, configs []string) []string {
	argv := []string{"index"}
	if len(configs) == 0 {
		argv = append(argv, "--infer-tsconfig")
	} else {
		argv = append(argv, configs...)
	}
	return append(argv, "--cwd", c.projectDir, "--output", c.artifact)
}

// webProjectConfigs lists the top-level tsconfig files in a project directory,
// sorted so indexer invocations and cache fingerprints stay deterministic.
func webProjectConfigs(projectDir string) []string {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}
	configs := make([]string, 0, 4)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "tsconfig") && strings.HasSuffix(name, ".json") {
			configs = append(configs, name)
		}
	}
	sort.Strings(configs)
	return configs
}

// findCompilationDatabase locates a clang JSON compilation database for a
// project, preferring the project root and then common build directories.
func findCompilationDatabase(projectDir string) (string, error) {
	candidates := []string{
		filepath.Join(projectDir, "compile_commands.json"),
		filepath.Join(projectDir, "build", "compile_commands.json"),
		filepath.Join(projectDir, "out", "compile_commands.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	var found string
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "compile_commands.json" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	return "", fmt.Errorf("scip-clang requires a compile_commands.json in %s", projectDir)
}
