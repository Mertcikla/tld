package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

type rustParser struct{}

func (p *rustParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parsed, err := parseTree(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse rust tree-sitter source: %w", err)
	}
	defer parsed.Close()

	result := &Result{}
	root := parsed.tree.RootNode()
	p.walkNode(root, parsed.lang, source, path, "", result)
	return result, nil
}

func (p *rustParser) walkNode(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	kind := nodeKind(node, lang)
	switch kind {
	case "function_item", "function_signature_item":
		nextParent = p.appendFunction(node, lang, source, path, parent, result)
	case "struct_item":
		nextParent = p.appendSymbol(node, lang, source, path, parent, "struct", result)
	case "enum_item":
		nextParent = p.appendSymbol(node, lang, source, path, parent, "enum", result)
	case "trait_item":
		nextParent = p.appendSymbol(node, lang, source, path, parent, "trait", result)
	case "mod_item":
		nextParent = p.appendSymbol(node, lang, source, path, parent, "module", result)
	case "impl_item":
		nextParent = p.handleImpl(node, lang, source, path, parent, result)
	case "type_item":
		nextParent = p.appendSymbol(node, lang, source, path, parent, "type", result)
	case "use_declaration":
		p.appendUse(node, lang, source, path, result)
	case "call_expression":
		p.appendCall(node, lang, source, path, result)
	case "macro_invocation":
		p.appendMacro(node, lang, source, path, result)
	case "struct_expression":
		p.appendStructExpr(node, lang, source, path, result)
	}

	for _, child := range namedChildren(node) {
		p.walkNode(child, lang, source, path, nextParent, result)
	}
}

func (p *rustParser) handleImpl(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	// impl Point { ... } or impl Drawable for Point { ... }
	// We want the type name as the parent for methods inside.
	typeNode := childByFieldName(node, lang, "type")
	if typeNode == nil {
		// Fallback to type_identifier
		for _, child := range namedChildren(node) {
			if nodeKind(child, lang) == "type_identifier" {
				typeNode = child
				break
			}
		}
	}
	if typeNode != nil {
		return nodeText(typeNode, source)
	}
	return parent
}

func (p *rustParser) appendFunction(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		// Fallback to first identifier child if name field is missing
		for _, child := range namedChildren(node) {
			if nodeKind(child, lang) == "identifier" {
				nameNode = child
				break
			}
		}
	}
	if nameNode == nil {
		return parent
	}

	name := nodeText(nameNode, source)
	kind := "function"
	// If parent is an impl or trait, it's a method
	if parent != "" {
		kind = "method"
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

func (p *rustParser) appendSymbol(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent, kind string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		// Fallback to first identifier or type_identifier
		for _, child := range namedChildren(node) {
			k := nodeKind(child, lang)
			if k == "identifier" || k == "type_identifier" {
				nameNode = child
				break
			}
		}
	}
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

func (p *rustParser) appendUse(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	argumentNode := childByFieldName(node, lang, "argument")
	if argumentNode == nil {
		// Fallback to searching children
		for _, child := range namedChildren(node) {
			k := nodeKind(child, lang)
			if k == "scoped_identifier" || k == "identifier" || k == "use_list" || k == "scoped_use_list" || k == "use_as_clause" {
				argumentNode = child
				break
			}
		}
	}
	if argumentNode == nil {
		return
	}

	p.processUseArgument(argumentNode, lang, source, path, "", result)
}

func (p *rustParser) processUseArgument(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, prefix string, result *Result) {
	kind := nodeKind(node, lang)
	switch kind {
	case "identifier", "scoped_identifier":
		targetPath := nodeText(node, source)
		if prefix != "" {
			targetPath = prefix + "::" + targetPath
		}
		name := targetPath
		if idx := strings.LastIndex(targetPath, "::"); idx >= 0 {
			name = targetPath[idx+2:]
		}
		result.Refs = append(result.Refs, Ref{
			Name:       name,
			Kind:       "import",
			TargetPath: targetPath,
			FilePath:   path,
			Line:       int(node.StartPoint().Row) + 1,
			Column:     int(node.StartPoint().Column) + 1,
		})
	case "use_list":
		for _, child := range namedChildren(node) {
			p.processUseArgument(child, lang, source, path, prefix, result)
		}
	case "scoped_use_list":
		newPrefix := ""
		// [scoped_identifier] [::] [use_list]
		if node.ChildCount() >= 3 {
			newPrefix = nodeText(node.Child(0), source)
			if prefix != "" {
				newPrefix = prefix + "::" + newPrefix
			}
			p.processUseArgument(node.Child(2), lang, source, path, newPrefix, result)
		}
	case "use_as_clause":
		aliasNode := childByFieldName(node, lang, "alias")
		pathNode := childByFieldName(node, lang, "path")
		if aliasNode != nil && pathNode != nil {
			targetPath := nodeText(pathNode, source)
			if prefix != "" {
				targetPath = prefix + "::" + targetPath
			}
			result.Refs = append(result.Refs, Ref{
				Name:       nodeText(aliasNode, source),
				Kind:       "import",
				TargetPath: targetPath,
				FilePath:   path,
				Line:       int(aliasNode.StartPoint().Row) + 1,
				Column:     int(aliasNode.StartPoint().Column) + 1,
			})
		}
	case "self":
		if prefix != "" {
			targetPath := prefix
			name := targetPath
			if idx := strings.LastIndex(targetPath, "::"); idx >= 0 {
				name = targetPath[idx+2:]
			}
			result.Refs = append(result.Refs, Ref{
				Name:       name,
				Kind:       "import",
				TargetPath: targetPath,
				FilePath:   path,
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column) + 1,
			})
		}
	}
}

