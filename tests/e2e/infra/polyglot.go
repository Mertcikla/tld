package infra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type PolyglotActor struct {
	RepoPath string
}

func NewPolyglotActor(repoPath string) *PolyglotActor {
	return &PolyglotActor{RepoPath: repoPath}
}

func (a *PolyglotActor) Init() error {
	if err := os.MkdirAll(a.RepoPath, 0o755); err != nil {
		return err
	}
	return a.runGit("init")
}

func (a *PolyglotActor) Commit(msg string) error {
	if err := a.runGit("add", "."); err != nil {
		return err
	}
	// Use --allow-empty in case nothing changed but we want a commit event
	return a.runGit("commit", "--allow-empty", "-m", msg, "--author=Test <test@example.com>")
}

func (a *PolyglotActor) AddFile(name, content string) error {
	path := filepath.Join(a.RepoPath, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (a *PolyglotActor) Stage(name string) error {
	return a.runGit("add", name)
}

// GenerateLanguageFile creates a "typical" source file for a given language.
// It includes a symbol name so we can verify its extraction later.
func (a *PolyglotActor) GenerateLanguageFile(lang, symbolName string) error {
	ext, content := getLanguageTemplate(lang, symbolName)
	fileName := fmt.Sprintf("src/pkg_%s/file_%s%s", lang, symbolName, ext)
	return a.AddFile(fileName, content)
}

func (a *PolyglotActor) runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = a.RepoPath
	// Set git configs for the session to avoid global side effects
	cmd.Env = append(os.Environ(), 
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v failed: %v\n%s", args, err, string(out))
	}
	return nil
}

func getLanguageTemplate(lang, symbol string) (ext, content string) {
	switch strings.ToLower(lang) {
	case "go":
		return ".go", fmt.Sprintf("package pkg\n\nfunc %s() {\n\t// fuzzy code\n}\n", symbol)
	case "python":
		return ".py", fmt.Sprintf("def %s():\n    pass\n", symbol)
	case "typescript", "ts":
		return ".ts", fmt.Sprintf("export function %s() {\n  return;\n}\n", symbol)
	case "javascript", "js":
		return ".js", fmt.Sprintf("function %s() {\n  console.log('hi');\n}\n", symbol)
	case "java":
		return ".java", fmt.Sprintf("public class %s {\n    public void run() {}\n}\n", symbol)
	case "rust":
		return ".rs", fmt.Sprintf("pub fn %s() {\n    println!(\"rust\");\n}\n", symbol)
	case "cpp", "c++":
		return ".cpp", fmt.Sprintf("void %s() {\n}\n", symbol)
	case "csharp", "c#":
		return ".cs", fmt.Sprintf("public class %s { public void Work() {} }\n", symbol)
	case "ruby":
		return ".rb", fmt.Sprintf("def %s\n  puts 'ruby'\nend\n", symbol)
	case "php":
		return ".php", fmt.Sprintf("<?php\nfunction %s() {\n}\n", symbol)
	case "swift":
		return ".swift", fmt.Sprintf("func %s() {\n}\n", symbol)
	case "kotlin":
		return ".kt", fmt.Sprintf("fun %s() {\n}\n", symbol)
	case "scala":
		return ".scala", fmt.Sprintf("object %s { def main() {} }\n", symbol)
	case "dart":
		return ".dart", fmt.Sprintf("void %s() {\n}\n", symbol)
	case "zig":
		return ".zig", fmt.Sprintf("pub fn %s() void {}\n", symbol)
	case "elixir":
		return ".ex", fmt.Sprintf("defmodule %s do\n  def run, do: :ok\nend\n", symbol)
	case "haskell":
		return ".hs", fmt.Sprintf("%s :: IO ()\n%s = putStrLn \"hs\"\n", symbol, symbol)
	case "clojure":
		return ".clj", fmt.Sprintf("(defn %s [] (println \"clj\"))\n", symbol)
	case "objective-c", "objc":
		return ".m", fmt.Sprintf("void %s() {}\n", symbol)
	case "lua":
		return ".lua", fmt.Sprintf("function %s()\nend\n", symbol)
	default:
		return ".txt", fmt.Sprintf("Generic content for %s with symbol %s\n", lang, symbol)
	}
}
