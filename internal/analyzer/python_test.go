package analyzer

import (
	"context"
	"testing"
)

func TestPythonParser_ParseFile(t *testing.T) {
	parser := &pythonParser{}
	source := `
import os
import sys as system
from datetime import datetime
from .local import config
from ..parent import base

class Service:
    @property
    def name(self):
        return "service"

    def __init__(self):
        self.os = os
        self.system = system

    def handle(self):
        helper()
        self.internal()

    def internal(self):
        pass

def helper():
    datetime.now()
`
	result, err := parser.ParseFile(context.Background(), "test.py", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Verify Symbols
	expectedSymbols := []Symbol{
		{Name: "Service", Kind: "class", Parent: ""},
		{Name: "name", Kind: "method", Parent: "Service"},
		{Name: "__init__", Kind: "constructor", Parent: "Service"},
		{Name: "handle", Kind: "method", Parent: "Service"},
		{Name: "internal", Kind: "method", Parent: "Service"},
		{Name: "helper", Kind: "function", Parent: ""},
	}

	for _, expected := range expectedSymbols {
		found := false
		for _, actual := range result.Symbols {
			if actual.Name == expected.Name && actual.Kind == expected.Kind && actual.Parent == expected.Parent {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Symbol not found or mismatch: %+v", expected)
		}
	}

	// Verify Refs
	expectedRefs := []Ref{
		{Name: "os", Kind: "import", TargetPath: "os"},
		{Name: "sys", Kind: "import", TargetPath: "sys"},
		{Name: "datetime", Kind: "import", TargetPath: "datetime"},
		{Name: "config", Kind: "import", TargetPath: ".local"},
		{Name: "base", Kind: "import", TargetPath: "..parent"},
		{Name: "property", Kind: "call"},
		{Name: "helper", Kind: "call"},
		{Name: "internal", Kind: "call"},
		{Name: "now", Kind: "call"},
	}

	for _, expected := range expectedRefs {
		found := false
		for _, actual := range result.Refs {
			if actual.Name == expected.Name && (expected.Kind == "" || actual.Kind == expected.Kind) {
				if expected.TargetPath != "" && actual.TargetPath != expected.TargetPath {
					continue
				}
				if expected.Name == "sys" {
					if actual.Line != 3 || actual.Column != 8 {
						t.Errorf("sys ref position mismatch: line %d col %d, want line 3 col 8", actual.Line, actual.Column)
					}
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Ref not found or mismatch: %+v", expected)
		}
	}
}
