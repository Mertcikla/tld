package index

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/digest"
	"github.com/mertcikla/codeindex/gitstate"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func TestMapFlagDefaultsOff(t *testing.T) {
	flag := NewIndexCmd().Flags().Lookup("map")
	if flag == nil || flag.DefValue != "false" {
		t.Fatal("map must default off")
	}
}

func TestIndexMapFlagMaterializesGraphMap(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	testGit(t, dir, "config", "user.name", "Test")
	testGit(t, dir, "config", "user.email", "test@example.com")
	writeSource(t, dir, "a.go", "package a\nfunc A() {}\n")
	writeSource(t, dir, "b.go", "package a\nfunc B() {}\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")

	sq, err := localstore.OpenLocal(ctx, &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	eng := &engine{
		store: idx, ws: sq, cfg: config.Default(), global: workspace.DefaultConfig(),
		opts: options{mapGraph: true}, out: &bytes.Buffer{},
	}
	snap, _, mapRes, reused, err := eng.buildAndPublish(ctx, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reused || snap == nil || mapRes == nil || mapRes.GetViewId() == 0 || mapRes.GetRunId() == "" {
		t.Fatalf("map result = %+v reused=%v", mapRes, reused)
	}
	maps, err := idx.CompletedMaps(ctx, snap.RepositoryId)
	if err != nil || len(maps) != 1 || maps[0].Result.ViewId != mapRes.GetViewId() {
		t.Fatalf("completed maps = %+v: %v", maps, err)
	}
	if _, err := sq.ViewByID(ctx, mapRes.GetViewId()); err != nil {
		t.Fatalf("map view missing: %v", err)
	}
}
func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	raw, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, raw)
	}
	return strings.TrimSpace(string(raw))
}
func writeSource(t *testing.T, dir, name, code string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestWatchedPartialCommitAndRevert(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	testGit(t, dir, "config", "user.name", "Test")
	testGit(t, dir, "config", "user.email", "test@example.com")
	writeSource(t, dir, "a.go", "package a\nfunc Base() {}\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")
	sq, err := localstore.OpenLocal(ctx, &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	out := &bytes.Buffer{}
	eng := &engine{store: idx, ws: sq, cfg: config.Default(), opts: options{}, out: out}
	state, err := gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	initial := state.Revision
	var stages []string
	if _, _, _, _, err = eng.scanWatched(ctx, dir, state, "", func(stage string) { stages = append(stages, stage) }); err != nil {
		t.Fatal(err)
	}
	if len(stages) < 3 || stages[0] != "waiting-indexer" || stages[len(stages)-1] != "live-map" {
		t.Fatalf("watch stages: %v", stages)
	}
	_, release, err := idx.AcquireLease(ctx, eng.repoID)
	if err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	_, _, _, _, err = eng.scanWatched(blocked, dir, state, "", func(stage string) {})
	cancel()
	release()
	if err == nil {
		t.Fatal("waiting scan ignored cancellation")
	}

	writeSource(t, dir, "a.go", "package a\nfunc Committed() {}\n")
	testGit(t, dir, "add", "a.go")
	writeSource(t, dir, "a.go", "package a\nfunc Pending() {}\n")
	testGit(t, dir, "commit", "-m", "partial commit")
	state, err = gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = eng.scanWatched(ctx, dir, state, initial); err != nil {
		t.Fatal(err)
	}
	live, err := idx.Impact(ctx, eng.repoID, "live")
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Nodes) != 1 || live.Nodes[0].Path != "a.go" {
		t.Fatalf("impact: %+v", live)
	}
	comparison, err := (impact.Service{Workspace: sq, Index: idx}).Compare(ctx, impact.CompareRequest{
		RepositoryID: eng.repoID,
		Base:         &pb.Revision{SnapshotId: live.Diff.FromSnapshotId},
		Head:         &pb.Revision{SnapshotId: live.Diff.ToSnapshotId},
	})
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Version != live.Version || comparison.GetComparisonKey() == "live" {
		t.Fatal("watcher must populate the immutable default comparison cache")
	}
	base, err := idx.Snapshot(ctx, live.Diff.FromSnapshotId)
	if err != nil {
		t.Fatal(err)
	}
	if base.GitBranch != "main" || base.Provenance != "commit" {
		t.Fatalf("commit metadata: %+v", base)
	}
	if base.CommitMessage != "partial commit" {
		t.Fatalf("commit message = %q, want %q", base.CommitMessage, "partial commit")
	}
	content, err := idx.Source(ctx, base.Sources[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Committed") || strings.Contains(string(content), "Pending") {
		t.Fatal("dirty contents leaked into commit")
	}
	testGit(t, dir, "restore", "a.go")
	clean, err := gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = eng.scanWatched(ctx, dir, clean, state.Revision); err != nil {
		t.Fatal(err)
	}
	empty, err := idx.Impact(ctx, eng.repoID, "live")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Nodes) != 0 {
		t.Fatal("revert did not clear live comparison")
	}
}

func TestWatchedLSPSnapshotCommit(t *testing.T) {
	if os.Getenv("CODEINDEX_TEST_LSP") == "" {
		t.Skip("set CODEINDEX_TEST_LSP=1 for watch Go LSP persistence parity")
	}
	testWatchedSemanticSnapshotCommit(t, "go")
}

func TestWatchedTypeScriptLSPSnapshotCommit(t *testing.T) {
	if os.Getenv("CODEINDEX_TEST_TS_LSP") == "" {
		t.Skip("set CODEINDEX_TEST_TS_LSP=1 for watch TypeScript LSP persistence parity")
	}
	testWatchedSemanticSnapshotCommit(t, "typescript")
}

func TestWatchedRustLSPSnapshotCommit(t *testing.T) {
	if os.Getenv("CODEINDEX_TEST_RUST_LSP") == "" {
		t.Skip("set CODEINDEX_TEST_RUST_LSP=1 for watch Rust LSP persistence parity")
	}
	testWatchedSemanticSnapshotCommit(t, "rust")
}

func TestWatchedPyrightSnapshotCommit(t *testing.T) {
	if os.Getenv("CODEINDEX_TEST_PYRIGHT") == "" {
		t.Skip("set CODEINDEX_TEST_PYRIGHT=1 for Python watch persistence parity")
	}
	testWatchedSemanticSnapshotCommit(t, "python")
}

func testWatchedSemanticSnapshotCommit(t *testing.T, language string) {
	t.Helper()
	t.Setenv("GOWORK", "off")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	testGit(t, dir, "config", "user.name", "Test")
	testGit(t, dir, "config", "user.email", "test@example.com")
	filename := "a.go"
	initialSource := "package fixture\nfunc A() { B() }\nfunc B() {}\nfunc C() {}\n"
	switch language {
	case "python":
		filename = "a.py"
		initialSource = "def A() -> None:\n    B()\n\ndef B() -> None:\n    pass\n\ndef C() -> None:\n    pass\n"
		writeSource(t, dir, "pyproject.toml", "[project]\nname=\"watch-pyright\"\nversion=\"1.0.0\"\n")
		writeSource(t, dir, "pyrightconfig.json", `{"include":["*.py"],"typeCheckingMode":"basic"}`)
	case "rust":
		filename = "src/lib.rs"
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0700); err != nil {
			t.Fatal(err)
		}
		initialSource = "pub fn a() { B() }\n#[allow(non_snake_case)] fn B() {}\n#[allow(non_snake_case)] fn C() {}\n"
		writeSource(t, dir, "Cargo.toml", "[package]\nname=\"watch-rust-lsp\"\nversion=\"0.1.0\"\nedition=\"2021\"\n")
		writeSource(t, dir, ".gitignore", "target/\n")
		writeSource(t, dir, "Cargo.lock", "version = 4\n\n[[package]]\nname = \"watch-rust-lsp\"\nversion = \"0.1.0\"\n")
	case "typescript":
		filename = "a.ts"
		initialSource = "export function A():void { B() }\nexport function B():void {}\nexport function C():void {}\n"
		writeSource(t, dir, "package.json", `{"name":"watch-typescript-lsp","version":"1.0.0"}`)
		writeSource(t, dir, "tsconfig.json", `{"compilerOptions":{"strict":true,"target":"ES2022"},"include":["*.ts"]}`)
	default:
		writeSource(t, dir, "go.mod", "module example.com/watchlsp\n\ngo 1.26\n")
	}
	writeSource(t, dir, filename, initialSource)
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")
	initial := testGit(t, dir, "rev-parse", "HEAD")
	sq, err := localstore.OpenLocal(ctx, &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	cfg := config.Default()
	semantic := indexer.NewWatchSession(ctx, cfg, dir)
	defer semantic.Close()
	eng := &engine{store: idx, ws: sq, cfg: cfg, out: io.Discard, semantics: semantic}
	scan := func(previous string) *pb.Snapshot {
		t.Helper()
		state, err := gitstate.CaptureQuick(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		snap, _, _, _, err := eng.scanWatched(ctx, dir, state, previous)
		if err != nil || len(snap.GetWarnings()) != 0 {
			t.Fatalf("scan: %v %v", err, snap.GetWarnings())
		}
		return snap
	}
	scan("")
	changedSource := strings.Replace(initialSource, "{ B() }", "{ C() }", 1)
	if language == "python" {
		changedSource = strings.Replace(initialSource, "    B()", "    C()", 1)
	}
	writeSource(t, dir, filename, changedSource)
	dirty := scan(initial)
	if dirty.Provenance != "working_tree" || dirty.GitRevision != initial {
		t.Fatalf("dirty metadata: %+v", dirty)
	}
	saved, err := idx.LoadGraph(ctx, dirty.Id)
	if err != nil {
		t.Fatal(err)
	}
	freshSnap, fresh, err := (indexer.Pipeline{Config: cfg, RepositoryID: eng.repoID}).Build(ctx, &pb.IndexRequest{Directory: dir}, nil)
	if err != nil || len(freshSnap.GetWarnings()) != 0 {
		t.Fatalf("fresh: %v %v", err, freshSnap.GetWarnings())
	}
	fingerprint := func(snap *pb.Snapshot, g *graph.Graph) string {
		facts := make([]*pb.CodeFact, 0, len(g.Facts))
		for _, fact := range g.Facts {
			facts = append(facts, fact)
		}
		edges := make([]*pb.EdgeFact, 0, len(g.EdgeFacts))
		for _, edge := range g.EdgeFacts {
			edges = append(edges, edge)
		}
		return digest.Snapshot(snap, facts, edges)
	}
	if fingerprint(dirty, saved) != fingerprint(freshSnap, fresh) {
		t.Fatal("persisted LSP snapshot differs from SCIP")
	}
	testGit(t, dir, "add", filename)
	testGit(t, dir, "commit", "-m", "commit live edit")
	committed := scan(initial)
	if committed.Id == dirty.Id || committed.Provenance != "commit" || committed.GitRevision == initial || committed.CommitMessage != "commit live edit" {
		t.Fatalf("commit metadata: %+v", committed)
	}
	old, err := idx.Snapshot(ctx, dirty.Id)
	if err != nil || old.Provenance != "working_tree" || old.GitRevision != initial {
		t.Fatalf("dirty snapshot was mutated: %+v %v", old, err)
	}
	live, err := idx.Impact(ctx, eng.repoID, "live")
	if err != nil || len(live.Nodes) != 0 {
		t.Fatalf("commit did not clear live changes: %+v %v", live, err)
	}
}

// TestWatchedConvergesThroughParallelEdits proves edits that land between the
// change capture and indexing no longer fail the scan: the snapshot is
// published and the next capture picks up the delta incrementally.
func TestWatchedConvergesThroughParallelEdits(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	testGit(t, dir, "config", "user.name", "Test")
	testGit(t, dir, "config", "user.email", "test@example.com")
	writeSource(t, dir, "a.go", "package a\nfunc Base() {}\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")
	sq, err := localstore.OpenLocal(ctx, &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	eng := &engine{store: idx, ws: sq, cfg: config.Default(), opts: options{}, out: &bytes.Buffer{}}

	stale, err := gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// A save lands after the change capture but before indexing starts. The
	// scan used to abort here with "git inputs changed during indexing".
	writeSource(t, dir, "a.go", "package a\nfunc Edited() {}\n")
	first, _, _, live, err := eng.scanWatched(ctx, dir, stale, "")
	if err != nil {
		t.Fatalf("scan must survive edits landing before indexing: %v", err)
	}
	if first == nil || live == nil {
		t.Fatal("scan published no snapshot or live comparison")
	}
	found := false
	for _, node := range live.GetNodes() {
		if node.GetPath() == "a.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("live overlay does not carry the concurrent edit: %+v", live.GetNodes())
	}

	// The next capture reports the delta and the follow-up scan converges.
	next, err := gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if next.Signature() == stale.Signature() {
		t.Fatal("edit did not change the signature")
	}
	second, _, _, _, err := eng.scanWatched(ctx, dir, next, stale.Revision)
	if err != nil {
		t.Fatalf("follow-up scan: %v", err)
	}
	if second == nil {
		t.Fatal("follow-up scan published no snapshot")
	}
}

// TestWatchConvergesUnderContinuousEdits drives the real watcher while files
// keep changing, proving scans make progress instead of aborting and
// restarting when Git inputs move mid-index.
func TestWatchConvergesUnderContinuousEdits(t *testing.T) {
	if testing.Short() {
		t.Skip("watch integration test")
	}
	if _, err := exec.LookPath("scip-go"); err != nil {
		t.Skip("scip-go is required for the watch integration test")
	}
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	testGit(t, dir, "config", "user.name", "Test")
	testGit(t, dir, "config", "user.email", "test@example.com")
	writeSource(t, dir, "go.mod", "module example.com/watch\n\ngo 1.26\n")
	writeSource(t, dir, "a.go", "package watch\nfunc Base() {}\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")

	sq, err := localstore.OpenLocal(context.Background(), &workspace.Config{}, t.TempDir(), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	eng := &engine{
		store: idx, ws: sq, cfg: config.Default(), global: workspace.DefaultConfig(),
		dataDir: t.TempDir(), out: io.Discard, errOut: io.Discard,
		opts:      options{pollInterval: 200 * time.Millisecond, debounce: 50 * time.Millisecond},
		manifests: gitstate.ManifestCache{},
	}
	repoID := graph.RepositoryID(dir)
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()
	cmd := &cobra.Command{}
	cmd.SetContext(watchCtx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	done := make(chan error, 1)
	go func() { done <- eng.watch(watchCtx, cmd, dir) }()

	// Keep saving while the watcher indexes. Each save produces a new signature
	// the watcher must fold into the next incremental scan.
	for i := 0; i < 25; i++ {
		writeSource(t, dir, "a.go", fmt.Sprintf("package watch\nfunc Edit%d() {}\n", i))
		time.Sleep(30 * time.Millisecond)
	}
	final := "package watch\nfunc Final() {}\n"
	writeSource(t, dir, "a.go", final)
	finalHash := graph.Hash([]byte(final))
	deadline := time.Now().Add(60 * time.Second)
	converged := false
	for time.Now().Before(deadline) {
		repositoryID, _, _ := idx.RepositoryByRoot(context.Background(), dir)
		latest, latestErr := idx.Latest(context.Background(), repositoryID)
		if latestErr == nil && latest != "" {
			if snap, snapErr := idx.Snapshot(context.Background(), latest); snapErr == nil {
				for _, src := range snap.Sources {
					if src.Path == "a.go" && src.Hash == finalHash {
						converged = true
					}
				}
			}
		}
		if converged {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !converged {
		state, _, _ := idx.WatchState(context.Background(), repoID)
		t.Fatalf("watcher never indexed the final edit; state=%+v", state)
	}
	state, ok, err := idx.WatchState(context.Background(), repoID)
	if err != nil || !ok {
		t.Fatalf("watch state: ok=%v err=%v", ok, err)
	}
	if state.Error != "" {
		t.Fatalf("watcher surfaced an error after convergence: %s", state.Error)
	}
	cancelWatch()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("watch returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("watcher did not stop after cancellation")
	}
}
