package indexer

import (
	"encoding/json"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// Whole-file chunks use the same split budget as declarations so a large file
// is embedded as several coherent passages rather than one oversized vector.
const (
	fileChunkTarget = 4096
	fileChunkMax    = 8192
)

// fileFactCache stores a whole-file fact and its chunks keyed by the content
// hash they were built from. Unchanged files adopt the cached fact and chunks
// (re-anchored to the same byte range) instead of re-splitting every build.
type fileFactCache struct {
	Hash   string
	Fact   *pb.CodeFact
	Chunks []*pb.Chunk
}

// addFileFacts publishes one FACT_KIND_FILE per discovered source and chunks the
// file's whole content. Symbol facts capture declarations; these facts capture
// the file as a unit so whole-file concepts (docs, markdown, config) get their
// own syntax extraction. A per-source cache keeps unchanged files from being
// re-chunked on every incremental build.
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
		ranges := splitChunk(src, 0, len(src.Text), fileChunkTarget, fileChunkMax)
		chunks := make([]*pb.Chunk, 0, len(ranges))
		for i, r := range ranges {
			chunks = append(chunks, g.AddChunk(f.Id, src.Anchor(r[0], r[1]), string(src.Text[r[0]:r[1]]), "", uint32(i), uint32(len(ranges))))
		}
		cache := fileFactCache{Hash: src.Hash, Fact: f, Chunks: chunks}
		if raw, err := json.Marshal(cache); err == nil {
			src.FileCache = string(raw)
		}
	}
}

// adoptCachedFileFact re-anchors a cached whole-file fact and its chunks when
// the source content is unchanged. It returns false when no usable cache exists.
func adoptCachedFileFact(g *graph.Graph, src *graph.Source, anchor *pb.SourceAnchor) bool {
	if src.FileCache == "" {
		return false
	}
	var cache fileFactCache
	if json.Unmarshal([]byte(src.FileCache), &cache) != nil || cache.Hash != src.Hash || cache.Fact == nil {
		return false
	}
	fact := g.AdoptFactAnchored(cache.Fact, anchor, "", "")
	if fact == nil {
		return false
	}
	fact.Evidence = append([]*pb.Evidence(nil), cache.Fact.Evidence...)
	for _, chunk := range cache.Chunks {
		start, end := int(chunk.Anchor.StartByte), int(chunk.Anchor.EndByte)
		if start < 0 || end > len(src.Text) {
			continue
		}
		g.AdoptChunkAnchored(&pb.Chunk{Id: chunk.Id, FactId: fact.Id, Anchor: src.Anchor(start, end), Text: chunk.Text, Context: chunk.Context, Index: chunk.Index, Total: chunk.Total}, fact.Id)
	}
	return true
}
