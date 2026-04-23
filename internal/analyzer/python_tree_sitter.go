package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

type pythonParser struct{}

func (p *pythonParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parsed, err := parseTree(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse python tree-sitter source: %w", err)
	}
	defer parsed.Close()

	result := &Result{}
	root := parsed.tree.RootNode()
	p.walkNode(root, parsed.lang, source, path, "", result)
	return result, nil
}

func (p *pythonParser) walkNode(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch nodeKind(node, lang) {
	case "function_definition":
		nextParent = p.appendFunction(node, lang, source, path, parent, result)
	case "class_definition":
		nextParent = p.appendClass(node, lang, source, path, parent, result)
	case "import_statement":
		p.appendImport(node, lang, source, path, result)
	case "import_from_statement":
		p.appendImportFrom(node, lang, source, path, result)
	case "decorator":
		p.appendDecorator(node, lang, source, path, result)
	case "call":
		p.appendCall(node, lang, source, path, result)
	}

	for _, child := range namedChildren(node) {
		p.walkNode(child, lang, source, path, nextParent, result)
	}
}

func (p *pythonParser) appendClass(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return parent
	}
	name := nodeText(nameNode, source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     "class",
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   parent,
	})
	return name
}

func (p *pythonParser) appendFunction(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return parent
	}
	name := nodeText(nameNode, source)
	kind := "function"
	if parent != "" {
		kind = "method"
		if name == "__init__" {
			kind = "constructor"
		}
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

func (p *pythonParser) appendImport(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || !child.IsNamed() {
			continue
		}
		fname := fieldNameForChild(node, lang, i)
		if fname != "name" {
			continue
		}
		p.processImportName(child, lang, source, path, result)
	}
}

func (p *pythonParser) processImportName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	switch nodeKind(node, lang) {
	case "dotted_name":
		p.addImportRef(node, source, path, nodeText(node, source), result)
	case "aliased_import":
		nameNode := childByFieldName(node, lang, "name")
		if nameNode != nil {
			p.addImportRef(nameNode, source, path, nodeText(nameNode, source), result)
		}
	}
}

func (p *pythonParser) addImportRef(node *gotreesitter.Node, source []byte, filePath, targetPath string, result *Result) {
	name := nodeText(node, source)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	result.Refs = append(result.Refs, Ref{
		Name:       name,
		Kind:       "import",
		TargetPath: targetPath,
		FilePath:   filePath,
		Line:       int(node.StartPoint().Row) + 1,
		Column:     int(node.StartPoint().Column) + 1,
	})
}

func (p *pythonParser) appendImportFrom(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	moduleNode := childByFieldName(node, lang, "module_name")
	if moduleNode == nil {
		return
	}
	modulePath := nodeText(moduleNode, source)
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || !child.IsNamed() {
			continue
		}
		fname := fieldNameForChild(node, lang, i)
		if fname != "name" {
			continue
		}
		p.addImportRef(child, source, path, modulePath, result)
	}
}

func (p *pythonParser) appendDecorator(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	for _, child := range namedChildren(node) {
		if nodeKind(child, lang) == "identifier" || nodeKind(child, lang) == "attribute" {
			name := pythonCallName(child, lang, source)
			if name != "" {
				result.Refs = append(result.Refs, Ref{
					Name:     name,
					Kind:     "call",
					FilePath: path,
					Line:     int(child.StartPoint().Row) + 1,
					Column:   int(child.StartPoint().Column) + 1,
				})
			}
		}
	}
}

func (p *pythonParser) appendCall(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	functionNode := childByFieldName(node, lang, "function")
	if functionNode == nil {
		return
	}
	name := pythonCallName(functionNode, lang, source)
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

func pythonCallName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	switch nodeKind(node, lang) {
	case "identifier":
		return nodeText(node, source)
	case "attribute":
		attributeNode := childByFieldName(node, lang, "attribute")
		if attributeNode != nil {
			return nodeText(attributeNode, source)
		}
	case "call":
		callNode := childByFieldName(node, lang, "function")
		if callNode != nil {
			return pythonCallName(callNode, lang, source)
		}
	}
	text := strings.TrimSpace(nodeText(node, source))
	if text == "" {
		return ""
	}
	if index := strings.LastIndex(text, "."); index >= 0 {
		text = text[index+1:]
	}
	if index := strings.Index(text, "("); index >= 0 {
		text = text[:index]
	}
	return strings.TrimSpace(text)
}
