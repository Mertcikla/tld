package analyzer

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type cppParser struct{}

func (p *cppParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(sitter.NewLanguage(tree_sitter_cpp.Language())); err != nil {
		return nil, fmt.Errorf("set c++ tree-sitter language: %w", err)
	}

	tree, err := parseTree(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("parse c++ tree-sitter source: %w", err)
	}
	defer tree.Close()

	result := &Result{}
	root := tree.RootNode()
	p.walkNode(root, source, path, "", result)
	return result, nil
}

func (p *cppParser) walkNode(node *sitter.Node, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch node.Kind() {
	case "class_specifier":
		nextParent = p.appendType(node, source, path, parent, "class", result)
	case "struct_specifier":
		nextParent = p.appendType(node, source, path, parent, "struct", result)
	case "enum_specifier":
		nextParent = p.appendType(node, source, path, parent, "enum", result)
	case "function_definition":
		p.appendFunction(node, source, path, parent, result)
	case "declaration":
		p.appendMemberDeclaration(node, source, path, parent, result)
	case "call_expression":
		p.appendCall(node, source, path, result)
	}

	cursor := node.Walk()
	defer cursor.Close()
	for _, child := range node.NamedChildren(cursor) {
		childCopy := child
		p.walkNode(&childCopy, source, path, nextParent, result)
	}
}

func (p *cppParser) appendType(node *sitter.Node, source []byte, path, parent, kind string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		nameNode = cppFirstNamedIdentifier(node, source)
	}
	if nameNode == nil {
		return parent
	}
	name := cppSimpleName(nameNode.Utf8Text(source))
	if name == "" {
		return parent
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     kind,
		FilePath: path,
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
	return name
}

func (p *cppParser) appendFunction(node *sitter.Node, source []byte, path, parent string, result *Result) {
	declarator := node.ChildByFieldName("declarator")
	name, owner := cppFunctionInfo(declarator, source)
	if name == "" {
		return
	}
	owner = cppResolveOwner(owner, parent)
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     cppFunctionKind(name, owner),
		FilePath: path,
		Line:     cppNodeLine(declarator, node),
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   owner,
	})
}

func (p *cppParser) appendMemberDeclaration(node *sitter.Node, source []byte, path, parent string, result *Result) {
	if parent == "" {
		return
	}
	declarator := node.ChildByFieldName("declarator")
	if !cppHasFunctionDeclarator(declarator) {
		return
	}
	name, owner := cppFunctionInfo(declarator, source)
	if name == "" {
		return
	}
	owner = cppResolveOwner(owner, parent)
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     cppFunctionKind(name, owner),
		FilePath: path,
		Line:     cppNodeLine(declarator, node),
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   owner,
	})
}

func (p *cppParser) appendCall(node *sitter.Node, source []byte, path string, result *Result) {
	functionNode := node.ChildByFieldName("function")
	if functionNode == nil {
		return
	}
	name := cppSimpleName(functionNode.Utf8Text(source))
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		FilePath: path,
		Line:     int(functionNode.StartPosition().Row) + 1,
		Column:   int(functionNode.StartPosition().Column) + 1,
	})
}

func cppFunctionInfo(declarator *sitter.Node, source []byte) (string, string) {
	if declarator == nil {
		return "", ""
	}
	text := strings.TrimSpace(declarator.Utf8Text(source))
	if text == "" || !strings.Contains(text, "(") {
		return "", ""
	}
	return cppFunctionName(text), cppFunctionOwner(text)
}

func cppHasFunctionDeclarator(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	if strings.HasSuffix(node.Kind(), "function_declarator") {
		return true
	}
	cursor := node.Walk()
	defer cursor.Close()
	for _, child := range node.NamedChildren(cursor) {
		childCopy := child
		if cppHasFunctionDeclarator(&childCopy) {
			return true
		}
	}
	return false
}

func cppFunctionKind(name, owner string) string {
	if owner == "" {
		return "function"
	}
	trimmed := strings.TrimPrefix(name, "~")
	if trimmed == owner {
		if strings.HasPrefix(name, "~") {
			return "destructor"
		}
		return "constructor"
	}
	return "method"
}

func cppResolveOwner(owner, parent string) string {
	if owner != "" {
		return owner
	}
	return parent
}

func cppFunctionName(text string) string {
	prefix := cppBeforeCall(text)
	return cppSimpleName(prefix)
}

func cppFunctionOwner(text string) string {
	prefix := cppBeforeCall(text)
	index := strings.LastIndex(prefix, "::")
	if index < 0 {
		return ""
	}
	return cppSimpleName(prefix[:index])
}

func cppBeforeCall(text string) string {
	text = strings.TrimSpace(text)
	if index := strings.Index(text, "("); index >= 0 {
		text = text[:index]
	}
	return strings.TrimSpace(text)
}

func cppSimpleName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.TrimLeft(text, "*&")
	for _, sep := range []string{"->", "::", "."} {
		if index := strings.LastIndex(text, sep); index >= 0 {
			text = text[index+len(sep):]
		}
	}
	fields := strings.Fields(text)
	if len(fields) > 0 {
		text = fields[len(fields)-1]
	}
	text = strings.TrimLeft(text, "*&")
	if index := strings.Index(text, "<"); index >= 0 {
		text = text[:index]
	}
	return strings.TrimSpace(text)
}

func cppNodeLine(primary, fallback *sitter.Node) int {
	if primary != nil {
		return int(primary.StartPosition().Row) + 1
	}
	if fallback != nil {
		return int(fallback.StartPosition().Row) + 1
	}
	return 0
}

func cppFirstNamedIdentifier(node *sitter.Node, source []byte) *sitter.Node {
	if node == nil {
		return nil
	}
	if name := cppSimpleName(node.Utf8Text(source)); name != "" {
		switch node.Kind() {
		case "type_identifier", "identifier", "field_identifier", "namespace_identifier":
			return node
		}
	}
	cursor := node.Walk()
	defer cursor.Close()
	for _, child := range node.NamedChildren(cursor) {
		childCopy := child
		if match := cppFirstNamedIdentifier(&childCopy, source); match != nil {
			return match
		}
	}
	return nil
}
