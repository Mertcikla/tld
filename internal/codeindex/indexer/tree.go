package indexer

import (
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// callSiteFor records a call expression found during the declaration walk so
// deriveCalls can resolve it against SCIP references without re-parsing.
func callSiteFor(n *tsNode, s *graph.Source) (callSite, bool) {
	isCall := n.Kind() == "call_expression" || (s.Language == "python" && n.Kind() == "call")
	if !isCall {
		return callSite{}, false
	}
	callee := n.ChildByFieldName("function")
	if callee == nil {
		return callSite{}, false
	}
	return callSite{path: s.Path, start: uint32(n.StartByte()), end: uint32(n.EndByte()), calleeStart: uint32(callee.StartByte()), calleeEnd: uint32(callee.EndByte())}, true
}
func nodeFactKind(n *tsNode, src []byte) pb.FactKind {
	kind := n.Kind()
	switch kind {
	case "function_declaration", "generator_function_declaration":
		return pb.FactKind_FACT_KIND_FUNCTION
	case "function_definition":
		if declaredInType(n) {
			return pb.FactKind_FACT_KIND_METHOD
		}
		return pb.FactKind_FACT_KIND_FUNCTION
	case "class_definition", "class_declaration", "class_specifier", "record_declaration":
		return pb.FactKind_FACT_KIND_CLASS
	case "decorated_definition":
		if definition := n.ChildByFieldName("definition"); definition != nil {
			return nodeFactKind(definition, src)
		}
	case "method_declaration", "method_definition", "method_signature":
		if name := declarationName(n, src); name == "constructor" || name == "__construct" || name == "__destruct" {
			return pb.FactKind_FACT_KIND_CONSTRUCTOR
		}
		return pb.FactKind_FACT_KIND_METHOD
	case "interface_declaration", "trait_declaration":
		return pb.FactKind_FACT_KIND_INTERFACE
	case "enum_declaration", "enum_specifier":
		return pb.FactKind_FACT_KIND_ENUM
	case "struct_declaration", "struct_specifier", "union_specifier":
		return pb.FactKind_FACT_KIND_STRUCT
	case "constructor_declaration":
		return pb.FactKind_FACT_KIND_CONSTRUCTOR
	case "constructor_signature", "factory_constructor_signature",
		"constant_constructor_signature", "redirecting_factory_constructor_signature":
		if nestedSignature(n) {
			return pb.FactKind_FACT_KIND_UNSPECIFIED
		}
		return pb.FactKind_FACT_KIND_CONSTRUCTOR
	case "destructor_declaration":
		return pb.FactKind_FACT_KIND_METHOD
	case "local_function_statement", "function_signature", "getter_signature", "setter_signature",
		"operator_signature", "operator_declaration", "conversion_operator_declaration":
		if nestedSignature(n) {
			return pb.FactKind_FACT_KIND_UNSPECIFIED
		}
		return pb.FactKind_FACT_KIND_FUNCTION
	case "delegate_declaration", "type_definition", "mixin_declaration", "extension_declaration":
		return pb.FactKind_FACT_KIND_TYPE
	case "declaration", "field_declaration":
		if hasFunctionDeclarator(n) {
			return pb.FactKind_FACT_KIND_FUNCTION
		}
	case "type_alias_declaration":
		return pb.FactKind_FACT_KIND_TYPE
	case "type_spec":
		code := string(src[n.StartByte():n.EndByte()])
		if strings.Contains(code, "struct {") || strings.Contains(code, "struct{") {
			return pb.FactKind_FACT_KIND_STRUCT
		}
		if strings.Contains(code, "interface {") || strings.Contains(code, "interface{") {
			return pb.FactKind_FACT_KIND_INTERFACE
		}
		return pb.FactKind_FACT_KIND_TYPE
	case "lexical_declaration", "variable_declaration":
		code := string(src[n.StartByte():n.EndByte()])
		if strings.Contains(code, "=>") || strings.Contains(code, "= function") {
			return pb.FactKind_FACT_KIND_FUNCTION
		}
	}
	return pb.FactKind_FACT_KIND_UNSPECIFIED
}

// declaredInType reports whether a function definition is a method: its nearest
// enclosing definition is a type rather than another function or the module.
func declaredInType(n *tsNode) bool {
	for parent := n.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "class_definition", "class_declaration", "class_specifier", "struct_specifier",
			"struct_declaration", "interface_declaration", "trait_declaration",
			"record_declaration", "mixin_declaration", "extension_declaration":
			return true
		case "function_definition", "function_declaration", "method_declaration",
			"method_definition", "module", "program", "translation_unit", "source_file":
			return false
		}
	}
	return false
}

