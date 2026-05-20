package analyzer

import (
	"context"
	"strings"
	"testing"
)

func TestParsersExtractSemanticDocsAndSignatures(t *testing.T) {
	tests := []struct {
		name   string
		parser interface {
			ParseFile(context.Context, string, []byte) (*Result, error)
		}
		path    string
		source  string
		symbol  string
		wantDoc string
		wantSig string
	}{
		{
			name:    "go",
			parser:  &goParser{},
			path:    "orders.go",
			source:  "package main\n\n// Handles orders.\nfunc Handle(ctx context.Context) error { return nil }\n",
			symbol:  "Handle",
			wantDoc: "Handles orders.",
			wantSig: "func Handle(ctx context.Context) error",
		},
		{
			name:    "typescript",
			parser:  &tsParser{},
			path:    "orders.ts",
			source:  "/** Handles orders.\n * @param id ignored\n */\nexport function handleOrder(id: string): void {}\n",
			symbol:  "handleOrder",
			wantDoc: "Handles orders.",
			wantSig: "function handleOrder(id: string): void",
		},
		{
			name:    "python",
			parser:  &pythonParser{},
			path:    "orders.py",
			source:  "def handle_order(order_id: str) -> None:\n    \"\"\"Handles orders.\n\n    :param order_id: ignored\n    \"\"\"\n    return None\n",
			symbol:  "handle_order",
			wantDoc: "Handles orders.",
			wantSig: "def handle_order(order_id: str) -> None:",
		},
		{
			name:    "java",
			parser:  &javaParser{},
			path:    "Orders.java",
			source:  "/** Handles orders.\n * @return ignored\n */\npublic class Orders {\n  /** Charges orders. */\n  public static void handle() {}\n}\n",
			symbol:  "Orders",
			wantDoc: "Handles orders.",
			wantSig: "class Orders",
		},
		{
			name:    "rust",
			parser:  &rustParser{},
			path:    "orders.rs",
			source:  "/// Handles orders.\npub fn handle_order(order_id: &str) -> Result<(), Error> { Ok(()) }\n",
			symbol:  "handle_order",
			wantDoc: "Handles orders.",
			wantSig: "pub fn handle_order(order_id: &str) -> Result<(), Error>",
		},
		{
			name:    "cpp",
			parser:  &cppParser{},
			path:    "orders.cpp",
			source:  "// Handles orders.\nvoid handleOrder(const std::string& orderId) {}\n",
			symbol:  "handleOrder",
			wantDoc: "Handles orders.",
			wantSig: "void handleOrder(const std::string& orderId)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.parser.ParseFile(context.Background(), tt.path, []byte(tt.source))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			var found Symbol
			for _, symbol := range result.Symbols {
				if symbol.Name == tt.symbol {
					found = symbol
					break
				}
			}
			if found.Name == "" {
				t.Fatalf("missing symbol %q in %+v", tt.symbol, result.Symbols)
			}
			if !strings.Contains(found.Description, tt.wantDoc) {
				t.Fatalf("description = %q, want %q", found.Description, tt.wantDoc)
			}
			if !strings.Contains(found.RawSignature, tt.wantSig) {
				t.Fatalf("signature = %q, want %q", found.RawSignature, tt.wantSig)
			}
		})
	}
}
