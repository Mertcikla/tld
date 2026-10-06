package indexer

import (
	"context"
	"encoding/json"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

const syntaxCacheVersion = 5

// syntaxCache is the per-source extraction cache. Version 4 stores one entry
// per declaration keyed by structural identity and body/context hash so an
// edited file reuses the facts of unchanged symbols.
type syntaxCache struct {
	Version  int
	FileHash string
	Decls    []declCacheEntry
	Calls    [][4]uint32
}

// declCacheEntry persists one declaration's extracted fact and the hashes that
// decide whether it can be reused after an edit.
type declCacheEntry struct {
	Key         string
	ParentKey   string
	Kind        int32
	Name        string
	BodyHash    string
	ContextHash string
	Start, End  int
	Fact        *pb.CodeFact
}

// syntaxFacts extracts a source's declarations. A current cache is only ever
// carried onto a source whose content hash is unchanged, so when it is present
// the facts are adopted verbatim without parsing the file at all.
// Otherwise the file is parsed once and only changed declarations are rebuilt.
func syntaxFacts(ctx context.Context, g *graph.Graph, src *graph.Source) ([]callSite, error) {
	var cached syntaxCache
	if src.SyntaxCache != "" {
		_ = json.Unmarshal([]byte(src.SyntaxCache), &cached)
	}
	if cached.Version == syntaxCacheVersion && cached.FileHash == src.Hash && len(cached.Decls) > 0 && allCached(cached) {
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

// adoptCached re-anchors an unchanged file's cached facts. It never parses, and
// leaves the cached bytes intact so repeated builds stay identical.
func adoptCached(g *graph.Graph, src *graph.Source, cache syntaxCache) []callSite {
	idByKey := map[string]string{}
	facts := make([]*pb.CodeFact, len(cache.Decls))
	for i := range cache.Decls {
		entry := cache.Decls[i]
		if entry.Fact == nil {
			continue
		}
		fact := g.AdoptFactAnchored(entry.Fact, src.Anchor(entry.Start, entry.End), entry.Fact.BodyHash, entry.Fact.Signature)
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
	}
	calls := make([]callSite, 0, len(cache.Calls))
	for _, span := range cache.Calls {
		calls = append(calls, callSite{path: src.Path, start: span[0], end: span[1], calleeStart: span[2], calleeEnd: span[3]})
	}
	return calls
}

// mergeExtraction rebuilds only declarations whose body or context changed and
// adopts cached facts (re-anchored to the new positions) for the rest. Fact ids
// are a function of snapshot, path, span, kind, and name, so a re-adopted
// unchanged declaration yields a deterministic id.
func mergeExtraction(g *graph.Graph, src *graph.Source, cached *syntaxCache, extraction fileExtraction) syntaxCache {
	oldByKey := map[string]declCacheEntry{}
	if cached != nil && cached.Version == syntaxCacheVersion {
		for _, entry := range cached.Decls {
			oldByKey[entry.Key] = entry
		}
	}
	next := syntaxCache{Version: syntaxCacheVersion}
	factIDs := make([]string, len(extraction.Decls))
	keys := make([]string, len(extraction.Decls))
	contexts := make([]string, len(extraction.Decls))
	contextHashes := make([]string, len(extraction.Decls))
	ordinals := map[string]int{}
	for i := range extraction.Decls {
		decl := extraction.Decls[i]
		scope := decl.Scope
		if decl.Parent >= 0 {
			scope = keys[decl.Parent] + "|" + scope
		}
		keys[i] = nextDeclKey(decl, scope, ordinals)
		contexts[i] = contextFor(extraction, i)
		contextHashes[i] = graph.Hash([]byte(contexts[i]))
	}

	// First pass: adopt unchanged declarations so parents have ids before
	// children are created.
	for i := range extraction.Decls {
		decl := extraction.Decls[i]
		prev, ok := oldByKey[keys[i]]
		if ok && prev.BodyHash == decl.BodyHash && prev.ContextHash == contextHashes[i] && prev.Fact != nil {
			fact := g.AdoptFactAnchored(prev.Fact, src.Anchor(decl.Start, decl.End), decl.BodyHash, decl.Signature)
			if fact != nil {
				factIDs[i] = fact.Id
			}
		}
	}
	// Second pass: create changed declarations and link parents.
	for i := range extraction.Decls {
		decl := extraction.Decls[i]
		entry := declCacheEntry{Kind: int32(decl.Kind), Name: decl.Name, BodyHash: decl.BodyHash, Key: keys[i], ContextHash: contextHashes[i], Start: decl.Start, End: decl.End}
		if decl.Parent >= 0 {
			entry.ParentKey = keys[decl.Parent]
		}
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
		fact.LogicalKey = graph.LogicalFactKey(decl.Kind, decl.Name, src.Path) + "|scope|" + keys[i]
		if decl.Parent >= 0 && factIDs[decl.Parent] != "" {
			fact.ParentFactId = factIDs[decl.Parent]
		}
		entry.Fact = fact
		next.Decls = append(next.Decls, entry)
	}
	for _, site := range extraction.Calls {
		next.Calls = append(next.Calls, [4]uint32{site.start, site.end, site.calleeStart, site.calleeEnd})
	}
	return next
}

func nextDeclKey(decl declExtraction, scope string, ordinals map[string]int) string {
	base := scope + "|" + decl.Kind.String() + "|" + decl.Name
	ordinal := ordinals[base]
	ordinals[base]++
	return base + "|" + strconv.Itoa(ordinal)
}

func spanRange(start, end int) string {
	return strconv.Itoa(start) + ":" + strconv.Itoa(end)
}
