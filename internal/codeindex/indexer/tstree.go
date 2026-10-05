package indexer

import (
	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// tsNode adapts a pure-Go gotreesitter node to the small surface the syntax
// walker needs. The language is carried alongside the node so Kind can resolve
// node type names without an explicit language argument at every call site.
//
// This replaces the cgo github.com/tree-sitter/go-tree-sitter binding so the
// indexer keeps building with CGO_ENABLED=0.
type tsNode struct {
	node *gotreesitter.Node
	lang *gotreesitter.Language
}

func wrapNode(node *gotreesitter.Node, lang *gotreesitter.Language) *tsNode {
	if node == nil {
		return nil
	}
	return &tsNode{node: node, lang: lang}
}

func (n *tsNode) Kind() string {
	if n == nil || n.node == nil || n.lang == nil {
		return ""
	}
	return n.node.Type(n.lang)
}

func (n *tsNode) StartByte() uint32 {
	if n == nil || n.node == nil {
		return 0
	}
	return n.node.StartByte()
}

func (n *tsNode) EndByte() uint32 {
	if n == nil || n.node == nil {
		return 0
	}
	return n.node.EndByte()
}

func (n *tsNode) NamedChildCount() int {
	if n == nil || n.node == nil {
		return 0
	}
	return n.node.NamedChildCount()
}

func (n *tsNode) NamedChild(i int) *tsNode {
	if n == nil || n.node == nil {
		return nil
	}
	return wrapNode(n.node.NamedChild(i), n.lang)
}

func (n *tsNode) ChildByFieldName(name string) *tsNode {
	if n == nil || n.node == nil || n.lang == nil {
		return nil
	}
	return wrapNode(n.node.ChildByFieldName(name, n.lang), n.lang)
}

func (n *tsNode) Parent() *tsNode {
	if n == nil || n.node == nil {
		return nil
	}
	return wrapNode(n.node.Parent(), n.lang)
}

// parserLanguage maps a source language to its pure-Go grammar.
func parserLanguage(language string) *gotreesitter.Language {
	name := grammarName(language)
	if name == "" {
		return nil
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		return nil
	}
	return entry.Language()
}

func grammarName(language string) string {
	switch language {
	case "go":
		return "go"
	case "javascript", "jsx":
		return "javascript"
	case "python":
		return "python"
	case "typescript":
		return "typescript"
	case "tsx":
		return "tsx"
	case "csharp":
		return "c_sharp"
	case "c":
		return "c"
	case "cpp":
		return "cpp"
	case "dart":
		return "dart"
	case "php":
		return "php"
	case "rust":
		return "rust"
	default:
		return ""
	}
}
