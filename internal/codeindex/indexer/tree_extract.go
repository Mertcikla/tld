package indexer

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/odvcencio/gotreesitter"
)

// treeFacts extracts a source's declarations into the graph with no reuse. It
// remains the cold-start entry point and the focus of the tree-sitter tests.
func treeFacts(ctx context.Context, g *graph.Graph, s *graph.Source) ([]callSite, error) {
	extraction, err := extractFile(ctx, s)
	if err != nil {
		return nil, err
	}
	mergeExtraction(g, s, nil, extraction)
	calls := make([]callSite, 0, len(extraction.Calls))
	calls = append(calls, extraction.Calls...)
	return calls, nil
}

// declExtraction is one declaration discovered by the tree-sitter walk. It is
// produced without touching the graph so unchanged declarations can be reused
// across snapshots by matching their body and context hashes.
type declExtraction struct {
	Kind       pb.FactKind
	Name       string
	Scope      string
	Parent     int
	Start, End int
	Code       string
	Signature  string
	BodyHash   string
}

// fileExtraction is the symbol-level result of parsing one source file.
type fileExtraction struct {
	Decls   []declExtraction
	Imports []string
	Calls   []callSite
}

// extractFile parses a source once and returns its declarations, imports, and
// call sites. No graph mutation happens here, which is what makes symbol-level
// reuse possible: a caller can compare the extraction against a previous cache
// and only rebuild changed declarations.
func extractFile(ctx context.Context, s *graph.Source) (fileExtraction, error) {
	lang := parserLanguage(s.Language)
	if lang == nil {
		return fileExtraction{}, nil
	}
	if err := ctx.Err(); err != nil {
		return fileExtraction{}, err
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
	tree, err := parser.Parse(s.Text)
	if err != nil {
		return fileExtraction{}, err
	}
	if tree == nil {
		return fileExtraction{}, fmt.Errorf("parse canceled for %s", s.Path)
	}
	defer tree.Release()
	root := wrapNode(tree.RootNode(), lang)
	out := fileExtraction{Imports: fileImports(root, s.Text)}
	var nodes []*tsNode
	var walk func(*tsNode)
	walk = func(n *tsNode) {
		if n == nil {
			return
		}
		if nodeFactKind(n, s.Text) != pb.FactKind_FACT_KIND_UNSPECIFIED {
			nodes = append(nodes, n)
		}
		if site, ok := callSiteFor(n, s); ok {
			out.Calls = append(out.Calls, site)
		}
		for i := 0; i < n.NamedChildCount(); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StartByte() == nodes[j].StartByte() {
			return nodes[i].EndByte() > nodes[j].EndByte()
		}
		return nodes[i].StartByte() < nodes[j].StartByte()
	})
	indexBySpan := map[string]int{}
	for _, n := range nodes {
		start, end := attachedComments(s.Text, int(n.StartByte())), int(n.EndByte())
		if end <= start || end > len(s.Text) {
			continue
		}
		name := declarationName(n, s.Text)
		if name == "" {
			continue
		}
		kind := nodeFactKind(n, s.Text)
		parent := -1
		for p := n.Parent(); p != nil; p = p.Parent() {
			if idx, ok := indexBySpan[fmt.Sprintf("%d:%d", p.StartByte(), p.EndByte())]; ok {
				parent = idx
				break
			}
		}
		signature := signatureContext(s.Text, start, end, name)
		decl := declExtraction{
			Kind:      kind,
			Name:      name,
			Scope:     receiverScope(n, s.Text),
			Parent:    parent,
			Start:     start,
			End:       end,
			Code:      string(s.Text[start:end]),
			Signature: signature,
			BodyHash:  graph.BodyHash(s.Text[start:end]),
		}
		indexBySpan[fmt.Sprintf("%d:%d", n.StartByte(), n.EndByte())] = len(out.Decls)
		out.Decls = append(out.Decls, decl)
	}
	return out, nil
}

// Go methods declare their receiver outside the type's syntax subtree.
func receiverScope(n *tsNode, text []byte) string {
	receiver := n.ChildByFieldName("receiver")
	if receiver == nil {
		return ""
	}
	for i := 0; i < receiver.NamedChildCount(); i++ {
		if typ := receiver.NamedChild(i).ChildByFieldName("type"); typ != nil {
			return string(text[typ.StartByte():typ.EndByte()])
		}
	}
	return ""
}

// contextFor returns the context for a declaration: its signature with
// the enclosing declaration's signature prepended, matching legacy behavior.
func contextFor(out fileExtraction, index int) string {
	if index < 0 || index >= len(out.Decls) {
		return ""
	}
	decl := out.Decls[index]
	context := decl.Signature
	if decl.Parent >= 0 && decl.Parent < len(out.Decls) {
		context = out.Decls[decl.Parent].Signature + "\n" + context
	}
	return context
}
