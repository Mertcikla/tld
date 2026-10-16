package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/uptrace/bun"
)

// Each snapshot owns its bindings to immutable groups, independently of every
// other snapshot. Deleting an earlier snapshot cannot invalidate a later one.
func saveMembership(ctx context.Context, tx bun.Tx, snap *pb.Snapshot, g *graph.Graph) error {
	type members struct{ facts, edges []string }
	files := map[string]*members{}
	get := func(path string) *members {
		if files[path] == nil {
			files[path] = &members{}
		}
		return files[path]
	}
	for id, f := range g.Facts {
		get(f.GetAnchor().GetPath()).facts = append(get(f.GetAnchor().GetPath()).facts, id)
	}
	for id, e := range g.EdgeFacts {
		path := ""
		if f := g.Facts[e.FromFactId]; f != nil {
			path = f.GetAnchor().GetPath()
		}
		get(path).edges = append(get(path).edges, id)
	}
	groups := map[string]*members{}
	keys := make([]string, 0, len(files))
	for path, m := range files {
		sort.Strings(m.facts)
		sort.Strings(m.edges)
		parts := []string{snap.RepositoryId, path, "facts"}
		parts = append(parts, m.facts...)
		parts = append(parts, "edges")
		parts = append(parts, m.edges...)
		key := graph.ID(parts...)
		keys = append(keys, key)
		groups[key] = m
	}
	org := scope(ctx).value()
	existing := map[string]bool{}
	for start := 0; start < len(keys); start += 500 {
		end := min(start+500, len(keys))
		args := []any{org}
		for _, key := range keys[start:end] {
			args = append(args, key)
		}
		rows, err := tx.QueryContext(ctx, `SELECT group_key FROM codeindex_membership_groups WHERE org_id = ? AND group_key IN (`+strings.TrimSuffix(strings.Repeat("?,", end-start), ",")+`)`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				_ = rows.Close()
				return err
			}
			existing[key] = true
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
	}
	var groupRows, factRows, edgeRows, bindings [][]any
	for _, key := range keys {
		bindings = append(bindings, []any{snap.Id, key, org})
		if existing[key] {
			continue
		}
		groupRows = append(groupRows, []any{key, snap.RepositoryId, org})
		for _, id := range groups[key].facts {
			factRows = append(factRows, []any{key, id, org})
		}
		for _, id := range groups[key].edges {
			edgeRows = append(edgeRows, []any{key, id, org})
		}
	}
	for _, insert := range []struct {
		table, columns string
		rows           [][]any
	}{
		{"codeindex_membership_groups", "group_key, repository_id, org_id", groupRows},
		{"codeindex_membership_facts", "group_key, fact_id, org_id", factRows},
		{"codeindex_membership_edges", "group_key, edge_id, org_id", edgeRows},
		{"codeindex_snapshot_membership", "snapshot_id, group_key, org_id", bindings},
	} {
		if err := insertRows(ctx, tx, "INSERT INTO "+insert.table+" ("+insert.columns+") VALUES ", " ON CONFLICT DO NOTHING", 3, insert.rows); err != nil {
			return fmt.Errorf("save membership: %w", err)
		}
	}
	return nil
}

func collectOrphanMembership(ctx context.Context, tx bun.Tx) error {
	org := scope(ctx).value()
	for _, table := range []string{"codeindex_membership_facts", "codeindex_membership_edges", "codeindex_membership_groups"} {
		if _, err := tx.NewRaw(`DELETE FROM `+table+` WHERE org_id = ? AND NOT EXISTS (SELECT 1 FROM codeindex_snapshot_membership s WHERE s.org_id = `+table+`.org_id AND s.group_key = `+table+`.group_key)`, org).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
