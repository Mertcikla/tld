package analyzer

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

type pythonParser struct{}

func (p *pythonParser) ParseFile(ctx context.Context, path string, source []byte) (*Result, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(sitter.NewLanguage(tree_sitter_python.Language())); err != nil {
		return nil, fmt.Errorf("set python tree-sitter language: %w", err)
	}

	tree, err := parseTree(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("parse python tree-sitter source: %w", err)
	}
	defer tree.Close()

	result := &Result{}
	root := tree.RootNode()
	p.walkNode(root, source, path, "", result)
	return result, nil
}

func (p *pythonParser) walkNode(node *sitter.Node, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch node.Kind() {
	case "function_definition":
		nextParent = p.appendFunction(node, source, path, parent, result)
	case "class_definition":
		nextParent = p.appendClass(node, source, path, parent, result)
	case "import_statement":
		p.appendImport(node, source, path, result)
	case "import_from_statement":
		p.appendImportFrom(node, source, path, result)
	case "decorator":
		p.appendDecorator(node, source, path, result)
	case "call":
		p.appendCall(node, source, path, result)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if !child.IsNamed() {
			continue
		}
		p.walkNode(child, source, path, nextParent, result)
	}
}

func (p *pythonParser) appendClass(node *sitter.Node, source []byte, path, parent string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parent
	}
	name := nameNode.Utf8Text(source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     "class",
		FilePath: path,
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
	return name
}

func (p *pythonParser) appendFunction(node *sitter.Node, source []byte, path, parent string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parent
	}
	name := nameNode.Utf8Text(source)
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
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
	return name
}

func (p *pythonParser) appendImport(node *sitter.Node, source []byte, path string, result *Result) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if !child.IsNamed() {
			continue
		}
		fname := node.FieldNameForChild(uint32(i))
		if fname != "name" {
			continue
		}
		p.processImportName(child, source, path, result)
	}
}

func (p *pythonParser) processImportName(node *sitter.Node, source []byte, path string, result *Result) {
	switch node.Kind() {
	case "dotted_name":
		p.addImportRef(node, source, path, node.Utf8Text(source), result)
	case "aliased_import":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			p.addImportRef(nameNode, source, path, nameNode.Utf8Text(source), result)
		}
	}
}

func (p *pythonParser) addImportRef(node *sitter.Node, source []byte, filePath, targetPath string, result *Result) {
	name := node.Utf8Text(source)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	result.Refs = append(result.Refs, Ref{
		Name:       name,
		Kind:       "import",
		TargetPath: targetPath,
		FilePath:   filePath,
		Line:       int(node.StartPosition().Row) + 1,
		Column:     int(node.StartPosition().Column) + 1,
	})
}

func (p *pythonParser) appendImportFrom(node *sitter.Node, source []byte, path string, result *Result) {
	moduleNode := node.ChildByFieldName("module_name")
	if moduleNode == nil {
		return
	}
	modulePath := moduleNode.Utf8Text(source)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if !child.IsNamed() {
			continue
		}
		fname := node.FieldNameForChild(uint32(i))
		if fname != "name" {
			continue
		}
		p.addImportRef(child, source, path, modulePath, result)
	}
}

func (p *pythonParser) appendDecorator(node *sitter.Node, source []byte, path string, result *Result) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if !child.IsNamed() {
			continue
		}
		if child.Kind() == "identifier" || child.Kind() == "attribute" {
			name := pythonCallName(child, source)
			if name != "" {
				result.Refs = append(result.Refs, Ref{
					Name:     name,
					Kind:     "call",
					FilePath: path,
					Line:     int(child.StartPosition().Row) + 1,
					Column:   int(child.StartPosition().Column) + 1,
				})
			}
		}
	}
}

func (p *pythonParser) appendCall(node *sitter.Node, source []byte, path string, result *Result) {
	functionNode := node.ChildByFieldName("function")
	if functionNode == nil {
		return
	}
	name := pythonCallName(functionNode, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(functionNode.StartPosition().Row) + 1,
		Column:   int(functionNode.StartPosition().Column) + 1,
	})
}

func pythonCallName(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier":
		return node.Utf8Text(source)
	case "attribute":
		attributeNode := node.ChildByFieldName("attribute")
		if attributeNode != nil {
			return attributeNode.Utf8Text(source)
		}
	case "call":
		callNode := node.ChildByFieldName("function")
		if callNode != nil {
			return pythonCallName(callNode, source)
		}
	}
	text := strings.TrimSpace(node.Utf8Text(source))
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
