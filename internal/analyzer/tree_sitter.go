package analyzer

import (
	"context"
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func parseTree(ctx context.Context, parser *sitter.Parser, source []byte) (*sitter.Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tree := parser.ParseWithOptions(
		func(offset int, _ sitter.Point) []byte {
			if ctx.Err() != nil {
				return nil
			}
			if offset >= len(source) {
				return nil
			}
			return source[offset:]
		},
		nil,
		&sitter.ParseOptions{
			ProgressCallback: func(_ sitter.ParseState) bool {
				return ctx.Err() != nil
			},
		},
	)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("parse tree")
	}
	return tree, nil
}
