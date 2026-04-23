package analyzer

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

type javaParser struct{}

func (p *javaParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(sitter.NewLanguage(tree_sitter_java.Language())); err != nil {
		return nil, fmt.Errorf("set java tree-sitter language: %w", err)
	}

	tree, err := parseTree(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("parse java tree-sitter source: %w", err)
	}
	defer tree.Close()

	result := &Result{}
	root := tree.RootNode()
	p.walkNode(root, source, path, "", result)
	return result, nil
}

func (p *javaParser) walkNode(node *sitter.Node, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch node.Kind() {
	case "class_declaration":
		nextParent = p.appendType(node, source, path, parent, "class", result)
	case "interface_declaration":
		nextParent = p.appendType(node, source, path, parent, "interface", result)
	case "enum_declaration":
		nextParent = p.appendType(node, source, path, parent, "enum", result)
	case "record_declaration":
		nextParent = p.appendType(node, source, path, parent, "record", result)
	case "method_declaration":
		p.appendMethod(node, source, path, parent, "method", result)
	case "constructor_declaration":
		p.appendMethod(node, source, path, parent, "constructor", result)
	case "method_invocation":
		p.appendCall(node, source, path, result)
	case "object_creation_expression":
		p.appendObjectCreation(node, source, path, result)
	}

	cursor := node.Walk()
	defer cursor.Close()
	for _, child := range node.NamedChildren(cursor) {
		childCopy := child
		p.walkNode(&childCopy, source, path, nextParent, result)
	}
}

func (p *javaParser) appendType(node *sitter.Node, source []byte, path, parent, kind string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parent
	}
	name := nameNode.Utf8Text(source)
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

func (p *javaParser) appendMethod(node *sitter.Node, source []byte, path, parent, kind string, result *Result) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     nameNode.Utf8Text(source),
		Kind:     kind,
		FilePath: path,
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
}

func (p *javaParser) appendCall(node *sitter.Node, source []byte, path string, result *Result) {
	nameNode := node.ChildByFieldName("name")
	line := int(node.StartPosition().Row) + 1
	name := ""
	if nameNode != nil {
		name = nameNode.Utf8Text(source)
		line = int(nameNode.StartPosition().Row) + 1
	} else {
		name = javaCallName(node.Utf8Text(source))
	}
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		FilePath: path,
		Line:     line,
		Column:   int(node.StartPosition().Column) + 1,
	})
}

func (p *javaParser) appendObjectCreation(node *sitter.Node, source []byte, path string, result *Result) {
	typeNode := node.ChildByFieldName("type")
	line := int(node.StartPosition().Row) + 1
	name := ""
	if typeNode != nil {
		name = javaSimpleName(typeNode.Utf8Text(source))
		line = int(typeNode.StartPosition().Row) + 1
	} else {
		name = javaConstructorName(node.Utf8Text(source))
	}
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		FilePath: path,
		Line:     line,
		Column:   int(node.StartPosition().Column) + 1,
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
