package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

type cppParser struct{}

func (p *cppParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parsed, err := parseTree(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse c++ tree-sitter source: %w", err)
	}
	defer parsed.Close()

	result := &Result{}
	root := parsed.tree.RootNode()
	p.walkNode(root, parsed.lang, source, path, "", result)
	return result, nil
}

func (p *cppParser) walkNode(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch nodeKind(node, lang) {
	case "class_specifier":
		nextParent = p.appendType(node, lang, source, path, parent, "class", result)
	case "struct_specifier":
		nextParent = p.appendType(node, lang, source, path, parent, "struct", result)
	case "enum_specifier":
		nextParent = p.appendType(node, lang, source, path, parent, "enum", result)
	case "function_definition":
		p.appendFunction(node, lang, source, path, parent, result)
	case "declaration":
		p.appendMemberDeclaration(node, lang, source, path, parent, result)
	case "call_expression":
		p.appendCall(node, lang, source, path, result)
	}

	for _, child := range namedChildren(node) {
		p.walkNode(child, lang, source, path, nextParent, result)
	}
}

func (p *cppParser) appendType(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent, kind string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		nameNode = cppFirstNamedIdentifier(node, lang, source)
	}
	if nameNode == nil {
		return parent
	}
	name := cppSimpleName(nodeText(nameNode, source))
	if name == "" {
		return parent
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     kind,
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   parent,
	})
	return name
}

func (p *cppParser) appendFunction(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	declarator := childByFieldName(node, lang, "declarator")
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
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   owner,
	})
}

func (p *cppParser) appendMemberDeclaration(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	if parent == "" {
		return
	}
	declarator := childByFieldName(node, lang, "declarator")
	if !cppHasFunctionDeclarator(declarator, lang) {
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
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   owner,
	})
}

func (p *cppParser) appendCall(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	functionNode := childByFieldName(node, lang, "function")
	if functionNode == nil {
		return
	}
	name := cppSimpleName(nodeText(functionNode, source))
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		FilePath: path,
		Line:     int(functionNode.StartPoint().Row) + 1,
		Column:   int(functionNode.StartPoint().Column) + 1,
	})
}

func cppFunctionInfo(declarator *gotreesitter.Node, source []byte) (string, string) {
	if declarator == nil {
		return "", ""
	}
	text := strings.TrimSpace(nodeText(declarator, source))
	if text == "" || !strings.Contains(text, "(") {
		return "", ""
	}
	return cppFunctionName(text), cppFunctionOwner(text)
}

func cppHasFunctionDeclarator(node *gotreesitter.Node, lang *gotreesitter.Language) bool {
	if node == nil {
		return false
	}
	if strings.HasSuffix(nodeKind(node, lang), "function_declarator") {
		return true
	}
	for _, child := range namedChildren(node) {
		if cppHasFunctionDeclarator(child, lang) {
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

func cppNodeLine(primary, fallback *gotreesitter.Node) int {
	if primary != nil {
		return int(primary.StartPoint().Row) + 1
	}
	if fallback != nil {
		return int(fallback.StartPoint().Row) + 1
	}
	return 0
}

func cppFirstNamedIdentifier(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if name := cppSimpleName(nodeText(node, source)); name != "" {
		switch nodeKind(node, lang) {
		case "type_identifier", "identifier", "field_identifier", "namespace_identifier":
			return node
		}
	}
	for _, child := range namedChildren(node) {
		if match := cppFirstNamedIdentifier(child, lang, source); match != nil {
			return match
		}
	}
	return nil
}
