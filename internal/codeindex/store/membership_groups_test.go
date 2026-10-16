package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

func TestMembershipGroupsKeepIndependentSnapshots(t *testing.T) {
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testMembershipGroups(t, idx)
}

func testMembershipGroups(t *testing.T, idx *Store) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	build := func(repo, seed, body string, base *graph.Graph) (*pb.Snapshot, *graph.Graph) {
		id := graph.ID(seed)
		g := graph.NewGraph(repo, id)
		snap := &pb.Snapshot{Id: id, RepositoryId: repo, IngestionStatus: "complete"}
		for _, path := range []string{"edited.go", "stable.go"} {
			text := "package p\nfunc Stable() {}\n"
			if path == "edited.go" {
				text = "package p\nfunc Changed() { " + body + " }\n"
			}
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(text))})
			if base != nil && path == "stable.go" {
				for _, f := range base.Facts {
					if f.GetAnchor().GetPath() == path {
						g.AdoptFactAnchored(f, f.Anchor, f.BodyHash, f.Signature)
					}
				}
			} else {
				g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, path, "go", src.Anchor(0, len(text)), text, "", nil)
			}
		}
		return snap, g
	}
	first, g1 := build("repo", "membership-first", "", nil)
	if err := idx.Publish(ctx, root, first, g1); err != nil {
		t.Fatal(err)
	}
	// Old databases retain direct memberships. Exercise mixed legacy/grouped
	// reads without rewriting the payloads or depending on an ancestor snapshot.
	org := scope(ctx).value()
	if _, err := idx.bun.NewRaw(`INSERT INTO codeindex_snapshot_facts_direct (snapshot_id,fact_id,org_id) SELECT snapshot_id,fact_id,org_id FROM codeindex_snapshot_facts WHERE snapshot_id=? AND org_id=?`, first.Id, org).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.bun.NewRaw(`DELETE FROM codeindex_snapshot_membership WHERE snapshot_id=? AND org_id=?`, first.Id, org).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// Migrated snapshots have no cached counts; all summary readers must
	// still resolve their original direct memberships correctly.
	if _, err := idx.bun.NewRaw("UPDATE codeindex_snapshots SET fact_count=NULL,edge_count=NULL,source_count=NULL WHERE id=? AND org_id=?", first.Id, org).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	legacy, err := idx.Snapshot(ctx, first.Id)
	if err != nil || legacy.GetStatistics().GetFacts() != 2 || legacy.GetStatistics().GetSources() != 2 {
		t.Fatalf("legacy statistics: %v %v", legacy, err)
	}
	repos, err := idx.ListRepositories(ctx)
	if err != nil || len(repos) != 1 || repos[0].Facts != 2 || repos[0].Sources != 2 {
		t.Fatalf("legacy repository counts: %v %v", repos, err)
	}
	second, g2 := build(first.RepositoryId, "membership-second", "println(1)", g1)
	if err := idx.PublishIncremental(ctx, root, second, g2, g1); err != nil {
		t.Fatal(err)
	}
	third, g3 := build(first.RepositoryId, "membership-third", "println(2)", g2)
	if err := idx.PublishIncremental(ctx, root, third, g3, g2); err != nil {
		t.Fatal(err)
	}
	var groups int
	if err := idx.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_membership_groups WHERE repository_id=? AND org_id=?`, first.RepositoryId, org).Scan(ctx, &groups); err != nil {
		t.Fatal(err)
	}
	if groups != 4 {
		t.Fatalf("membership groups=%d, want one shared stable file and three edited versions", groups)
	}
	stored, err := idx.LoadGraph(ctx, third.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.Id, second.Id} {
		if err := idx.DeleteSnapshot(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := idx.LoadGraph(ctx, third.Id)
	if err != nil {
		t.Fatal(err)
	}
	for id, f := range stored.Facts {
		if !proto.Equal(f, remaining.Facts[id]) {
			t.Fatalf("deleting earlier snapshots changed fact %s", id)
		}
	}
	if err := idx.DeleteSnapshot(ctx, third.Id); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"codeindex_membership_groups", "codeindex_membership_facts", "codeindex_membership_edges", "codeindex_snapshot_membership"} {
		var count int
		if err := idx.bun.NewRaw(`SELECT COUNT(*) FROM `+table+` WHERE org_id=?`, org).Scan(ctx, &count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d orphan rows", table, count)
		}
	}
}
