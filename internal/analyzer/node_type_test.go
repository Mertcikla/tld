package analyzer

import "testing"

func assertSymbolNodeType(t *testing.T, result *Result, name, kind, parent, nodeType string) {
	t.Helper()
	for _, actual := range result.Symbols {
		if actual.Name == name && actual.Kind == kind && actual.Parent == parent {
			if actual.NodeType != nodeType {
				t.Fatalf("%s node_type = %q, want %q in symbols: %+v", name, actual.NodeType, nodeType, result.Symbols)
			}
			return
		}
	}
	t.Fatalf("symbol %s/%s/%s not found in symbols: %+v", name, kind, parent, result.Symbols)
}
