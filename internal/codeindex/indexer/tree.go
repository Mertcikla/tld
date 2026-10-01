package indexer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/odvcencio/gotreesitter"
)

func treeFacts(ctx context.Context, g *graph.Graph, s *graph.Source) ([]callSite, error) {
	lang := parserLanguage(s.Language)
	if lang == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("parse canceled for %s", s.Path)
	}
	defer tree.Release()
	imports := fileImports(wrapNode(tree.RootNode(), lang), s.Text)
	var nodes []*tsNode
	var calls []callSite
	var walk func(*tsNode)
	walk = func(n *tsNode) {
		if n == nil {
			return
		}
		if nodeFactKind(n, s.Text) != pb.FactKind_FACT_KIND_UNSPECIFIED {
			nodes = append(nodes, n)
		}
		if site, ok := callSiteFor(n, s); ok {
			calls = append(calls, site)
		}
		for i := 0; i < n.NamedChildCount(); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(wrapNode(tree.RootNode(), lang))
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StartByte() == nodes[j].StartByte() {
			return nodes[i].EndByte() > nodes[j].EndByte()
		}
		return nodes[i].StartByte() < nodes[j].StartByte()
	})
	created := map[string]*pb.CodeFact{}
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
		anchor := s.Anchor(start, end)
		code := string(s.Text[start:end])
		signature := signatureContext(s.Text, start, end, name)
		f := g.AddFact(kind, name, s.Language, anchor, code, signature, &pb.Evidence{Producer: "tree-sitter", OriginalId: n.Kind(), OriginalRange: fmt.Sprintf("%d:%d", start, end), Anchor: anchor})
		if len(imports) > 0 {
			f.Imports = imports
		}
		created[fmt.Sprintf("%d:%d", n.StartByte(), n.EndByte())] = f
		for parent := n.Parent(); parent != nil; parent = parent.Parent() {
			if candidate := created[fmt.Sprintf("%d:%d", parent.StartByte(), parent.EndByte())]; candidate != nil {
				f.ParentFactId = candidate.Id
				break
			}
		}
		context := signature
		if f.ParentFactId != "" {
			context = g.Facts[f.ParentFactId].Signature + "\n" + context
		}
		ranges := splitDeclaration(s, n, start, end, 4096, 8192)
		for i, r := range ranges {
			chunkAnchor := s.Anchor(r[0], r[1])
			g.AddChunk(f.Id, chunkAnchor, string(s.Text[r[0]:r[1]]), context, uint32(i), uint32(len(ranges)))
		}
	}
	return calls, nil
}

// callSiteFor records a call expression found during the declaration walk so
// deriveCalls can resolve it against SCIP references without re-parsing.
func callSiteFor(n *tsNode, s *graph.Source) (callSite, bool) {
	isCall := n.Kind() == "call_expression" || (s.Language == "python" && n.Kind() == "call")
	if !isCall {
		return callSite{}, false
	}
	callee := n.ChildByFieldName("function")
	if callee == nil {
		return callSite{}, false
	}
	return callSite{path: s.Path, start: uint32(n.StartByte()), end: uint32(n.EndByte()), calleeStart: uint32(callee.StartByte()), calleeEnd: uint32(callee.EndByte())}, true
}
func nodeFactKind(n *tsNode, src []byte) pb.FactKind {
	kind := n.Kind()
	switch kind {
	case "function_declaration", "generator_function_declaration":
		return pb.FactKind_FACT_KIND_FUNCTION
	case "function_definition":
		if pythonMethod(n) {
			return pb.FactKind_FACT_KIND_METHOD
		}
		return pb.FactKind_FACT_KIND_FUNCTION
	case "class_definition":
		return pb.FactKind_FACT_KIND_CLASS
	case "decorated_definition":
		if definition := n.ChildByFieldName("definition"); definition != nil {
			return nodeFactKind(definition, src)
		}
	case "method_declaration", "method_definition":
		if declarationName(n, src) == "constructor" {
			return pb.FactKind_FACT_KIND_CONSTRUCTOR
		}
		return pb.FactKind_FACT_KIND_METHOD
	case "class_declaration":
		return pb.FactKind_FACT_KIND_CLASS
	case "interface_declaration":
		return pb.FactKind_FACT_KIND_INTERFACE
	case "enum_declaration":
		return pb.FactKind_FACT_KIND_ENUM
	case "type_alias_declaration":
		return pb.FactKind_FACT_KIND_TYPE
	case "type_spec":
		code := string(src[n.StartByte():n.EndByte()])
		if strings.Contains(code, "struct {") || strings.Contains(code, "struct{") {
			return pb.FactKind_FACT_KIND_STRUCT
		}
		if strings.Contains(code, "interface {") || strings.Contains(code, "interface{") {
			return pb.FactKind_FACT_KIND_INTERFACE
		}
		return pb.FactKind_FACT_KIND_TYPE
	case "lexical_declaration", "variable_declaration":
		code := string(src[n.StartByte():n.EndByte()])
		if strings.Contains(code, "=>") || strings.Contains(code, "= function") {
			return pb.FactKind_FACT_KIND_FUNCTION
		}
	}
	return pb.FactKind_FACT_KIND_UNSPECIFIED
}

