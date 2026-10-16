package store

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"encoding/hex"
	"encoding/json"
	"github.com/mertcikla/codeindex/graph"
	"sort"
	"strconv"
)

// temporaryID identifies extraction-local hashes. Persisted IDs are decimal strings.
func temporaryID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func graphIDCount(g *graph.Graph) int {
	count := 1
	for id := range g.Facts {
		if !g.Reused[id] || temporaryID(id) {
			count++
		}
	}
	for id := range g.EdgeFacts {
		if !g.Reused[id] || temporaryID(id) {
			count++
		}
	}
	return count
}

// assignGraphIDs translates extraction-local references once, at publication.
// Content hashes, logical keys, anchors and evidence are left untouched.
func assignGraphIDs(snap *pb.Snapshot, g *graph.Graph, first int64) {
	oldSnapshot := g.SnapshotID
	snap.Id = strconv.FormatInt(first, 10)
	first++
	remap := map[string]string{oldSnapshot: snap.Id, g.RepositoryID: snap.RepositoryId}
	var freshFacts []*pb.CodeFact
	var freshEdges []*pb.EdgeFact
	for _, f := range g.Facts {
		if !g.Reused[f.Id] || temporaryID(f.Id) {
			freshFacts = append(freshFacts, f)
		}
	}
	for _, e := range g.EdgeFacts {
		if !g.Reused[e.Id] || temporaryID(e.Id) {
			freshEdges = append(freshEdges, e)
		}
	}
	sort.Slice(freshFacts, func(i, j int) bool { return freshFacts[i].Id < freshFacts[j].Id })
	sort.Slice(freshEdges, func(i, j int) bool { return freshEdges[i].Id < freshEdges[j].Id })
	for _, f := range freshFacts {
		if !g.Reused[f.Id] || temporaryID(f.Id) {
			g.Reused[f.Id] = false
			remap[f.Id] = strconv.FormatInt(first, 10)
			first++
		}
	}
	for _, e := range freshEdges {
		if !g.Reused[e.Id] || temporaryID(e.Id) {
			g.Reused[e.Id] = false
			remap[e.Id] = strconv.FormatInt(first, 10)
			first++
		}
	}
	g.RepositoryID, g.SnapshotID = snap.RepositoryId, snap.Id
	for _, f := range g.Facts {
		f.RepositoryId, f.SnapshotId = snap.RepositoryId, snap.Id
	}
	for _, e := range g.EdgeFacts {
		e.RepositoryId, e.SnapshotId = snap.RepositoryId, snap.Id
	}
	remapEntities(g, remap)
}

func rewriteGraphCaches(g *graph.Graph, remap map[string]string) {
	paths := map[string]bool{}
	for _, id := range remap {
		if f := g.Facts[id]; f != nil {
			paths[f.GetAnchor().GetPath()] = true
		}
	}
	mapped := func(id string) string {
		if next, ok := remap[id]; ok {
			return next
		}
		return id
	}
	// Caches contain extracted facts. Refresh their rows so subsequent indexing
	// sees assigned IDs rather than resurrecting temporary references.
	var rewrite func(any) any
	rewrite = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				if fact := g.Facts[mapped(id)]; fact != nil {
					// Extraction caches contain syntax metadata, not later SCIP
					// enrichment. Rewrite references without replacing that payload.
					v["id"], v["repository_id"], v["snapshot_id"] = fact.Id, g.RepositoryID, g.SnapshotID
					if parent, ok := v["parent_fact_id"].(string); ok {
						v["parent_fact_id"] = mapped(parent)
					}
				}
			}
			for key, item := range v {
				v[key] = rewrite(item)
			}
		case []any:
			for i, item := range v {
				v[i] = rewrite(item)
			}
		}
		return value
	}
	for _, src := range g.Sources {
		if !paths[src.Path] {
			continue
		}
		for _, cache := range []*string{&src.SyntaxCache, &src.FileCache} {
			if *cache == "" {
				continue
			}
			var value any
			if json.Unmarshal([]byte(*cache), &value) != nil {
				continue
			}
			raw, err := json.Marshal(rewrite(value))
			if err == nil {
				*cache = string(raw)
			}
		}
	}
}
