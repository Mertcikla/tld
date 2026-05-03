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