// pythonMethod reports whether a function definition is a method: its nearest
// enclosing definition is a class rather than another function.
func pythonMethod(n *tsNode) bool {
	for parent := n.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "class_definition":
			return true
		case "function_definition", "module":
			return false
		}
	}
	return false
}

func declarationName(n *tsNode, src []byte) string {
	if n.Kind() == "decorated_definition" {
		if definition := n.ChildByFieldName("definition"); definition != nil {
			return declarationName(definition, src)
		}
	}
	if child := n.ChildByFieldName("name"); child != nil {
		return string(src[child.StartByte():child.EndByte()])
	}
	if n.Kind() == "lexical_declaration" || n.Kind() == "variable_declaration" {
		for i := 0; i < n.NamedChildCount(); i++ {
			child := n.NamedChild(i)
			if child.Kind() == "variable_declarator" {
				if name := child.ChildByFieldName("name"); name != nil {
					return string(src[name.StartByte():name.EndByte()])
				}
			}
		}
	}
	return ""
}
func attachedComments(src []byte, start int) int {
	if start <= 0 {
		return start
	}
	lineStart := start
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}
	at := lineStart
	for at > 0 {
		prevEnd := at - 1
		if prevEnd > 0 && src[prevEnd] == '\n' {
			prevEnd--
		}
		prevStart := prevEnd
		for prevStart > 0 && src[prevStart-1] != '\n' {
			prevStart--
		}
		line := strings.TrimSpace(string(src[prevStart:at]))
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") ||
			strings.HasPrefix(line, "*") || strings.HasPrefix(line, "#") {
			at = prevStart
			continue
		}
		break
	}
	if at < lineStart {
		return at
	}
	return start
}
func signatureContext(src []byte, start, end int, name string) string {
	b := src[start:end]
	cut := len(b)
	if p := strings.IndexByte(string(b), '{'); p >= 0 && p < cut {
		cut = p
	}
	if p := strings.IndexByte(string(b), '\n'); p >= 0 && p < cut {
		cut = p
	}
	if cut > 240 {
		cut = 240
	}
	t := strings.TrimSpace(string(b[:cut]))
	if t == "" {
		return name
	}
	return t
}
func splitDeclaration(s *graph.Source, n *tsNode, start, end, target, max int) [][2]int {
	if end-start <= max {
		return [][2]int{{start, end}}
	}
	var boundaries []int
	var collect func(*tsNode)
	collect = func(node *tsNode) {
		for i := 0; i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if int(child.EndByte()) > start && int(child.EndByte()) < end {
				boundaries = append(boundaries, int(child.EndByte()))
			}
			if uint(child.EndByte()-child.StartByte()) > uint(target) {
				collect(child)
			}
		}
	}
	collect(n)
	sort.Ints(boundaries)
	var out [][2]int
	for start < end {
		limit := start + max
		if limit >= end {
			out = append(out, [2]int{start, end})
			break
		}
		targetEnd := start + target
		cut := 0
		for _, b := range boundaries {
			if b > start && b <= limit && (cut == 0 || absInt(b-targetEnd) < absInt(cut-targetEnd)) {
				cut = b
			}
		}
		if cut == 0 {
			part := splitChunk(s, start, end, target, max)
			out = append(out, part...)
			break
		}
		out = append(out, [2]int{start, cut})
		start = cut
	}
	return out
}
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func splitChunk(s *graph.Source, start, end, target, max int) [][2]int {
	if end-start <= max {
		return [][2]int{{start, end}}
	}
	var out [][2]int
	for start < end {
		cut := start + target
		if cut >= end {
			cut = end
		} else {
			limit := start + max
			if limit > end {
				limit = end
			}
			if p := strings.LastIndexByte(string(s.Text[cut:limit]), '\n'); p >= 0 {
				cut += p + 1
			} else if p := strings.LastIndexByte(string(s.Text[start:cut]), '\n'); p >= 0 && start+p+1 > start {
				cut = start + p + 1
			} else {
				cut = limit
			}
		}
		if cut <= start {
			cut = start + max
			if cut > end {
				cut = end
			}
		}
		for cut < end && cut > start && !utf8.RuneStart(s.Text[cut]) {
			cut--
		}
		if cut <= start {
			cut = end
		}
		out = append(out, [2]int{start, cut})
		start = cut
	}
	return out
}
