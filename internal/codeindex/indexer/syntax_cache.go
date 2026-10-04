package indexer

import (
	"context"
	"encoding/json"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

// syntaxCache is the per-source extraction cache. Version 3 stores one entry
// per declaration keyed by structural identity and body/context hash so an
// edited file reuses the facts and chunks of unchanged symbols.
type syntaxCache struct {
	Version  int
	FileHash string
	Decls    []declCacheEntry
	Calls    [][4]uint32
}

// declCacheEntry persists one declaration's extracted facts, chunks, and the
// hashes that decide whether it can be reused after an edit.
type declCacheEntry struct {
	Key         string
	ParentKey   string
	Kind        int32
	Name        string
	BodyHash    string
	ContextHash string
	Start, End  int
	Fact        *pb.CodeFact
	Chunks      []*pb.Chunk
}

// syntaxFacts extracts a source's declarations. A version 3 cache is only ever
// carried onto a source whose content hash is unchanged, so when it is present
// the facts and chunks are adopted verbatim without parsing the file at all.
// Otherwise the file is parsed once and only changed declarations are rebuilt.
func syntaxFacts(ctx context.Context, g *graph.Graph, src *graph.Source) ([]callSite, error) {
	var cached syntaxCache
	if src.SyntaxCache != "" {
		_ = json.Unmarshal([]byte(src.SyntaxCache), &cached)
	}
	if cached.Version == 3 && cached.FileHash == src.Hash && len(cached.Decls) > 0 && allCached(cached) {
		return adoptCached(g, src, cached), nil
	}
	extraction, err := extractFile(ctx, src)
	if err != nil {
		return nil, err
	}
	next := mergeExtraction(g, src, &cached, extraction)
	next.FileHash = src.Hash
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	src.SyntaxCache = string(raw)
	calls := make([]callSite, 0, len(extraction.Calls))
	calls = append(calls, extraction.Calls...)
	return calls, nil
}

func allCached(cache syntaxCache) bool {
	for _, entry := range cache.Decls {
		if entry.Fact == nil || entry.End <= entry.Start {
			return false
		}
	}
	return true
}

// adoptCached re-anchors an unchanged file's cached facts and chunks. It never
// parses, and leaves the cached bytes intact so repeated builds stay identical.
func adoptCached(g *graph.Graph, src *graph.Source, cache syntaxCache) []callSite {
	idByKey := map[string]string{}
	facts := make([]*pb.CodeFact, len(cache.Decls))
	for i := range cache.Decls {
		entry := cache.Decls[i]
		if entry.Fact == nil {
			continue
		}
		fact := g.AdoptFactAnchored(entry.Fact, src.Anchor(entry.Start, entry.End), entry.Fact.Code, entry.Fact.Signature)
		if fact == nil {
			continue
		}
		idByKey[entry.Key] = fact.Id
		facts[i] = fact
	}
	for i := range cache.Decls {
		entry := cache.Decls[i]
		fact := facts[i]
		if fact == nil {
			continue
		}
		if parentID, ok := idByKey[entry.ParentKey]; ok {
			fact.ParentFactId = parentID
		}
		for _, chunk := range entry.Chunks {
			anchor := src.Anchor(int(chunk.Anchor.StartByte), int(chunk.Anchor.EndByte))
			g.AdoptChunkAnchored(&pb.Chunk{Id: chunk.Id, FactId: fact.Id, Anchor: anchor, Text: chunk.Text, Context: chunk.Context, Index: chunk.Index, Total: chunk.Total}, fact.Id)
		}
	}
	calls := make([]callSite, 0, len(cache.Calls))
	for _, span := range cache.Calls {
		calls = append(calls, callSite{path: src.Path, start: span[0], end: span[1], calleeStart: span[2], calleeEnd: span[3]})
	}
	return calls
}

// mergeExtraction rebuilds only declarations whose body or context changed and
// adopts cached facts and chunks (re-anchored to the new positions) for the
// rest. Fact ids are a function of snapshot, path, span, kind, and name, so a
// re-adopted unchanged declaration yields a deterministic id.
func mergeExtraction(g *graph.Graph, src *graph.Source, cached *syntaxCache, extraction fileExtraction) syntaxCache {
	oldByKey := map[string]declCacheEntry{}
	if cached != nil {
		for _, entry := range cached.Decls {
			oldByKey[entry.Key] = entry
		}
	}
	next := syntaxCache{Version: 3}
	factIDs := make([]string, len(extraction.Decls))
	keys := make([]string, len(extraction.Decls))
	contexts := make([]string, len(extraction.Decls))
	contextHashes := make([]string, len(extraction.Decls))
	ordinals := map[string]int{}
	reused := make([]bool, len(extraction.Decls))
	for i := range extraction.Decls {
		decl := extraction.Decls[i]
		keys[i] = nextDeclKey(decl, ordinals)
		contexts[i] = contextFor(extraction, i)
		contextHashes[i] = graph.Hash([]byte(contexts[i]))
	}

	// First pass: adopt unchanged declarations so parents have ids before
	// children are created.
	for i := range extraction.Decls {
		decl := extraction.Decls[i]
		prev, ok := oldByKey[keys[i]]
		if ok && prev.BodyHash == decl.BodyHash && prev.ContextHash == contextHashes[i] && prev.Fact != nil {
			fact := g.AdoptFactAnchored(prev.Fact, src.Anchor(decl.Start, decl.End), decl.Code, decl.Signature)
			if fact != nil {
				factIDs[i] = fact.Id
				reused[i] = true
			}
		}
	}
	// Second pass: create changed declarations, link parents, and record chunks.
	for i := range extraction.Decls {
		decl := extraction.Decls[i]
		entry := declCacheEntry{Kind: int32(decl.Kind), Name: decl.Name, BodyHash: decl.BodyHash, Key: keys[i], ContextHash: contextHashes[i], Start: decl.Start, End: decl.End}
		if decl.Parent >= 0 {
			entry.ParentKey = keys[decl.Parent]
		}
		context := contexts[i]
		if factIDs[i] == "" {
			anchor := src.Anchor(decl.Start, decl.End)
			fact := g.AddFact(decl.Kind, decl.Name, src.Language, anchor, decl.Code, decl.Signature, &pb.Evidence{Producer: "tree-sitter", OriginalRange: spanRange(decl.Start, decl.End), Anchor: anchor})
			fact.Imports = append([]string(nil), extraction.Imports...)
			factIDs[i] = fact.Id
		}
		fact := g.Facts[factIDs[i]]
		if fact == nil {
			next.Decls = append(next.Decls, entry)
			continue
		}
		if decl.Parent >= 0 && factIDs[decl.Parent] != "" {
			fact.ParentFactId = factIDs[decl.Parent]
		}
		if reused[i] {
			entry.Fact = fact
			entry.Chunks = rebuildChunkAnchors(g, src, fact, decl, context, oldByKey[keys[i]].Chunks)
		} else {
			entry.Fact = fact
			entry.Chunks = buildChunks(g, src, fact, decl, context)
		}
		next.Decls = append(next.Decls, entry)
	}
	for _, site := range extraction.Calls {
		next.Calls = append(next.Calls, [4]uint32{site.start, site.end, site.calleeStart, site.calleeEnd})
	}
	return next
}

func buildChunks(g *graph.Graph, src *graph.Source, fact *pb.CodeFact, decl declExtraction, context string) []*pb.Chunk {
	out := make([]*pb.Chunk, 0, len(decl.chunks))
	for i, r := range decl.chunks {
		anchor := src.Anchor(r[0], r[1])
		out = append(out, g.AddChunk(fact.Id, anchor, string(src.Text[r[0]:r[1]]), context, uint32(i), uint32(len(decl.chunks))))
	}
	return out
}

// rebuildChunkAnchors re-anchors the declaration's chunks onto their new source
// positions, sharing its row only when all persisted metadata is unchanged.
// The text and context are unchanged because the body hash matched.
func rebuildChunkAnchors(g *graph.Graph, src *graph.Source, fact *pb.CodeFact, decl declExtraction, context string, previous []*pb.Chunk) []*pb.Chunk {
	out := make([]*pb.Chunk, 0, len(decl.chunks))
	for i, r := range decl.chunks {
		anchor := src.Anchor(r[0], r[1])
		chunk := &pb.Chunk{FactId: fact.Id, SnapshotId: g.SnapshotID, Anchor: anchor, Text: string(src.Text[r[0]:r[1]]), Context: context, Index: uint32(i), Total: uint32(len(decl.chunks))}
		if i < len(previous) && previous[i].FactId == fact.Id && proto.Equal(previous[i].Anchor, anchor) && previous[i].Text == chunk.Text && previous[i].Context == context && previous[i].Index == chunk.Index && previous[i].Total == chunk.Total {
			chunk.Id = previous[i].Id
		}
		out = append(out, g.AdoptChunkAnchored(chunk, fact.Id))
	}
	return out
}

func nextDeclKey(decl declExtraction, ordinals map[string]int) string {
	base := decl.Kind.String() + "|" + decl.Name
	ordinal := ordinals[base]
	ordinals[base]++
	return base + "|" + strconv.Itoa(ordinal)
}

func spanRange(start, end int) string {
	return strconv.Itoa(start) + ":" + strconv.Itoa(end)
}
