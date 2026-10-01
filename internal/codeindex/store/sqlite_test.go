package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

const testSource = `package sample

type Item struct {
	Name string
}

func Run() {
	helper()
}

func helper() {}
`

func openTestStore(t *testing.T) (*Store, *dbrepo.Handle) {
	t.Helper()
	handle, err := dbrepo.OpenSQLite(context.Background(), dbrepo.DBOptions{
		SQLitePath: filepath.Join(t.TempDir(), "tld.db"),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return NewStoreFromHandle(handle), handle
}

func TestPublishRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	pipeline := indexer.Pipeline{Config: config.Default()}
	snap, g, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: dir}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(g.Facts) == 0 || len(g.Chunks) == 0 {
		t.Fatalf("indexer produced no graph: facts=%d chunks=%d", len(g.Facts), len(g.Chunks))
	}

	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	if err := st.Publish(ctx, dir, snap, g); err != nil {
		t.Fatalf("publish: %v", err)
	}

	latest, err := st.Latest(ctx, snap.RepositoryId)
	if err != nil || latest != snap.Id {
		t.Fatalf("latest = %q (err %v), want %q", latest, err, snap.Id)
	}

	loadedSnap, err := st.Snapshot(ctx, snap.Id)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(loadedSnap.Sources) != len(snap.Sources) {
		t.Fatalf("sources = %d, want %d", len(loadedSnap.Sources), len(snap.Sources))
	}

	facts, err := st.Facts(ctx, snap.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 1000)
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(facts) != len(g.Facts) {
		t.Fatalf("facts = %d, want %d", len(facts), len(g.Facts))
	}

	chunks, err := st.Chunks(ctx, snap.Id)
	if err != nil {
		t.Fatalf("chunks: %v", err)
	}
	if len(chunks) != len(g.Chunks) {
		t.Fatalf("chunks = %d, want %d", len(chunks), len(g.Chunks))
	}

	edges, err := st.EdgeFacts(ctx, snap.Id, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", "", 1000)
	if err != nil {
		t.Fatalf("edges: %v", err)
	}
	if len(edges) != len(g.EdgeFacts) {
		t.Fatalf("edges = %d, want %d", len(edges), len(g.EdgeFacts))
	}

	// A fact round-trips with its anchor and evidence intact.
	var factID string
	for id := range g.Facts {
		factID = id
		break
	}
	fact, err := st.Fact(ctx, factID)
	if err != nil {
		t.Fatalf("fact: %v", err)
	}
	if fact.Anchor == nil || fact.Anchor.Path != "sample.go" {
		t.Fatalf("fact anchor = %+v", fact.Anchor)
	}
	if len(fact.Evidence) == 0 {
		t.Fatal("fact evidence lost")
	}

	// Source content is retrievable by hash.
	var hash string
	for _, src := range snap.Sources {
		hash = src.Hash
	}
	content, err := st.Source(ctx, hash)
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if string(content) != testSource {
		t.Fatalf("source content mismatch")
	}

	// LoadGraph reconstructs the same counts.
	lg, err := st.LoadGraph(ctx, snap.Id)
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	if len(lg.Facts) != len(g.Facts) || len(lg.Chunks) != len(g.Chunks) || len(lg.EdgeFacts) != len(g.EdgeFacts) {
		t.Fatalf("load graph mismatch: %d/%d/%d", len(lg.Facts), len(lg.Chunks), len(lg.EdgeFacts))
	}

	// Diffing a snapshot against itself yields no changes.
	diff, err := st.Diff(ctx, snap.Id, snap.Id, false)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.Sources) != 0 || len(diff.Facts.Added) != 0 || len(diff.Facts.Removed) != 0 || len(diff.Facts.Modified) != 0 {
		t.Fatalf("self diff not empty: %+v", diff)
	}

	// Re-publishing the same snapshot is idempotent.
	if err := st.Publish(ctx, dir, snap, g); err != nil {
		t.Fatalf("republish: %v", err)
	}
	again, err := st.Facts(ctx, snap.Id, pb.FactKind_FACT_KIND_UNSPECIFIED, "", "", 1000)
	if err != nil {
		t.Fatalf("facts after republish: %v", err)
	}
	if len(again) != len(g.Facts) {
		t.Fatalf("republish duplicated facts: %d want %d", len(again), len(g.Facts))
	}

	// ListRepositories summarizes the indexed repository and latest snapshot.
	repos, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(repos) != 1 || repos[0].Id != snap.RepositoryId {
		t.Fatalf("repositories = %+v", repos)
	}
	if repos[0].LatestSnapshotId != snap.Id || repos[0].Facts != uint32(len(g.Facts)) || repos[0].Sources != uint32(len(snap.Sources)) {
		t.Fatalf("repository summary = %+v", repos[0])
	}

	// Vector similarity ranks the fact whose embedding is closest to the query.
	if len(facts) >= 2 {
		profile := "test-profile"
		a, b := facts[0], facts[1]
		if err := st.SaveFactEmbedding(ctx, &pb.Embedding{FactId: a.Id, SnapshotId: snap.Id, Profile: profile, Model: "test", Dimensions: 2, Vector: []float32{1, 0}}); err != nil {
			t.Fatalf("save fact embedding a: %v", err)
		}
		if err := st.SaveFactEmbedding(ctx, &pb.Embedding{FactId: b.Id, SnapshotId: snap.Id, Profile: profile, Model: "test", Dimensions: 2, Vector: []float32{0, 1}}); err != nil {
			t.Fatalf("save fact embedding b: %v", err)
		}
		scores, err := st.SimilarFacts(ctx, snap.Id, profile, []float32{1, 0}, 10)
		if err != nil {
			t.Fatalf("similar facts: %v", err)
		}
		if len(scores) == 0 || scores[0].FactID != a.Id {
			t.Fatalf("similar facts top = %+v, want %s", scores, a.Id)
		}
	}
}
