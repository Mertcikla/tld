package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/uptrace/bun"
)

// Fingerprints cover persisted payload fields, including source positions,
// evidence and physical parent IDs. Facts additionally normalize package versions
// belonging to this snapshot; raw symbol strings are restored on snapshot reads.
func rowFingerprint(values []any) (string, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return graph.Hash(raw), nil
}

func matchingEntities(ctx context.Context, tx bun.Tx, table, repositoryID string, hashes map[string]string) (map[string][]string, error) {
	unique := map[string]bool{}
	for _, hash := range hashes {
		unique[hash] = true
	}
	ordered := make([]string, 0, len(unique))
	for hash := range unique {
		ordered = append(ordered, hash)
	}
	found := map[string][]string{}
	for start := 0; start < len(ordered); start += 500 {
		end := min(start+500, len(ordered))
		args := []any{scope(ctx).value(), repositoryID}
		for _, hash := range ordered[start:end] {
			args = append(args, hash)
		}
		rows, err := tx.QueryContext(ctx, `SELECT content_hash, id FROM `+table+` WHERE org_id = ? AND repository_id = ? AND content_hash IN (`+strings.TrimSuffix(strings.Repeat("?,", end-start), ",")+`) ORDER BY id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var hash, id string
			if err := rows.Scan(&hash, &id); err != nil {
				_ = rows.Close()
				return nil, err
			}
			found[hash] = append(found[hash], id)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

// Each existing row can satisfy one observation per snapshot. Equal repeated
// edges must remain distinct so reference counts and dependency weights agree.
func entityRemap(hashes map[string]string, candidates map[string][]string, reused map[string]bool) map[string]string {
	ids := make([]string, 0, len(hashes))
	for id := range hashes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	remap := map[string]string{}
	for _, id := range ids {
		hash := hashes[id]
		available := candidates[hash]
		for len(available) > 0 && reused[available[0]] {
			available = available[1:]
		}
		candidates[hash] = available
		if len(available) > 0 {
			remap[id] = available[0]
			reused[available[0]] = true
			candidates[hash] = available[1:]
		}
	}
	return remap
}

func remapEntities(g *graph.Graph, remap map[string]string) {
	if len(remap) == 0 {
		return
	}
	mapped := func(id string) string {
		if next, ok := remap[id]; ok {
			return next
		}
		return id
	}
	// Most warm remaps touch only edited declarations and their edges. Move
	// those keys after iteration rather than rebuilding both complete maps.
	var facts []*pb.CodeFact
	for id, f := range g.Facts {
		f.ParentFactId = mapped(f.ParentFactId)
		if next := mapped(f.Id); next != id {
			delete(g.Facts, id)
			f.Id = next
			facts = append(facts, f)
		}
	}
	for _, f := range facts {
		g.Facts[f.Id] = f
	}
	var edges []*pb.EdgeFact
	for id, e := range g.EdgeFacts {
		e.FromFactId, e.ToFactId = mapped(e.FromFactId), mapped(e.ToFactId)
		if next := mapped(e.Id); next != id {
			delete(g.EdgeFacts, id)
			e.Id = next
			edges = append(edges, e)
		}
	}
	for _, e := range edges {
		g.EdgeFacts[e.Id] = e
	}
	rewriteGraphCaches(g, remap)
}

func deduplicateFacts(ctx context.Context, tx bun.Tx, g *graph.Graph, versions map[string]string) (map[string]string, error) {
	pending := make(map[string]*pb.CodeFact, len(g.Facts))
	for id, f := range g.Facts {
		if !g.Reused[id] {
			pending[id] = f
		}
	}
	hashes, remap := map[string]string{}, map[string]string{}
	// Resolve parents before children: a reused child must refer to the exact
	// parent row selected for this snapshot, not merely an equivalent parent.
	for len(pending) > 0 {
		ready := map[string]string{}
		for id, f := range pending {
			if pending[f.ParentFactId] != nil {
				continue
			}
			if parent, ok := remap[f.ParentFactId]; ok {
				f.ParentFactId = parent
			}
			hash, err := factFingerprint(f, versions)
			if err != nil {
				return nil, err
			}
			ready[id] = hash
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("cyclic fact parent references")
		}
		candidates, err := matchingEntities(ctx, tx, "codeindex_facts", g.RepositoryID, ready)
		if err != nil {
			return nil, err
		}
		for id, existing := range entityRemap(ready, candidates, g.Reused) {
			remap[id] = existing
		}
		for id, hash := range ready {
			target := id
			if existing, ok := remap[id]; ok {
				target = existing
			}
			hashes[target] = hash
			delete(pending, id)
		}
	}
	remapEntities(g, remap)
	return hashes, nil
}

func deduplicateEdges(ctx context.Context, tx bun.Tx, g *graph.Graph) (map[string]string, error) {
	hashes := map[string]string{}
	for id, e := range g.EdgeFacts {
		if g.Reused[id] {
			continue
		}
		values, err := edgeValues(e)
		if err != nil {
			return nil, err
		}
		hash, err := rowFingerprint(values)
		if err != nil {
			return nil, err
		}
		hashes[id] = hash
	}
	candidates, err := matchingEntities(ctx, tx, "codeindex_edges", g.RepositoryID, hashes)
	if err != nil {
		return nil, err
	}
	remap := entityRemap(hashes, candidates, g.Reused)
	for id, existing := range remap {
		hash := hashes[id]
		delete(hashes, id)
		hashes[existing] = hash
	}
	remapEntities(g, remap)
	return hashes, nil
}
