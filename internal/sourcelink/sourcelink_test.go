package sourcelink

import "testing"

func TestParseLineAnchors(t *testing.T) {
	parsed := Parse("grpc/server.go#L18")
	if parsed.BasePath != "grpc/server.go" {
		t.Fatalf("basePath = %q", parsed.BasePath)
	}
	if parsed.Anchor.Kind != AnchorLine || parsed.Anchor.StartLine != 18 || parsed.Anchor.EndLine != 18 {
		t.Fatalf("anchor = %+v", parsed.Anchor)
	}
	if got := Label(parsed.Anchor); got != "L18" {
		t.Fatalf("label = %q", got)
	}

	rangeParsed := Parse("grpc/server.go#L7-L9")
	if rangeParsed.Anchor.StartLine != 7 || rangeParsed.Anchor.EndLine != 9 {
		t.Fatalf("range anchor = %+v", rangeParsed.Anchor)
	}
}

func TestFormatAndParseSymbolAnchors(t *testing.T) {
	link := FormatSymbol("grpc/server.go", "method_declaration", "Server.Listen")
	if link != "grpc/server.go#method_declaration:Server.Listen" {
		t.Fatalf("link = %q", link)
	}
	parsed := Parse(link)
	if parsed.BasePath != "grpc/server.go" {
		t.Fatalf("basePath = %q", parsed.BasePath)
	}
	if parsed.Anchor.Kind != AnchorSymbol || parsed.Anchor.NodeType != "method_declaration" || parsed.Anchor.Symbol != "Server.Listen" {
		t.Fatalf("anchor = %+v", parsed.Anchor)
	}
}

func TestSymbolEncodingPreservesNodeType(t *testing.T) {
	link := FormatSymbol("src/api.ts#L4", "function_declaration", "listen:public route")
	if link != "src/api.ts#function_declaration:listen%3Apublic%20route" {
		t.Fatalf("link = %q", link)
	}
	parsed := Parse(link)
	if parsed.Anchor.Kind != AnchorSymbol || parsed.Anchor.NodeType != "function_declaration" || parsed.Anchor.Symbol != "listen:public route" {
		t.Fatalf("anchor = %+v", parsed.Anchor)
	}
}

func TestFormatLineReplacesExistingAnchor(t *testing.T) {
	if got := FormatLine("grpc/server.go", 18); got != "grpc/server.go#L18" {
		t.Fatalf("got %q", got)
	}
	if got := FormatLine("grpc/server.go#function_declaration:Listen", 22); got != "grpc/server.go#L22" {
		t.Fatalf("got %q", got)
	}
}

func TestParseLegacyJSONAnchors(t *testing.T) {
	parsed := Parse(`src/app.ts#{"name":"App","type":"function_declaration"}`)
	if parsed.Anchor.Kind != AnchorSymbol || parsed.Anchor.NodeType != "function_declaration" || parsed.Anchor.Symbol != "App" {
		t.Fatalf("symbol anchor = %+v", parsed.Anchor)
	}
	lineParsed := Parse(`src/app.ts#{"startLine":7,"endLine":9}`)
	if lineParsed.Anchor.Kind != AnchorLine || lineParsed.Anchor.StartLine != 7 || lineParsed.Anchor.EndLine != 9 {
		t.Fatalf("line anchor = %+v", lineParsed.Anchor)
	}
}

func TestParseNoAnchor(t *testing.T) {
	parsed := Parse("internal/api.go")
	if parsed.BasePath != "internal/api.go" || parsed.Anchor.Kind != AnchorNone {
		t.Fatalf("parsed = %+v", parsed)
	}
	if got := BasePath("internal/api.go#L3"); got != "internal/api.go" {
		t.Fatalf("basePath = %q", got)
	}
}
