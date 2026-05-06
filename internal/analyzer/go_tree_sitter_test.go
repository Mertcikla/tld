package analyzer

import (
	"context"
	"testing"
)

func TestGoParser_MethodReceiverParent(t *testing.T) {
	parser := &goParser{}
	source := `package main

type Page struct{}
type Card struct{}

func (p *Page) Render() {}
func (c Card) Render() {}
`
	result, err := parser.ParseFile(context.Background(), "view.go", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	parents := map[string]bool{}
	for _, sym := range result.Symbols {
		if sym.Kind == "method" && sym.Name == "Render" {
			parents[sym.Parent] = true
		}
	}
	for _, want := range []string{"Page", "Card"} {
		if !parents[want] {
			t.Fatalf("missing Render parent %q in symbols: %+v", want, result.Symbols)
		}
	}
}
