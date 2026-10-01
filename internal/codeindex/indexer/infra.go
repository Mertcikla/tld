package indexer

import (
	"context"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/parser"
)

// addInfraFacts scans configuration and deployment sources and records their
// facts on the graph. Unchanged sources are carried forward from the base
// snapshot so their fact ids stay stable across incremental builds.
func addInfraFacts(ctx context.Context, g *graph.Graph, root string, base *IncrementalBase, carried map[string]string) error {
	scanned, err := parser.Scan(ctx, root, g.Sources)
	if err != nil {
		return err
	}
	keptCarried := map[string]bool{}
	if base != nil {
		for _, id := range carried {
			f := base.Graph.Facts[id]
			if f != nil && isInfraKind(f.Kind) {
				keptCarried[f.LogicalKey] = true
			}
		}
	}
	for _, f := range scanned {
		src := g.Sources[f.Path]
		if src == nil {
			continue
		}
		language := src.Language
		anchor := src.Anchor(byteOffset(src.Text, f.Line, f.Column, f.EndLine, f.EndColumn))
		key := graph.LogicalInfraKey(f.Kind, f.Subject, f.Object, f.Path)
		if keptCarried[key] {
			continue
		}
		g.AddInfraFact(f.Kind, f.Subject, f.Object, language, f.Extractor, anchor, f.Text)
	}
	// Carry forward infra facts whose source was unchanged but which the scan no
	// longer produces (for example an external dependency classification).
	if base != nil {
		for _, id := range carried {
			f := base.Graph.Facts[id]
			if f == nil || !isInfraKind(f.Kind) {
				continue
			}
			if g.FactsByLogicalKey()[f.LogicalKey] != nil {
				continue
			}
			g.AdoptFact(f)
		}
	}
	return nil
}

func isInfraKind(kind pb.FactKind) bool {
	return kind >= pb.FactKind_FACT_KIND_DEPLOYABLE
}

// byteOffset resolves an infra fact's line/column span to byte offsets in the
// captured source. When the end span is unspecified the start position is used
// for both, so the anchor still points at the reporting line.
func byteOffset(text []byte, line, column, endLine, endColumn int) (int, int) {
	start := offsetOf(text, line, column)
	end := start
	if endLine > 0 && endColumn > 0 {
		end = offsetOf(text, endLine, endColumn)
	}
	if end < start {
		end = start
	}
	return start, end
}

func offsetOf(text []byte, line, column int) int {
	if line <= 0 {
		return 0
	}
	if column <= 0 {
		column = 1
	}
	current := 1
	offset := 0
	for offset < len(text) && current < line {
		if text[offset] == '\n' {
			current++
		}
		offset++
	}
	offset += column - 1
	if offset > len(text) {
		offset = len(text)
	}
	return offset
}
