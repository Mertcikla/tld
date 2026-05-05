package analyzer

import (
	"context"
	"testing"
)

func TestCPPParser_TopLevelFunctionDeclarations(t *testing.T) {
	parser := &cppParser{}
	source := `#ifndef SERVICE_H
#define SERVICE_H

UV_EXTERN void uv_sleep(unsigned int msec);
int helper(int value);

#endif
`
	result, err := parser.ParseFile(context.Background(), "service.h", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	symbols := map[string]Symbol{}
	for _, sym := range result.Symbols {
		symbols[sym.Name] = sym
	}
	for _, want := range []string{"uv_sleep", "helper"} {
		sym, ok := symbols[want]
		if !ok {
			t.Fatalf("missing top-level declaration %q in symbols: %+v", want, result.Symbols)
		}
		if sym.Kind != "function" || sym.Parent != "" {
			t.Fatalf("%s = kind %q parent %q, want top-level function", want, sym.Kind, sym.Parent)
		}
	}
}

func TestCPPParser_ComplexFunctionDeclarations(t *testing.T) {
	parser := &cppParser{}
	source := `
// Single line
void foo();

/*
   Multi-line
*/
int
bar(
  int x,
  char* y
);

// Declaration with comment inside
void baz( /* inline comment */ int z );

// String containing semicolon and braces
const char* s = "void fake(); { }";

// Braces in comment
/* { */ void depth_test(); /* } */

// Multiline string
const char* ms = "multi \
line \
string";

void after_multiline_string();

void overload(int x);
void overload(double x);
`
	result, err := parser.ParseFile(context.Background(), "complex.h", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	symbols := map[string]Symbol{}
	counts := map[string]int{}
	for _, sym := range result.Symbols {
		symbols[sym.Name] = sym
		counts[sym.Name]++
	}

	wants := []struct {
		name string
		line int
	}{
		{"foo", 3},
		{"bar", 8}, // Fallback scanner finds it at start of declaration
		{"baz", 15},
		{"depth_test", 21},
		{"after_multiline_string", 28},
	}

	for _, want := range wants {
		sym, ok := symbols[want.name]
		if !ok {
			t.Errorf("missing symbol %q", want.name)
			continue
		}
		if counts[want.name] != 1 {
			t.Errorf("%s count = %d, want 1", want.name, counts[want.name])
		}
		// Tree-Sitter and fallback scanner might find same symbol on different lines (start of decl vs start of declarator)
		// We allow both for this test as long as they are close.
		if sym.Line != want.line && sym.Line != want.line+1 {
			t.Errorf("%s line = %d, want %d (or %d)", want.name, sym.Line, want.line, want.line+1)
		}
	}
	if counts["overload"] != 2 {
		t.Errorf("overload count = %d, want 2", counts["overload"])
	}
}
