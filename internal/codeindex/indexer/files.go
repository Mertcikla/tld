package indexer

import (
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

// addFileFacts publishes one FACT_KIND_FILE per discovered source and chunks the
// file's whole content. Symbol facts capture declarations; these facts capture
// the file as a unit so whole-file concepts (docs, markdown, config) get their
// own embeddings.
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
		f := g.AddFact(pb.FactKind_FACT_KIND_FILE, path, src.Language, anchor, "", "", &pb.Evidence{Producer: "file"})
		ranges := splitChunk(src, 0, len(src.Text), fileChunkTarget, fileChunkMax)
		for i, r := range ranges {
			g.AddChunk(f.Id, src.Anchor(r[0], r[1]), string(src.Text[r[0]:r[1]]), "", uint32(i), uint32(len(ranges)))
		}
	}
}
