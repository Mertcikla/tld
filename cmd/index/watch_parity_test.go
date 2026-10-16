package index

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/digest"
	"github.com/mertcikla/codeindex/gitstate"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	"github.com/mertcikla/codeindex/ingest"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/proto"
)

func graphDigest(snap *pb.Snapshot, g *graph.Graph) string {
	facts, edges := make([]*pb.CodeFact, 0, len(g.Facts)), make([]*pb.EdgeFact, 0, len(g.EdgeFacts))
	for _, f := range g.Facts {
		facts = append(facts, f)
	}
	for _, e := range g.EdgeFacts {
		edges = append(edges, e)
	}
	return digest.Snapshot(snap, facts, edges)
}

func TestWarmWatchSnapshotDiffParity(t *testing.T) {
	if os.Getenv("CODEINDEX_TEST_LSP") == "" {
		t.Skip("set CODEINDEX_TEST_LSP=1 for warm watch / full SCIP and persisted diff parity")
	}
	t.Setenv("GOWORK", "off")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	testGit(t, root, "init", "-b", "main")
	testGit(t, root, "config", "user.name", "Test")
	testGit(t, root, "config", "user.email", "test@example.com")
	writeSource(t, root, "go.mod", "module example.com/warm\n\ngo 1.26\n")
	original := "package warm\nfunc A() { B() }\nfunc B() {}\nfunc C() {}\n"
	writeSource(t, root, "a.go", original)
	writeSource(t, root, "b.go", "package warm\nfunc D() { A() }\n")
	writeSource(t, root, "stable.go", "package warm\nfunc Stable() {}\n")
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-m", "base")
	ws, err := localstore.OpenLocal(ctx, &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	cfg := config.Default()
	session := indexer.NewWatchSession(ctx, cfg, root)
	defer func() { session.Close() }()
	e := &engine{store: idx, ws: ws, cfg: cfg, out: io.Discard, semantics: session}
	revision := ""
	saved := map[string]string{}
	for _, step := range []struct {
		name string
		edit func()
	}{
		{"initial", func() {}},
		{"body", func() { writeSource(t, root, "a.go", "package warm\nfunc A() { C() }\nfunc B() {}\nfunc C() {}\n") }},
		{"shifted", func() {
			writeSource(t, root, "a.go", "package warm\nfunc A() {\n // 😀 shifted\n B(); C()\n}\nfunc B() {}\nfunc C() {}\n")
		}},
		{"revert", func() { writeSource(t, root, "a.go", original) }},
		{"addition", func() { writeSource(t, root, "new.go", "package warm\nfunc New() { C() }\n") }},
		{"restart", func() {
			session.Close()
			session = indexer.NewWatchSession(ctx, cfg, root)
			e.semantics = session
			e.cache = ingest.WatchCache{}
			e.baseline = nil
			writeSource(t, root, "a.go", "package warm\nfunc A() { C(); B() }\nfunc B() {}\nfunc C() {}\n")
		}},
		{"deletion", func() {
			if err := os.Remove(filepath.Join(root, "b.go")); err != nil {
				t.Fatal(err)
			}
		}},
		{"configuration", func() { writeSource(t, root, "go.mod", "module example.com/warm\n\ngo 1.26.1\n") }},
	} {
		t.Run(step.name, func(t *testing.T) {
			step.edit()
			state, err := gitstate.CaptureQuick(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			snap, _, _, live, err := e.scanWatched(ctx, root, state, revision)
			if err != nil || len(snap.GetWarnings()) > 0 {
				t.Fatalf("watch: %v %v", err, snap.GetWarnings())
			}
			revision = state.Revision
			stored, err := idx.LoadGraph(ctx, snap.Id)
			if err != nil {
				t.Fatal(err)
			}
			fullSnap, full, err := (indexer.Pipeline{Config: cfg, RepositoryID: e.repoID}).Build(ctx, &pb.IndexRequest{Directory: root}, nil)
			if err != nil || len(fullSnap.GetWarnings()) > 0 {
				t.Fatalf("full SCIP: %v %v", err, fullSnap.GetWarnings())
			}
			if graphDigest(snap, stored) != graphDigest(fullSnap, full) {
				t.Fatal("published warm graph differs from full SCIP")
			}
			legacy, err := idx.ImpactDiff(ctx, live.Diff.FromSnapshotId, snap.Id)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(live.Diff, legacy) {
				t.Fatalf("live diff differs from persisted pipeline:\nnew %v\nold %v", live.Diff, legacy)
			}
			counts, kinds, err := idx.FilePairStats(ctx, snap.Id)
			if err != nil {
				t.Fatal(err)
			}
			gotCounts, gotKinds := cstore.GraphFilePairStats(e.cache.Base.Graph)
			if !reflect.DeepEqual(counts, gotCounts) || !reflect.DeepEqual(kinds, gotKinds) {
				t.Fatal("retained dependency statistics differ from database")
			}
			service := impact.Service{Workspace: ws, Index: idx}
			for radius := uint32(0); radius <= impact.MaxBlastRadius; radius++ {
				warm, err := service.CompareGraphs(ctx, e.baseline.Snapshot, snap, e.baseline.Graph, e.cache.Base.Graph, radius)
				if err != nil {
					t.Fatal(err)
				}
				// Remove both cache layers so the original SQL pipeline must recompute.
				if _, err = ws.BunDB().NewRaw("DELETE FROM codeindex_impacts WHERE repository_id = ? AND comparison_key <> 'live'", snap.RepositoryId).Exec(ctx); err != nil {
					t.Fatal(err)
				}
				original, err := service.CompareSnapshots(ctx, snap.RepositoryId, e.baseline.Snapshot.Id, snap.Id, radius)
				if err != nil {
					t.Fatal(err)
				}
				normalize := func(d *model.Diagram) string {
					d = model.Clone(d)
					d.Version = ""
					d.Scene.Version = ""
					raw, err := json.Marshal(d)
					if err != nil {
						t.Fatal(err)
					}
					return string(raw)
				}
				if normalize(warm) != normalize(original) {
					t.Fatalf("complete scene differs from persisted pipeline at radius %d", radius)
				}
			}
			saved[snap.Id] = graphDigest(snap, stored)
			for id, want := range saved {
				historical, err := idx.Snapshot(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				g, err := idx.LoadGraph(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if graphDigest(historical, g) != want {
					t.Fatalf("historical snapshot %s changed", id)
				}
			}
		})
	}
}

// The same fixture used by the canvas latency profile, copied before editing.
func TestWarmWatchRepositoryParity(t *testing.T) {
	fixture := os.Getenv("CODEINDEX_WATCH_PARITY_REPO")
	if fixture == "" {
		t.Skip("set CODEINDEX_WATCH_PARITY_REPO to a tld checkout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	root := filepath.Join(t.TempDir(), "repo")
	if out, err := exec.CommandContext(ctx, "git", "clone", "--shared", "--quiet", fixture, root).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	path := filepath.Join(root, "cmd/index/index.go")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(original), "return s[:12]") {
		t.Fatal("fixture lacks latency edit site")
	}
	ws, err := localstore.OpenLocal(ctx, &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	cfg := config.Default()
	session := indexer.NewWatchSession(ctx, cfg, root)
	defer session.Close()
	e := &engine{store: idx, ws: ws, cfg: cfg, out: io.Discard, semantics: session, opts: options{exclude: []string{"frontend", "scripts"}}}
	revision := ""
	for _, n := range []string{"12", "11", "10", "12"} {
		if err := os.WriteFile(path, []byte(strings.Replace(string(original), "return s[:12]", "return s[:"+n+"]", 1)), 0600); err != nil {
			t.Fatal(err)
		}
		state, err := gitstate.CaptureQuick(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		snap, _, _, live, err := e.scanWatched(ctx, root, state, revision)
		if err != nil || len(snap.GetWarnings()) > 0 {
			t.Fatalf("watch: %v %v", err, snap.GetWarnings())
		}
		revision = state.Revision
		fullSnap, full, err := (indexer.Pipeline{Config: cfg, RepositoryID: e.repoID}).Build(ctx, &pb.IndexRequest{Directory: root, Exclude: e.opts.exclude}, nil)
		if err != nil || len(fullSnap.GetWarnings()) > 0 {
			t.Fatalf("SCIP: %v %v", err, fullSnap.GetWarnings())
		}
		stored, err := idx.LoadGraph(ctx, snap.Id)
		if err != nil {
			t.Fatal(err)
		}
		if graphDigest(snap, stored) != graphDigest(fullSnap, full) {
			t.Fatal("warm repository differs from full SCIP")
		}
		sqlDiff, err := idx.ImpactDiff(ctx, live.Diff.FromSnapshotId, snap.Id)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(live.Diff, sqlDiff) {
			t.Fatal("warm repository differs from original SQL diff")
		}
		t.Logf("body %s: %d facts, %d edges; full SCIP and SQL diff parity", n, len(stored.Facts), len(stored.EdgeFacts))
	}
}
