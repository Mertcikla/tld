package indexer

import (
	"context"

	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/parser"
)

// addInfraFacts recomputes classifications across configuration sources so
// changes to one manifest cannot leave stale facts in another file.
func addInfraFacts(ctx context.Context, g *graph.Graph, root string) error {
	scanned, err := parser.Scan(ctx, root, g.Sources)
	if err != nil {
		return err
	}
	for _, f := range scanned {
		src := g.Sources[f.Path]
		if src == nil {
			continue
		}
		start, end := src.OffsetForLineColumn(f.Line, f.Column, f.EndLine, f.EndColumn)
		g.AddInfraFact(f.Kind, f.Subject, f.Object, src.Language, f.Extractor, src.Anchor(start, end), f.Text)
	}
	return nil
}

// byteOffset resolves an infra fact's line/column span to byte offsets in the
// captured source. When the end span is unspecified the start position is used
// for both, so the anchor still points at the reporting line.
func byteOffset(text []byte, line, column, endLine, endColumn int) (int, int) {
	return (&graph.Source{Text: text}).OffsetForLineColumn(line, column, endLine, endColumn)
}
