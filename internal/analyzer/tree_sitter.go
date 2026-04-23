package analyzer

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type parsedTree struct {
	entry *grammars.LangEntry
	lang  *gotreesitter.Language
	tree  *gotreesitter.Tree
}

func parseTree(ctx context.Context, path string, source []byte) (*parsedTree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entry := grammars.DetectLanguage(path)
	if entry == nil {
		return nil, unsupportedLanguageError(path, "")
	}

	lang := entry.Language()
	if lang == nil {
		return nil, fmt.Errorf("load %s grammar", entry.Name)
	}

	parser := gotreesitter.NewParser(lang)
	var cancellationFlag uint32
	parser.SetCancellationFlag(&cancellationFlag)

	done := make(chan struct{})
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				atomic.StoreUint32(&cancellationFlag, 1)
			case <-done:
			}
		}()
	}
	defer close(done)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
	} else {
		tree, err = parser.Parse(source)
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		if tree != nil {
			tree.Release()
		}
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("parse tree")
	}
	return &parsedTree{
		entry: entry,
		lang:  lang,
		tree:  tree,
	}, nil
}

func (p *parsedTree) Close() {
	if p == nil || p.tree == nil {
		return
	}
	p.tree.Release()
}

func nodeKind(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil || lang == nil {
		return ""
	}
	return node.Type(lang)
}

func nodeText(node *gotreesitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return node.Text(source)
}

func childByFieldName(node *gotreesitter.Node, lang *gotreesitter.Language, fieldName string) *gotreesitter.Node {
	if node == nil || lang == nil {
		return nil
	}
	return node.ChildByFieldName(fieldName, lang)
}

func fieldNameForChild(node *gotreesitter.Node, lang *gotreesitter.Language, childIndex int) string {
	if node == nil || lang == nil {
		return ""
	}
	return node.FieldNameForChild(childIndex, lang)
}

func namedChildren(node *gotreesitter.Node) []*gotreesitter.Node {
	if node == nil {
		return nil
	}
	children := make([]*gotreesitter.Node, 0, node.NamedChildCount())
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || !child.IsNamed() {
			continue
		}
		children = append(children, child)
	}
	return children
}

func prevNamedSibling(node *gotreesitter.Node) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	for sibling := node.PrevSibling(); sibling != nil; sibling = sibling.PrevSibling() {
		if sibling.IsNamed() {
			return sibling
		}
	}
	return nil
}
