package indexer

import (
	"encoding/json"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// fileFactCache stores a whole-file fact keyed by the content hash it was built
// from. Unchanged files adopt the cached fact (re-anchored to the same byte
// range) instead of rebuilding it every build.
type fileFactCache struct {
	Hash string
	Fact *pb.CodeFact
}

// addFileFacts publishes one FACT_KIND_FILE per discovered source. Symbol facts
// capture declarations; these facts capture the file as a unit so whole-file
// concepts (docs, markdown, config) get their own syntax extraction. A
// per-source cache keeps unchanged files from being rebuilt on every
// incremental build.
func addFileFacts(g *graph.Graph) {
	paths := make([]string, 0, len(g.Sources))
	for path := range g.Sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		src := g.Sources[path]
		if src == nil || len(src.Text) == 0 {
			continue
		}
		anchor := src.Anchor(0, len(src.Text))
		if adoptCachedFileFact(g, src, anchor) {
			continue
		}
		f := g.AddFact(pb.FactKind_FACT_KIND_FILE, path, src.Language, anchor, "", "", &pb.Evidence{Producer: "file"})
		cache := fileFactCache{Hash: src.Hash, Fact: f}
		if raw, err := json.Marshal(cache); err == nil {
			src.FileCache = string(raw)
		}
	}
}

// adoptCachedFileFact re-anchors a cached whole-file fact when the source
// content is unchanged. It returns false when no usable cache exists.
func adoptCachedFileFact(g *graph.Graph, src *graph.Source, anchor *pb.SourceAnchor) bool {
	if src.FileCache == "" {
		return false
	}
	var cache fileFactCache
	if json.Unmarshal([]byte(src.FileCache), &cache) != nil || cache.Hash != src.Hash || cache.Fact == nil {
		return false
	}
	fact := g.AdoptFactAnchored(cache.Fact, anchor, cache.Fact.BodyHash, "")
	if fact == nil {
		return false
	}
	fact.Evidence = append([]*pb.Evidence(nil), cache.Fact.Evidence...)
	return true
}
