package analyzer

import (
	"context"
	"testing"
)

func TestJavaParser_NodeTypes(t *testing.T) {
	parser := &javaParser{}
	source := `
class Service {
  Service() {}
  void start() {}
}
`
	result, err := parser.ParseFile(context.Background(), "Service.java", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	assertSymbolNodeType(t, result, "Service", "class", "", "class_declaration")
	assertSymbolNodeType(t, result, "start", "method", "Service", "method_declaration")
}