// nestedSignature reports whether a bare function/getter/setter/operator
// signature is already wrapped by a method signature, which is the declaration
// node that owns the body for Dart members.
func nestedSignature(n *tsNode) bool {
	if parent := n.Parent(); parent != nil {
		return parent.Kind() == "method_signature"
	}
	return false
}

// hasFunctionDeclarator reports whether a C/C++ declaration or field
// declaration introduces a function prototype rather than a variable.
func hasFunctionDeclarator(n *tsNode) bool {
	for i := 0; i < n.NamedChildCount(); i++ {
		if n.NamedChild(i).Kind() == "function_declarator" {
			return true
		}
	}
	return false
}

func declarationName(n *tsNode, src []byte) string {
	if n.Kind() == "decorated_definition" {
		if definition := n.ChildByFieldName("definition"); definition != nil {
			return declarationName(definition, src)
		}
	}
	if child := n.ChildByFieldName("name"); child != nil {
		return string(src[child.StartByte():child.EndByte()])
	}
	if n.Kind() == "method_signature" {
		for i := 0; i < n.NamedChildCount(); i++ {
			if name := declarationName(n.NamedChild(i), src); name != "" {
				return name
			}
		}
	}
	if n.Kind() == "function_definition" || n.Kind() == "declaration" || n.Kind() == "field_declaration" || n.Kind() == "type_definition" {
		if declarator := n.ChildByFieldName("declarator"); declarator != nil {
			if name := declaratorName(declarator, src); name != "" {
				return name
			}
		}
	}
	if n.Kind() == "lexical_declaration" || n.Kind() == "variable_declaration" {
		for i := 0; i < n.NamedChildCount(); i++ {
			child := n.NamedChild(i)
			if child.Kind() == "variable_declarator" {
				if name := child.ChildByFieldName("name"); name != nil {
					return string(src[name.StartByte():name.EndByte()])
				}
			}
		}
	}
	return ""
}

// declaratorName resolves the declared identifier of a C/C++ declarator chain,
// which nests the name before the parameter list.
func declaratorName(n *tsNode, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier", "field_identifier", "type_identifier", "operator_name", "destructor_name":
		return string(src[n.StartByte():n.EndByte()])
	}
	if name := n.ChildByFieldName("name"); name != nil {
		if value := declaratorName(name, src); value != "" {
			return value
		}
	}
	if inner := n.ChildByFieldName("declarator"); inner != nil {
		if value := declaratorName(inner, src); value != "" {
			return value
		}
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		if value := declaratorName(n.NamedChild(i), src); value != "" {
			return value
		}
	}
	return ""
}
func attachedComments(src []byte, start int) int {
	if start <= 0 {
		return start
	}
	lineStart := start
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}
	at := lineStart
	for at > 0 {
		prevEnd := at - 1
		if prevEnd > 0 && src[prevEnd] == '\n' {
			prevEnd--
		}
		prevStart := prevEnd
		for prevStart > 0 && src[prevStart-1] != '\n' {
			prevStart--
		}
		line := strings.TrimSpace(string(src[prevStart:at]))
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") ||
			strings.HasPrefix(line, "*") || strings.HasPrefix(line, "#") {
			at = prevStart
			continue
		}
		break
	}
	if at < lineStart {
		return at
	}
	return start
}
func signatureContext(src []byte, start, end int, name string) string {
	b := src[start:end]
	cut := len(b)
	if p := strings.IndexByte(string(b), '{'); p >= 0 && p < cut {
		cut = p
	}
	if p := strings.IndexByte(string(b), '\n'); p >= 0 && p < cut {
		cut = p
	}
	if cut > 240 {
		cut = 240
	}
	t := strings.TrimSpace(string(b[:cut]))
	if t == "" {
		return name
	}
	return t
}
