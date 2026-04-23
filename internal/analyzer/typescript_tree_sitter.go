package analyzer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// tsParser handles TypeScript (.ts, .tsx, .mts, .cts) files.
// It automatically uses the TSX grammar for .tsx files.
type tsParser struct{}

// jsParser handles JavaScript (.js, .jsx, .mjs, .cjs) files.
type jsParser struct{}

func (p *tsParser) ParseFile(ctx context.Context, filePath string, source []byte) (*Result, error) {
	var lang *sitter.Language
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".tsx" {
		lang = sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	} else {
		lang = sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	}
	return parseTSFamily(ctx, filePath, source, lang, "typescript")
}

func (p *jsParser) ParseFile(ctx context.Context, filePath string, source []byte) (*Result, error) {
	lang := sitter.NewLanguage(tree_sitter_javascript.Language())
	return parseTSFamily(ctx, filePath, source, lang, "javascript")
}

// parseTSFamily is the shared parse implementation for all TS/JS variants.
func parseTSFamily(ctx context.Context, path string, source []byte, lang *sitter.Language, langLabel string) (*Result, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("set %s tree-sitter language: %w", langLabel, err)
	}

	tree, err := parseTree(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("parse %s tree-sitter source: %w", langLabel, err)
	}
	defer tree.Close()

	result := &Result{}
	root := tree.RootNode()
	walkTSNode(root, source, path, "", result)
	return result, nil
}

// walkTSNode walks the AST recursively, collecting symbols and refs.
// parent is the name of the immediately enclosing class/interface/enum.
func walkTSNode(node *sitter.Node, source []byte, path, parent string, result *Result) {
	if node == nil {
		return
	}

	nextParent := parent
	switch node.Kind() {
	case "class_declaration", "abstract_class_declaration":
		nextParent = appendTSClass(node, source, path, parent, result)
	case "interface_declaration":
		nextParent = appendTSInterface(node, source, path, parent, result)
	case "enum_declaration":
		nextParent = appendTSEnum(node, source, path, parent, result)
	case "type_alias_declaration":
		appendTSTypeAlias(node, source, path, parent, result)
	case "function_declaration", "generator_function_declaration":
		appendTSFunction(node, source, path, parent, result)
	case "method_definition":
		appendTSMethod(node, source, path, parent, result)
	case "lexical_declaration", "variable_declaration":
		// Captures: const foo = () => {}  /  const foo = function() {}
		appendTSVariableDecl(node, source, path, parent, result)
	case "import_statement":
		appendTSImport(node, source, path, result)
	case "call_expression":
		appendTSCall(node, source, path, result)
	case "new_expression":
		appendTSNew(node, source, path, result)
	}

	cursor := node.Walk()
	defer cursor.Close()
	for _, child := range node.NamedChildren(cursor) {
		childCopy := child
		walkTSNode(&childCopy, source, path, nextParent, result)
	}
}

// ---------- Symbol extractors ----------

func appendTSClass(node *sitter.Node, source []byte, path, parent string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parent
	}
	name := nameNode.Utf8Text(source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:        name,
		Kind:        "class",
		FilePath:    path,
		Line:        int(nameNode.StartPosition().Row) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		Parent:      parent,
		Description: findTSComment(node, source),
	})
	return name
}

func appendTSInterface(node *sitter.Node, source []byte, path, parent string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parent
	}
	name := nameNode.Utf8Text(source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:        name,
		Kind:        "interface",
		FilePath:    path,
		Line:        int(nameNode.StartPosition().Row) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		Parent:      parent,
		Description: findTSComment(node, source),
	})
	return name
}

func appendTSEnum(node *sitter.Node, source []byte, path, parent string, result *Result) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parent
	}
	name := nameNode.Utf8Text(source)
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     "enum",
		FilePath: path,
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
	return name
}

func appendTSTypeAlias(node *sitter.Node, source []byte, path, parent string, result *Result) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     nameNode.Utf8Text(source),
		Kind:     "type",
		FilePath: path,
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
}

func appendTSFunction(node *sitter.Node, source []byte, path, parent string, result *Result) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:        nameNode.Utf8Text(source),
		Kind:        "function",
		FilePath:    path,
		Line:        int(nameNode.StartPosition().Row) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		Parent:      parent,
		Description: findTSComment(node, source),
	})
}

func appendTSMethod(node *sitter.Node, source []byte, path, parent string, result *Result) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Utf8Text(source)
	kind := "method"
	if name == "constructor" {
		kind = "constructor"
	}
	result.Symbols = append(result.Symbols, Symbol{
		Name:     name,
		Kind:     kind,
		FilePath: path,
		Line:     int(nameNode.StartPosition().Row) + 1,
		EndLine:  int(node.EndPosition().Row) + 1,
		Parent:   parent,
	})
}

// appendTSVariableDecl captures arrow functions and function expressions assigned
// to const/let/var declarations: `const foo = () => {}` or `const foo = function() {}`.
func appendTSVariableDecl(node *sitter.Node, source []byte, path, parent string, result *Result) {
	cursor := node.Walk()
	defer cursor.Close()
	for _, child := range node.NamedChildren(cursor) {
		if child.Kind() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		valueNode := child.ChildByFieldName("value")
		if nameNode == nil || valueNode == nil {
			continue
		}
		kind := ""
		switch valueNode.Kind() {
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
			Line:     int(nameNode.StartPosition().Row) + 1,
			EndLine:  int(valueNode.EndPosition().Row) + 1,
			Parent:   parent,
		})
	}
}

