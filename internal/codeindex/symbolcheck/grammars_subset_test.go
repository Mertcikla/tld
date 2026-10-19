//go:build grammar_subset

package symbolcheck

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Exercise actual parsing so missing embedded blobs or external scanners fail
// here rather than silently disabling indexing in a release binary.
func TestGrammarSubsetParsesSupportedLanguages(t *testing.T) {
	samples := map[string]string{
		"go":         "package main\nfunc main() {}\n",
		"javascript": "function hello() { return 1; }\n",
		"python":     "def hello():\n    return 1\n",
		"typescript": "function hello(): number { return 1; }\n",
		"tsx":        "const hello = <div>Hello</div>;\n",
		"c_sharp":    "class Hello { int Value() { return 1; } }\n",
		"c":          "int hello(void) { return 1; }\n",
		"cpp":        "int hello() { return 1; }\n",
		"dart":       "int hello() { return 1; }\n",
		"php":        "<?php function hello() { return 1; }\n",
		"rust":       "fn hello() -> i32 { 1 }\n",
	}
	if entries := grammars.AllLanguages(); len(entries) != len(samples) {
		t.Fatalf("registered %d grammars, want exactly %d", len(entries), len(samples))
	}
	for name, source := range samples {
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			if entry == nil {
				t.Fatalf("grammar %q is not registered", name)
			}
			language := entry.Language()
			if language == nil {
				t.Fatalf("grammar %q could not load its embedded blob", name)
			}
			parser := gotreesitter.NewParser(language)
			tree, err := parser.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if tree == nil {
				t.Fatal("parser returned no tree")
			}
			defer tree.Release()
			if root := tree.RootNode(); root == nil || root.HasError() {
				t.Fatalf("grammar %q did not parse its sample cleanly", name)
			}
		})
	}
}