func (p *rustParser) appendCall(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	functionNode := childByFieldName(node, lang, "function")
	if functionNode == nil {
		return
	}
	name := rustCallName(functionNode, lang, source)
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

func (p *rustParser) appendMacro(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	nameNode := childByFieldName(node, lang, "macro")
	if nameNode == nil {
		// First child is usually the identifier for macros
		for _, child := range namedChildren(node) {
			if nodeKind(child, lang) == "identifier" {
				nameNode = child
				break
			}
		}
	}
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, source)
	result.Refs = append(result.Refs, Ref{
		Name:     name + "!",
		Kind:     "call",
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Column:   int(nameNode.StartPoint().Column) + 1,
	})
}

func (p *rustParser) appendStructExpr(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		// Fallback
		for _, child := range namedChildren(node) {
			k := nodeKind(child, lang)
			if k == "type_identifier" || k == "scoped_type_identifier" || k == "generic_type" {
				nameNode = child
				break
			}
		}
	}
	if nameNode == nil {
		return
	}
	name := rustCallName(nameNode, lang, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		Column:   int(nameNode.StartPoint().Column) + 1,
	})
}

func rustCallName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	kind := nodeKind(node, lang)
	switch kind {
	case "identifier", "field_identifier", "type_identifier":
		return nodeText(node, source)
	case "scoped_identifier", "scoped_type_identifier":
		nameNode := childByFieldName(node, lang, "name")
		if nameNode != nil {
			return nodeText(nameNode, source)
		}
		// Fallback to last identifier
		children := namedChildren(node)
		if len(children) > 0 {
			return rustCallName(children[len(children)-1], lang, source)
		}
	case "field_expression":
		fieldNode := childByFieldName(node, lang, "field")
		if fieldNode != nil {
			return nodeText(fieldNode, source)
		}
	case "generic_type":
		typeNode := childByFieldName(node, lang, "type")
		if typeNode != nil {
			return rustCallName(typeNode, lang, source)
		}
	}
	text := strings.TrimSpace(nodeText(node, source))
	if text == "" {
		return ""
	}
	if index := strings.LastIndex(text, "::"); index >= 0 {
		text = text[index+2:]
	}
	if index := strings.LastIndex(text, "."); index >= 0 {
		text = text[index+1:]
	}
	if index := strings.Index(text, "<"); index >= 0 {
		text = text[:index]
	}
	if index := strings.Index(text, "("); index >= 0 {
		text = text[:index]
	}
	return strings.TrimSpace(text)
}
