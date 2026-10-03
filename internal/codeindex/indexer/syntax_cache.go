package indexer

import (
	"context"
	"encoding/json"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

type syntaxCache struct {
	Version int
	Facts   []*pb.CodeFact
	Chunks  []*pb.Chunk
	Calls   [][4]uint32
}

// Cache syntax-only facts before SCIP enriches them, so enrichment and bindings
// are recomputed from the current project rather than inherited from old inputs.
func syntaxFacts(ctx context.Context, g *graph.Graph, src *graph.Source) ([]callSite, error) {
	var cached syntaxCache
	if src.SyntaxCache != "" && json.Unmarshal([]byte(src.SyntaxCache), &cached) == nil && cached.Version == 2 {
		remap := map[string]string{}
		for _, fact := range cached.Facts {
			if adopted := g.AdoptFact(fact); adopted != nil {
				remap[fact.Id] = adopted.Id
			}
		}
		for _, fact := range cached.Facts {
			if adopted := g.Facts[remap[fact.Id]]; adopted != nil {
				adopted.ParentFactId = remap[fact.ParentFactId]
			}
		}
		for _, chunk := range cached.Chunks {
			if factID := remap[chunk.FactId]; factID != "" {
				g.AdoptChunk(chunk, factID)
			}
		}
		calls := make([]callSite, 0, len(cached.Calls))
		for _, span := range cached.Calls {
			calls = append(calls, callSite{path: src.Path, start: span[0], end: span[1], calleeStart: span[2], calleeEnd: span[3]})
		}
		return calls, nil
	}
	calls, err := treeFacts(ctx, g, src)
	if err != nil {
		return nil, err
	}
	cached.Version = 2
	for _, fact := range g.SortedFacts() {
		if fact.Anchor != nil && fact.Anchor.Path == src.Path {
			cached.Facts = append(cached.Facts, fact)
		}
	}
	for _, chunk := range g.SortedChunks() {
		if chunk.Anchor != nil && chunk.Anchor.Path == src.Path {
			cached.Chunks = append(cached.Chunks, chunk)
		}
	}
	for _, site := range calls {
		cached.Calls = append(cached.Calls, [4]uint32{site.start, site.end, site.calleeStart, site.calleeEnd})
	}
	raw, err := json.Marshal(cached)
	if err != nil {
		return nil, err
	}
	src.SyntaxCache = string(raw)
	return calls, nil
}
