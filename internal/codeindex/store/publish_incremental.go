package store

import (
	"context"
	"slices"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/symbols"
	"google.golang.org/protobuf/proto"
)

// PublishIncremental validates carried entity identities against the previous
// published graph. The caller holds the repository indexing lease through
// preparation and publication. Full publication is the fallback for cold callers.
func (s *Store) PublishIncremental(ctx context.Context, root string, snap *pb.Snapshot, g, base *graph.Graph) error {
	if base == nil || base.RepositoryID != snap.RepositoryId {
		return s.Publish(ctx, root, snap, g)
	}
	var count int
	where, args := scope(ctx).clause("org_id")
	if err := s.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_snapshots WHERE id = ? AND repository_id = ?`+where, append([]any{base.SnapshotID, snap.RepositoryId}, args...)...).Scan(ctx, &count); err != nil {
		return err
	}
	if count == 0 {
		return s.Publish(ctx, root, snap, g)
	}
	return s.publish(ctx, root, snap, g, true, base)
}

func reusePublishedGraph(g, base *graph.Graph) error {
	var versions, oldVersions map[string]string
	factRemap := map[string]string{}
	for id, f := range g.Facts {
		old := base.Facts[id]
		if temporaryID(id) || old == nil {
			continue
		}
		if equalFactPayload(f, old) {
			g.Reused[id] = true
			continue
		}
		if versions == nil {
			versions, oldVersions = symbols.Versions(g), symbols.Versions(base)
		}
		a, err := factFingerprint(f, versions)
		if err != nil {
			return err
		}
		b, err := factFingerprint(old, oldVersions)
		if err != nil {
			return err
		}
		if a != b { // A changed payload cannot overwrite an immutable row.
			factRemap[id] = graph.ID(g.SnapshotID, "changed-fact", id)
			continue
		}
		g.Reused[id] = true
	}
	// Parent identity is persisted payload too. A changed parent invalidates
	// every carried descendant, even when its own source bytes are unchanged.
	for moved := true; moved; {
		moved = false
		for id, f := range g.Facts {
			if parent, ok := factRemap[f.ParentFactId]; ok {
				f.ParentFactId = parent
				delete(g.Reused, id)
				if !temporaryID(id) && base.Facts[id] != nil {
					factRemap[id] = graph.ID(g.SnapshotID, "changed-fact", id)
				}
				moved = true
			}
		}
	}
	remapEntities(g, factRemap)
	type key struct {
		kind                      pb.EdgeKind
		from, to, target, logical string
		weight                    float64
	}
	edgeKey := func(e *pb.EdgeFact) key {
		return key{e.Kind, e.FromFactId, e.ToFactId, e.TargetSymbolKey, e.LogicalKey, e.Weight}
	}
	used := map[string]bool{}
	pending := map[string]*pb.EdgeFact{}
	for id, e := range g.EdgeFacts {
		if old := base.EdgeFacts[id]; old != nil && edgeKey(e) == edgeKey(old) {
			g.Reused[id] = true
			used[id] = true
		} else {
			pending[id] = e
		}
	}
	candidates := map[key][]string{}
	for id, e := range base.EdgeFacts {
		if used[id] {
			continue
		}
		candidates[edgeKey(e)] = append(candidates[edgeKey(e)], id)
	}
	for k := range candidates {
		sort.Strings(candidates[k])
	}
	remap := map[string]string{}
	for id, e := range pending {
		k := edgeKey(e)
		if available := candidates[k]; len(available) > 0 {
			remap[id] = available[0]
			g.Reused[available[0]] = true
			candidates[k] = available[1:]
		}
	}
	remapEntities(g, remap)
	return nil
}

func equalFactPayload(a, b *pb.CodeFact) bool {
	if a.Language != b.Language || a.Kind != b.Kind || a.Name != b.Name || a.QualifiedName != b.QualifiedName || a.SymbolKey != b.SymbolKey || a.Signature != b.Signature || a.Documentation != b.Documentation || a.BodyHash != b.BodyHash || a.ParentFactId != b.ParentFactId || a.LogicalKey != b.LogicalKey || !proto.Equal(a.Anchor, b.Anchor) || !slices.Equal(a.Imports, b.Imports) || len(a.Evidence) != len(b.Evidence) {
		return false
	}
	for i, e := range a.Evidence {
		if !proto.Equal(e, b.Evidence[i]) {
			return false
		}
	}
	return true
}
