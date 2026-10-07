package indexer

import (
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// callSite is a call expression recorded by the single tree-sitter pass in
// treeFacts. The callee span frames the reference occurrence that SCIP uses to
// resolve the call; the outer span locates the enclosing code Fact.
type callSite struct {
	path                   string
	start, end             uint32
	calleeStart, calleeEnd uint32
}

// deriveCalls pairs call sites collected during tree-sitter indexing with the
// SCIP references for the same file. SCIP resolves the callee to an internal
// Fact or leaves it as an unresolved external symbol key; the enclosing code
// Fact is the call's source. No source is re-parsed.
func deriveCalls(g *graph.Graph, calls []callSite, table *symbols) {
	references := table.byPath()
	owners := graph.NewFactIndex(g.Facts)
	for _, site := range calls {
		source := g.Sources[site.path]
		if source == nil {
			continue
		}
		list := references[site.path]
		i := sort.Search(len(list), func(i int) bool { return list[i].anchor.StartByte >= site.calleeStart })
		var selected *occurrence
		for ; i < len(list) && list[i].anchor.StartByte < site.calleeEnd; i++ {
			o := &list[i]
			if o.anchor.EndByte <= site.calleeEnd && (selected == nil || o.anchor.StartByte > selected.anchor.StartByte) {
				selected = o
			}
		}
		if selected == nil {
			continue
		}
		callAnchor := source.Anchor(int(site.start), int(site.end))
		owner := owners.Enclosing(callAnchor)
		if owner == nil {
			continue
		}
		to := table.definitions[selected.key]
		targetKey := selected.key
		if to != "" {
			targetKey = ""
		}
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, owner.Id, to, targetKey, callAnchor, &pb.Evidence{Producer: "tree-sitter+scip", Version: selected.version, OriginalId: selected.key, Anchor: callAnchor, Derivation: "call expression with resolved reference"})
	}
}
