package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// tsParser handles TypeScript (.ts, .tsx, .mts, .cts) files.
// It automatically uses the TSX grammar for .tsx files.
type tsParser struct{}

// jsParser handles JavaScript (.js, .jsx, .mjs, .cjs) files.
type jsParser struct{}

func (p *tsParser) ParseFile(ctx context.Context, filePath string, source []byte) (*Result, error) {
	return parseTSFamily(ctx, filePath, source, "typescript")
}

func (p *jsParser) ParseFile(ctx context.Context, filePath string, source []byte) (*Result, error) {
	return parseTSFamily(ctx, filePath, source, "javascript")
}

// parseTSFamily is the shared parse implementation for all TS/JS variants.
func parseTSFamily(ctx context.Context, path string, source []byte, langLabel string) (*Result, error) {
	parsed, err := parseTree(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse %s tree-sitter source: %w", langLabel, err)
	}
	defer parsed.Close()

	result := &Result{}
	root := parsed.tree.RootNode()
	walkTSNode(root, parsed.lang, source, path, "", result)
	return result, nil
}

// walkTSNode walks the AST recursively, collecting symbols and refs.
// parent is the name of the immediately enclosing class/interface/enum.
func walkTSNode(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch nodeKind(node, lang) {
	case "class_declaration", "abstract_class_declaration":
		nextParent = appendTSClass(node, lang, source, path, parent, result)
	case "interface_declaration":
		nextParent = appendTSInterface(node, lang, source, path, parent, result)
	case "enum_declaration":
		nextParent = appendTSEnum(node, lang, source, path, parent, result)
	case "type_alias_declaration":
		appendTSTypeAlias(node, lang, source, path, parent, result)
	case "function_declaration", "generator_function_declaration":
		appendTSFunction(node, lang, source, path, parent, result)
	case "method_definition":
		appendTSMethod(node, lang, source, path, parent, result)
	case "lexical_declaration", "variable_declaration":
		// Captures: const foo = () => {}  /  const foo = function() {}
		appendTSVariableDecl(node, lang, source, path, parent, result)
	case "import_statement":
		appendTSImport(node, lang, source, path, result)
	case "call_expression":
		appendTSCall(node, lang, source, path, result)
	case "new_expression":
		appendTSNew(node, lang, source, path, result)
	}

	for _, child := range namedChildren(node) {
		walkTSNode(child, lang, source, path, nextParent, result)
	}
}

// ---------- Symbol extractors ----------

func appendTSClass(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return parent
	}
	name := nodeText(nameNode, source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:        name,
		Kind:        "class",
		FilePath:    path,
		Line:        int(nameNode.StartPoint().Row) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		Parent:      parent,
		Description: findTSComment(node, lang, source),
	})
	return name
}

func appendTSInterface(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return parent
	}
	name := nodeText(nameNode, source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:        name,
		Kind:        "interface",
		FilePath:    path,
		Line:        int(nameNode.StartPoint().Row) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		Parent:      parent,
		Description: findTSComment(node, lang, source),
	})
	return name
}

func appendTSEnum(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) string {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return parent
	}
	name := nodeText(nameNode, source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     "enum",
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   parent,
	})
	return name
}

func appendTSTypeAlias(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
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
		Parent:   parent,
	})
}

func appendTSFunction(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:        nodeText(nameNode, source),
		Kind:        "function",
		FilePath:    path,
		Line:        int(nameNode.StartPoint().Row) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		Parent:      parent,
		Description: findTSComment(node, lang, source),
	})
}

func appendTSMethod(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	nameNode := childByFieldName(node, lang, "name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, source)
	kind := "method"
	if name == "constructor" {
		kind = "constructor"
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     kind,
		FilePath: path,
		Line:     int(nameNode.StartPoint().Row) + 1,
		EndLine:  int(node.EndPoint().Row) + 1,
		Parent:   parent,
	})
}

// appendTSVariableDecl captures arrow functions and function expressions assigned
// to const/let/var declarations: `const foo = () => {}` or `const foo = function() {}`.
func appendTSVariableDecl(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path, parent string, result *Result) {
	for _, child := range namedChildren(node) {
		if nodeKind(child, lang) != "variable_declarator" {
			continue
		}
		nameNode := childByFieldName(child, lang, "name")
		valueNode := childByFieldName(child, lang, "value")
		if nameNode == nil || valueNode == nil {
			continue
		}
		kind := ""
		switch nodeKind(valueNode, lang) {
		case "arrow_function", "function_expression", "generator_function_expression":
			kind = "function"
		}
		if kind == "" {
			continue
		}
		result.Symbols = append(result.Symbols, Symbol{
			Name:     tsIdentifierName(nameNode, source),
			Kind:     kind,
			FilePath: path,
			Line:     int(nameNode.StartPoint().Row) + 1,
			EndLine:  int(valueNode.EndPoint().Row) + 1,
			Parent:   parent,
		})
	}
}

// ---------- Ref extractors ----------