// ---------- Ref extractors ----------

func appendTSImport(node *sitter.Node, source []byte, filePath string, result *Result) {
	// import ... from "module-path"
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return
	}
	raw := sourceNode.Utf8Text(source)
	modulePath := strings.Trim(raw, `"'`+"`")
	if modulePath == "" {
		return
	}
	// Extract the imported names from the import clause, defaulting to the module base name.
	names := extractTSImportedNames(node, source, modulePath)
	for _, name := range names {
		result.Refs = append(result.Refs, Ref{
			Name:       name,
			Kind:       "import",
			TargetPath: modulePath,
			FilePath:   filePath,
			Line:       int(sourceNode.StartPosition().Row) + 1,
			Column:     int(sourceNode.StartPosition().Column) + 1,
		})
	}
}

// extractTSImportedNames returns the local names of imported bindings.
// Falls back to the last segment of the module path when no named imports exist.
func extractTSImportedNames(importNode *sitter.Node, source []byte, modulePath string) []string {
	cursor := importNode.Walk()
	defer cursor.Close()
	var names []string
	for _, child := range importNode.NamedChildren(cursor) {
		if child.Kind() == "import_clause" {
			childCopy := child
			names = append(names, extractFromImportClause(&childCopy, source)...)
		}
	}
	if len(names) == 0 {
		// Side-effect import: `import "module"` – use base name as ref target label.
		names = append(names, tsModuleBaseName(modulePath))
	}
	return names
}

func extractFromImportClause(clause *sitter.Node, source []byte) []string {
	cursor := clause.Walk()
	defer cursor.Close()
	var names []string
	for _, child := range clause.NamedChildren(cursor) {
		switch child.Kind() {
		case "identifier":
			// default import: import Foo from "..."
			names = append(names, child.Utf8Text(source))
		case "namespace_import":
			// import * as X from "..."
			childCopy := child
			aliasCursor := childCopy.Walk()
			for _, nc := range childCopy.NamedChildren(aliasCursor) {
				if nc.Kind() == "identifier" {
					names = append(names, nc.Utf8Text(source))
				}
			}
			aliasCursor.Close()
		case "named_imports":
			// import { A, B as C } from "..."
			childCopy := child
			names = append(names, extractNamedImports(&childCopy, source)...)
		}
	}
	return names
}

func extractNamedImports(namedImports *sitter.Node, source []byte) []string {
	cursor := namedImports.Walk()
	defer cursor.Close()
	var names []string
	for _, child := range namedImports.NamedChildren(cursor) {
		if child.Kind() != "import_specifier" {
			continue
		}
		// Prefer `alias` field (local name) if present, otherwise use `name`.
		localNode := child.ChildByFieldName("alias")
		if localNode == nil {
			localNode = child.ChildByFieldName("name")
		}
		if localNode != nil {
			names = append(names, localNode.Utf8Text(source))
		}
	}
	return names
}

func appendTSCall(node *sitter.Node, source []byte, path string, result *Result) {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return
	}
	name := tsCallName(fnNode, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(fnNode.StartPosition().Row) + 1,
		Column:   int(fnNode.StartPosition().Column) + 1,
	})
}

func appendTSNew(node *sitter.Node, source []byte, path string, result *Result) {
	constructorNode := node.ChildByFieldName("constructor")
	if constructorNode == nil {
		return
	}
	name := tsCallName(constructorNode, source)
	if name == "" {
		return
	}
	result.Refs = append(result.Refs, Ref{
		Name:     name,
		Kind:     "call",
		FilePath: path,
		Line:     int(constructorNode.StartPosition().Row) + 1,
		Column:   int(constructorNode.StartPosition().Column) + 1,
	})
}

// ---------- Helpers ----------

// tsCallName extracts the terminal callable name from a function node in a call expression.
// Mirrors the same disambiguation logic used in goCallName and pythonCallName.
func tsCallName(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier":
		return node.Utf8Text(source)
	case "member_expression":
		// obj.method → return "method"
		propNode := node.ChildByFieldName("property")
		if propNode != nil {
			return propNode.Utf8Text(source)
		}
	case "call_expression":
		// Chained: foo()() → resolve inner
		innerFn := node.ChildByFieldName("function")
		if innerFn != nil {
			return tsCallName(innerFn, source)
		}
	}
	// Fallback: last segment after ".".
	text := strings.TrimSpace(node.Utf8Text(source))
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
func tsIdentifierName(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.Utf8Text(source))
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
func findTSComment(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	prev := node.PrevNamedSibling()
	if prev == nil || prev.Kind() != "comment" {
		return ""
	}
	// Must be immediately above (no blank lines between).
	if node.StartPosition().Row-prev.EndPosition().Row > 1 {
		return ""
	}
	text := strings.TrimSpace(prev.Utf8Text(source))
	text = strings.TrimPrefix(text, "/**")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	text = strings.TrimPrefix(text, "//")
	return strings.TrimSpace(text)
}
