package index

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/gitstate"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestMaterializeFlagDefaultsOff(t *testing.T) {
	flag := NewIndexCmd().Flags().Lookup("materialize")
	if flag == nil || flag.DefValue != "false" {
		t.Fatal("materialize must default off")
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
	eng := &engine{store: idx, ws: sq, cfg: config.Default(), opts: options{}, repoName: "fixture", repoRoot: dir, out: out}
	state, err := gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	initial := state.Revision
	if _, _, _, err = eng.scanWatched(ctx, dir, state, ""); err != nil {
		t.Fatal(err)
	}
	writeSource(t, dir, "a.go", "package a\nfunc Committed() {}\n")
	testGit(t, dir, "add", "a.go")
	writeSource(t, dir, "a.go", "package a\nfunc Pending() {}\n")
	testGit(t, dir, "commit", "-m", "partial commit")
	state, err = gitstate.CaptureQuick(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = eng.scanWatched(ctx, dir, state, initial); err != nil {
		t.Fatal(err)
	}
	live, err := idx.Impact(ctx, graph.RepositoryID(dir), "live")
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Nodes) != 1 || live.Nodes[0].Path != "a.go" {
		t.Fatalf("impact: %+v", live)
	}
	base, err := idx.Snapshot(ctx, live.Diff.FromSnapshotId)
	if err != nil {
		t.Fatal(err)
	}
	if base.GitBranch != "main" || base.Provenance != "commit" {
		t.Fatalf("commit metadata: %+v", base)
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
	if _, _, _, err = eng.scanWatched(ctx, dir, clean, state.Revision); err != nil {
		t.Fatal(err)
	}
	empty, err := idx.Impact(ctx, graph.RepositoryID(dir), "live")
	if err != nil {
		t.Fatal(err)
	}
	if empty.ViewId != live.ViewId || len(empty.Nodes) != 0 {
		t.Fatal("revert did not clear reusable impact view")
	}
}
