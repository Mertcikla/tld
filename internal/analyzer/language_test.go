package analyzer

import (
	"reflect"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := map[string]Language{
		"service.go": LanguageGo,
		"worker.py":  LanguagePython,
		"main.rs":    LanguageRust,
		"app.java":   LanguageJava,
		"index.ts":   LanguageTypeScript,
		"widget.tsx": LanguageTypeScript,
		"server.js":  LanguageJavaScript,
		"client.jsx": LanguageJavaScript,
		"engine.cpp": LanguageCPP,
		"header.h":   LanguageC,
		"program.c":  LanguageC,
	}

	for path, want := range tests {
		got, ok := DetectLanguage(path)
		if !ok {
			t.Fatalf("DetectLanguage(%q) reported unsupported", path)
		}
		if got != want {
			t.Fatalf("DetectLanguage(%q) = %q, want %q", path, got, want)
		}
	}

	if _, ok := DetectLanguage("README.md"); ok {
		t.Fatal("DetectLanguage reported markdown as supported")
	}
}

func TestGroupFilesByLanguage(t *testing.T) {
	grouped := GroupFilesByLanguage([]string{
		"README.md",
		"pkg/service.go",
		"pkg/handler.go",
		"src/index.ts",
		"src/util.tsx",
		"scripts/tool.py",
	})

	if !reflect.DeepEqual(grouped[LanguageGo], []string{"pkg/handler.go", "pkg/service.go"}) {
		t.Fatalf("unexpected go grouping: %#v", grouped[LanguageGo])
	}
	if !reflect.DeepEqual(grouped[LanguageTypeScript], []string{"src/index.ts", "src/util.tsx"}) {
		t.Fatalf("unexpected ts grouping: %#v", grouped[LanguageTypeScript])
	}
	if !reflect.DeepEqual(grouped[LanguagePython], []string{"scripts/tool.py"}) {
		t.Fatalf("unexpected python grouping: %#v", grouped[LanguagePython])
	}
	if _, ok := grouped[LanguageJavaScript]; ok {
		t.Fatalf("unexpected javascript grouping: %#v", grouped[LanguageJavaScript])
	}
	if len(grouped) != 3 {
		t.Fatalf("expected 3 supported language groups, got %d", len(grouped))
	}
}
