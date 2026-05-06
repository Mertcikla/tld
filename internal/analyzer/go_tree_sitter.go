package analyzer

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

type goParser struct{}

func (p *goParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parsed, err := parseTree(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse go tree-sitter source: %w", err)
	}
	defer parsed.Close()

	result := &Result{}
	root := parsed.tree.RootNode()
	p.walkNode(root, parsed.lang, source, path, result)
	return result, nil
}

func (p *goParser) walkNode(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	if node == nil {
		return
	}

	switch nodeKind(node, lang) {
	case "function_declaration":
		p.appendFunction(node, lang, source, path, "function", result)
	case "method_declaration":
		p.appendFunction(node, lang, source, path, "method", result)
	case "type_spec":
		p.appendTypeSpec(node, lang, source, path, result)
	case "type_alias":
		p.appendTypeAlias(node, lang, source, path, result)
	case "import_spec":
		p.appendImport(node, lang, source, path, result)
	case "call_expression":
		p.appendCall(node, lang, source, path, result)
	}

	for _, child := range namedChildren(node) {
		p.walkNode(child, lang, source, path, result)
	}
}

func (p *goParser) appendFunction(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, kind string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return
	}
	parent := ""
	if kind == "method" {
		parent = goReceiverTypeName(node, lang, source)
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:        nodeText(nameNode, source),
		Kind:        kind,
		FilePath:    path,
		Line:        int(nameNode.StartPoint().Row) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		Parent:      parent,
		Description: p.findComment(node, lang, source),
	})
}

func (p *goParser) appendTypeSpec(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return
	}
	typeNode := childByFieldName(node, lang, "type")
	kind := "type"
	if typeNode != nil {
		switch nodeKind(typeNode, lang) {
		case "struct_type":
			kind = "struct"
		case "interface_type":
			kind = "interface"
		}
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:        nodeText(nameNode, source),
		Kind:        kind,
		FilePath:    path,
		Line:        int(nameNode.StartPoint().Row) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		Description: p.findComment(node, lang, source),
	})
}

func (p *goParser) appendTypeAlias(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     nodeText(nameNode, source),
		Kind:     "type",
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		EndLine:  int(node.EndPoint().Row) + 1,
	})
}

func (p *goParser) appendCall(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	functionNode := childByFieldName(node, lang, "function")
	if functionNode == nil {
		return
	}
	name := goCallName(functionNode, lang, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(functionNode.StartPoint().Row) + 1,
		Column:   int(functionNode.StartPoint().Column) + 1,
	})
}

func (p *goParser) appendImport(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, filePath string, result *Result) {
	pathNode := childByFieldName(node, lang, "path")
	if pathNode == nil {
		return
	}
	importPath, err := strconv.Unquote(strings.TrimSpace(nodeText(pathNode, source)))
	if err != nil || importPath == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:       path.Base(importPath),
		Kind:       "import",
		TargetPath: importPath,
		FilePath:   filePath,
		Line:       int(pathNode.StartPoint().Row) + 1,
		Column:     int(pathNode.StartPoint().Column) + 1,
	})
}

func (p *goParser) findComment(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	prev := prevNamedSibling(node)
	if prev == nil || nodeKind(prev, lang) != "comment" {
		return ""
	}
	// Check if it's immediately above
	if node.StartPoint().Row-prev.EndPoint().Row > 1 {
		return ""
	}
	text := strings.TrimSpace(nodeText(prev, source))
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	return strings.TrimSpace(text)
}

func goCallName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	switch nodeKind(node, lang) {
	case "identifier", "field_identifier", "type_identifier":
		return nodeText(node, source)
	case "selector_expression":
		fieldNode := childByFieldName(node, lang, "field")
		if fieldNode != nil {
			return nodeText(fieldNode, source)
		}
	case "parenthesized_expression":
		children := namedChildren(node)
		if len(children) > 0 {
			return goCallName(children[0], lang, source)
		}
	}
	text := strings.TrimSpace(nodeText(node, source))
	if text == "" {
		return ""
	}
	if index := strings.LastIndex(text, "."); index >= 0 {
		text = text[index+1:]
	}
	if index := strings.Index(text, "["); index >= 0 {
		text = text[:index]
	}
	return strings.TrimSpace(text)
}

func goReceiverTypeName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	receiver := childByFieldName(node, lang, "receiver")
	if receiver == nil {
		return ""
	}
	for _, child := range namedChildren(receiver) {
		if nodeKind(child, lang) != "parameter_declaration" {
			continue
		}
		if name := goTypeName(childByFieldName(child, lang, "type"), lang, source); name != "" {
			return name
		}
	}
	return goTypeName(receiver, lang, source)
}

func goTypeName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	switch nodeKind(node, lang) {
	case "type_identifier", "identifier", "field_identifier":
		return nodeText(node, source)
	case "qualified_type", "selector_expression":
		children := namedChildren(node)
		for i := len(children) - 1; i >= 0; i-- {
			if name := goTypeName(children[i], lang, source); name != "" {
				return name
			}
		}
	}
	for _, field := range []string{"type", "name"} {
		if name := goTypeName(childByFieldName(node, lang, field), lang, source); name != "" {
			return name
		}
	}
	for _, child := range namedChildren(node) {
		if nodeKind(child, lang) == "type_arguments" {
			continue
		}
		if name := goTypeName(child, lang, source); name != "" {
			return name
		}
	}
	return ""
}
