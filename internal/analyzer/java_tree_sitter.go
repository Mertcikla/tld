package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

type javaParser struct{}

func (p *javaParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parsed, err := parseTree(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse java tree-sitter source: %w", err)
	}
	defer parsed.Close()

	result := &Result{}
	root := parsed.tree.RootNode()
	p.walkNode(root, parsed.lang, source, path, "", result)
	return result, nil
}

func (p *javaParser) walkNode(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch nodeKind(node, lang) {
	case "class_declaration":
		nextParent = p.appendType(node, lang, source, path, parent, "class", result)
	case "interface_declaration":
		nextParent = p.appendType(node, lang, source, path, parent, "interface", result)
	case "enum_declaration":
		nextParent = p.appendType(node, lang, source, path, parent, "enum", result)
	case "record_declaration":
		nextParent = p.appendType(node, lang, source, path, parent, "record", result)
	case "method_declaration":
		p.appendMethod(node, lang, source, path, parent, "method", result)
	case "constructor_declaration":
		p.appendMethod(node, lang, source, path, parent, "constructor", result)
	case "method_invocation":
		p.appendCall(node, lang, source, path, result)
	case "object_creation_expression":
		p.appendObjectCreation(node, lang, source, path, result)
	}

	for _, child := range namedChildren(node) {
		p.walkNode(child, lang, source, path, nextParent, result)
	}
}

func (p *javaParser) appendType(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent, kind string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return parent
	}
	name := nodeText(nameNode, source)
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

func (p *javaParser) appendMethod(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent, kind string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     nodeText(nameNode, source),
		Kind:     kind,
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   parent,
	})
}

func (p *javaParser) appendCall(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	line := int(node.StartPoint().Row) + 1
	name := ""
	if nameNode != nil {
		name = nodeText(nameNode, source)
		line = int(nameNode.StartPoint().Row) + 1
	} else {
		name = javaCallName(nodeText(node, source))
	}
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		FilePath: path,
		Line:     line,
		Column:   int(node.StartPoint().Column) + 1,
	})
}

func (p *javaParser) appendObjectCreation(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	typeNode := childByFieldName(node, lang, "type")
	line := int(node.StartPoint().Row) + 1
	name := ""
	if typeNode != nil {
		name = javaSimpleName(nodeText(typeNode, source))
		line = int(typeNode.StartPoint().Row) + 1
	} else {
		name = javaConstructorName(nodeText(node, source))
	}
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		FilePath: path,
		Line:     line,
		Column:   int(node.StartPoint().Column) + 1,
	})
}

func javaCallName(text string) string {
	if index := strings.Index(text, "("); index >= 0 {
		text = text[:index]
	}
	return javaSimpleName(text)
}

func javaConstructorName(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "new "))
	if index := strings.Index(text, "("); index >= 0 {
		text = text[:index]
	}
	return javaSimpleName(text)
}

func javaSimpleName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if index := strings.Index(text, "<"); index >= 0 {
		text = text[:index]
	}
	if index := strings.LastIndex(text, "."); index >= 0 {
		text = text[index+1:]
	}
	fields := strings.Fields(text)
	if len(fields) > 0 {
		text = fields[len(fields)-1]
	}
	return strings.TrimSpace(text)
}
