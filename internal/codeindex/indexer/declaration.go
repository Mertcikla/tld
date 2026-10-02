package indexer

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/odvcencio/gotreesitter"
)

// declarationIndex resolves the full source span of the declaration enclosing a
// SCIP definition occurrence for languages whose indexers omit an enclosing
// range. It parses the source lazily with the same pure-Go grammar the
// tree-sitter declaration walker uses, so no source is parsed unless a
// definition actually needs the fallback.
type declarationIndex struct {
	source *graph.Source
	lang   *gotreesitter.Language
	tree   *gotreesitter.Tree
	root   *tsNode
	tried  bool
}

// newDeclarationIndex prepares a resolver for a source. The source is not parsed
// until span is first called.
func newDeclarationIndex(source *graph.Source) *declarationIndex {
	if source == nil {
		return nil
	}
	return &declarationIndex{source: source, lang: parserLanguage(source.Language)}
}

// span returns the byte span of the smallest code-bearing declaration that
// contains [start, end), widening Dart signatures to their adjacent body. ok is
// false when the language has no grammar, the source does not parse, or no
// enclosing declaration is found.
func (d *declarationIndex) span(start, end int) (int, int, bool) {
	if d == nil || d.lang == nil || start < 0 || end > len(d.source.Text) {
		return 0, 0, false
	}
	if !d.tried {
		d.tried = true
		tree, err := gotreesitter.NewParser(d.lang).Parse(d.source.Text)
		if err != nil || tree == nil {
			return 0, 0, false
		}
		d.tree = tree
		d.root = wrapNode(tree.RootNode(), d.lang)
	}
	if d.root == nil {
		return 0, 0, false
	}
	node := enclosingDeclaration(d.root, d.source.Text, start, end)
	if node == nil {
		return 0, 0, false
	}
	declStart, declEnd := int(node.StartByte()), int(node.EndByte())
	if body := dartBodySibling(node); body != nil {
		if end := int(body.EndByte()); end > declEnd {
			declEnd = end
		}
	}
	if declStart > start || declEnd < end || declStart >= declEnd {
		return 0, 0, false
	}
	return declStart, declEnd, true
}

// Release frees the parsed tree, if the resolver ever parsed one.
func (d *declarationIndex) Release() {
	if d == nil || d.tree == nil {
		return
	}
	d.tree.Release()
	d.tree = nil
	d.root = nil
}

// enclosingDeclaration returns the smallest declaration node whose kind
// nodeFactKind recognizes and whose span contains [start, end).
func enclosingDeclaration(n *tsNode, src []byte, start, end int) *tsNode {
	if n == nil || int(n.StartByte()) > start || int(n.EndByte()) < end {
		return nil
	}
	var best *tsNode
	for i := 0; i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if child == nil || int(child.StartByte()) > start || int(child.EndByte()) < end {
			continue
		}
		if candidate := enclosingDeclaration(child, src, start, end); candidate != nil {
			if best == nil || spanLen(candidate) < spanLen(best) {
				best = candidate
			}
		}
	}
	if best != nil {
		return best
	}
	if nodeFactKind(n, src) != pb.FactKind_FACT_KIND_UNSPECIFIED {
		return n
	}
	return nil
}

func spanLen(n *tsNode) int { return int(n.EndByte() - n.StartByte()) }

// dartBodySibling returns the function body that follows a Dart signature (or
// its method signature wrapper) so the captured declaration includes the body.
func dartBodySibling(n *tsNode) *tsNode {
	if n == nil {
		return nil
	}
	node := n
	if isDartSignature(node.Kind()) {
		if parent := node.Parent(); parent != nil && parent.Kind() == "method_signature" {
			node = parent
		}
	} else if node.Kind() != "method_signature" {
		return nil
	}
	parent := node.Parent()
	if parent == nil {
		return nil
	}
	for i := 0; i < parent.NamedChildCount(); i++ {
		child := parent.NamedChild(i)
		if child == nil || child.StartByte() != node.StartByte() || child.EndByte() != node.EndByte() {
			continue
		}
		for j := i + 1; j < parent.NamedChildCount(); j++ {
			next := parent.NamedChild(j)
			if next == nil {
				continue
			}
			switch next.Kind() {
			case "function_body", "function_expression_body":
				return next
			case "comment", "documentation_comment", "block_comment", "line_comment":
				continue
			}
			return nil
		}
		return nil
	}
	return nil
}

func isDartSignature(kind string) bool {
	switch kind {
	case "function_signature", "getter_signature", "setter_signature", "operator_signature",
		"constructor_signature", "factory_constructor_signature",
		"constant_constructor_signature", "redirecting_factory_constructor_signature":
		return true
	}
	return false
}