func appendTSImport(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, filePath string, result *Result) {
	// import ... from "module-path"
	sourceNode := childByFieldName(node, lang, "source")
	if sourceNode == nil {
		return
	}
	raw := nodeText(sourceNode, source)
	modulePath := strings.Trim(raw, `"'`+"`")
	if modulePath == "" {
		return
	}
	// Extract the imported names from the import clause, defaulting to the module base name.
	names := extractTSImportedNames(node, lang, source, modulePath)
	for _, name := range names {
		result.Refs = append(result.Refs, Ref{
			Name:       name,
			Kind:       "import",
			TargetPath: modulePath,
			FilePath:   filePath,
			Line:       int(sourceNode.StartPoint().Row) + 1,
			Column:     int(sourceNode.StartPoint().Column) + 1,
		})
	}
}

// extractTSImportedNames returns the local names of imported bindings.
// Falls back to the last segment of the module path when no named imports exist.
func extractTSImportedNames(importNode *gotreesitter.Node, lang *gotreesitter.Language, source []byte, modulePath string) []string {
	var names []string
	for _, child := range namedChildren(importNode) {
		if nodeKind(child, lang) == "import_clause" {
			names = append(names, extractFromImportClause(child, lang, source)...)
		}
	}
	if len(names) == 0 {
		// Side-effect import: `import "module"` – use base name as ref target label.
		names = append(names, tsModuleBaseName(modulePath))
	}
	return names
}

func extractFromImportClause(clause *gotreesitter.Node, lang *gotreesitter.Language, source []byte) []string {
	var names []string
	for _, child := range namedChildren(clause) {
		switch nodeKind(child, lang) {
		case "identifier":
			// default import: import Foo from "..."
			names = append(names, nodeText(child, source))
		case "namespace_import":
			// import * as X from "..."
			for _, nc := range namedChildren(child) {
				if nodeKind(nc, lang) == "identifier" {
					names = append(names, nodeText(nc, source))
				}
			}
		case "named_imports":
			// import { A, B as C } from "..."
			names = append(names, extractNamedImports(child, lang, source)...)
		}
	}
	return names
}

func extractNamedImports(namedImports *gotreesitter.Node, lang *gotreesitter.Language, source []byte) []string {
	var names []string
	for _, child := range namedChildren(namedImports) {
		if nodeKind(child, lang) != "import_specifier" {
			continue
		}
		// Prefer `alias` field (local name) if present, otherwise use `name`.
		localNode := childByFieldName(child, lang, "alias")
		if localNode == nil {
			localNode = childByFieldName(child, lang, "name")
		}
		if localNode != nil {
			names = append(names, nodeText(localNode, source))
		}
	}
	return names
}

func appendTSCall(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	fnNode := childByFieldName(node, lang, "function")
	if fnNode == nil {
		return
	}
	name := tsCallName(fnNode, lang, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(fnNode.StartPoint().Row) + 1,
		Column:   int(fnNode.StartPoint().Column) + 1,
	})
}

func appendTSNew(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, path string, result *Result) {
	constructorNode := childByFieldName(node, lang, "constructor")
	if constructorNode == nil {
		return
	}
	name := tsCallName(constructorNode, lang, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(constructorNode.StartPoint().Row) + 1,
		Column:   int(constructorNode.StartPoint().Column) + 1,
	})
}

// ---------- Helpers ----------

// tsCallName extracts the terminal callable name from a function node in a call expression.
// Mirrors the same disambiguation logic used in goCallName and pythonCallName.
func tsCallName(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	switch nodeKind(node, lang) {
	case "identifier":
		return nodeText(node, source)
	case "member_expression":
		// obj.method → return "method"
		propNode := childByFieldName(node, lang, "property")
		if propNode != nil {
			return nodeText(propNode, source)
		}
	case "call_expression":
		// Chained: foo()() → resolve inner
		innerFn := childByFieldName(node, lang, "function")
		if innerFn != nil {
			return tsCallName(innerFn, lang, source)
		}
	}
	// Fallback: last segment after ".".
	text := strings.TrimSpace(nodeText(node, source))
	if text == "" {
		return ""
	}
	if i := strings.LastIndex(text, "."); i >= 0 {
		text = text[i+1:]
	}
	// Strip generic args.
	if i := strings.Index(text, "<"); i >= 0 {
		text = text[:i]
	}
	// Strip call parens.
	if i := strings.Index(text, "("); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// tsIdentifierName extracts the identifier text from a name node.
// For simple identifiers it returns the raw text; for complex patterns (e.g.
// destructuring) it returns the full text as a best-effort fallback.
func tsIdentifierName(node *gotreesitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(nodeText(node, source))
}

// tsModuleBaseName returns the trailing path segment, stripping the file extension.
// Used as a fallback ref label for side-effect imports (`import "module"`).
func tsModuleBaseName(modulePath string) string {
	if i := strings.LastIndex(modulePath, "/"); i >= 0 {
		modulePath = modulePath[i+1:]
	}
	if i := strings.LastIndex(modulePath, "."); i >= 0 {
		modulePath = modulePath[:i]
	}
	return modulePath
}

// findTSComment looks for a JSDoc or single-line comment immediately preceding the node.
func findTSComment(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	prev := prevNamedSibling(node)
	if prev == nil || nodeKind(prev, lang) != "comment" {
		return ""
	}
	// Must be immediately above (no blank lines between).
	if node.StartPoint().Row-prev.EndPoint().Row > 1 {
		return ""
	}
	text := strings.TrimSpace(nodeText(prev, source))
	text = strings.TrimPrefix(text, "/**")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	text = strings.TrimPrefix(text, "//")
	return strings.TrimSpace(text)
}
