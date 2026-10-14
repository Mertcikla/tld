package store

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"encoding/hex"
	"encoding/json"
	"github.com/mertcikla/codeindex/graph"
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
	facts, edges := g.SortedFacts(), g.SortedEdgeFacts()
	for _, f := range facts {
		if !g.Reused[f.Id] || temporaryID(f.Id) {
			g.Reused[f.Id] = false
			remap[f.Id] = strconv.FormatInt(first, 10)
			first++
		}
	}
	for _, e := range edges {
		if !g.Reused[e.Id] || temporaryID(e.Id) {
			g.Reused[e.Id] = false
			remap[e.Id] = strconv.FormatInt(first, 10)
			first++
		}
	}
	mapped := func(id string) string {
		if next, ok := remap[id]; ok {
			return next
		}
		return id
	}
	g.RepositoryID, g.SnapshotID = snap.RepositoryId, snap.Id
	g.Facts = make(map[string]*pb.CodeFact, len(facts))
	for _, f := range facts {
		f.Id, f.ParentFactId = mapped(f.Id), mapped(f.ParentFactId)
		f.RepositoryId, f.SnapshotId = snap.RepositoryId, mapped(f.SnapshotId)
		g.Facts[f.Id] = f
	}
	g.EdgeFacts = make(map[string]*pb.EdgeFact, len(edges))
	for _, e := range edges {
		e.Id, e.FromFactId, e.ToFactId = mapped(e.Id), mapped(e.FromFactId), mapped(e.ToFactId)
		e.RepositoryId, e.SnapshotId = snap.RepositoryId, mapped(e.SnapshotId)
		g.EdgeFacts[e.Id] = e
	}
	// Caches contain extracted facts. Refresh their rows so subsequent indexing
	// sees assigned IDs rather than resurrecting temporary references.
	var rewrite func(any) any
	rewrite = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				if fact := g.Facts[mapped(id)]; fact != nil {
					raw, _ := json.Marshal(fact)
					var row map[string]any
					if json.Unmarshal(raw, &row) == nil {
						return row
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

// normalizeReuse prevents cached rows from another repository or a replaced
// parent from being published as shared rows with different references.
func normalizeReuse(g *graph.Graph, repositoryID string) {
	for id, fact := range g.Facts {
		if temporaryID(id) || fact.RepositoryId != repositoryID {
			g.Reused[id] = false
		}
	}
	for changed := true; changed; {
		changed = false
		for id, fact := range g.Facts {
			if g.Reused[id] && fact.ParentFactId != "" {
				if _, present := g.Facts[fact.ParentFactId]; present && !g.Reused[fact.ParentFactId] {
					g.Reused[id] = false
					changed = true
				}
			}
		}
	}
}
