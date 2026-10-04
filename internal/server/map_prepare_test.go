package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

func gitFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}} {
		testGit(t, root, args...)
	}
	writeFixtureSource(t, root, "sample.go", "package sample\nfunc Run() {}\n")
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-m", "initial")
	return root, strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
}
func testGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, raw)
	}
	return string(raw)
}
func writeFixtureSource(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
func prepareFixture(t *testing.T) (*mapperService, string, string, string) {
	t.Helper()
	root, sha := gitFixture(t)
	ws, _ := newTestServer(t, uuid.New(), nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	repoID := graph.RepositoryID(root)
	seed := &pb.Snapshot{Id: "legacy-live", RepositoryId: repoID, GitRevision: sha, IngestionStatus: "complete"}
	if err := idx.Publish(context.Background(), root, seed, graph.NewGraph(repoID, seed.Id)); err != nil {
		t.Fatal(err)
	}
	return &mapperService{ws: ws, idx: idx, running: map[string]struct{}{}}, root, sha, repoID
}

func TestPrepareCommitSnapshotPreservesIdentityAndLatest(t *testing.T) {
	s, root, sha, repoID := prepareFixture(t)
	ctx := context.Background()
	send := func(*pb.MapProgress) {}
	snap, err := s.prepareSnapshot(ctx, &pb.MapRepositoryRequest{RepositoryId: repoID, GitRevision: sha, GitBranch: "main"}, send)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Id == "legacy-live" || snap.Provenance != "commit" || snap.GitRevision != sha || snap.GitBranch != "main" || snap.RepositoryId != repoID {
		t.Fatalf("snapshot = %+v", snap)
	}
	latest, err := s.idx.Latest(ctx, repoID)
	if err != nil || latest != "legacy-live" {
		t.Fatalf("latest = %s: %v", latest, err)
	}
	repos, err := s.idx.ListRepositories(ctx)
	if err != nil || len(repos) != 1 || repos[0].Root != root {
		t.Fatalf("repositories = %+v: %v", repos, err)
	}
	facts, err := s.idx.Facts(ctx, snap.Id, 0, "", "", 100)
	if err != nil || len(facts) == 0 {
		t.Fatalf("facts: %v", err)
	}
	for _, fact := range facts {
		if fact.RepositoryId != repoID {
			t.Fatal("temporary checkout changed fact identity")
		}
	}
	if strings.Count(testGit(t, root, "worktree", "list", "--porcelain"), "worktree ") != 1 {
		t.Fatal("temporary worktree remains registered")
	}
	// Reuse does not need the original checkout and does not mutate captured branch metadata.
	missing := root + "-missing"
	if err := os.Rename(root, missing); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(missing, root) })
	reused, err := s.prepareSnapshot(ctx, &pb.MapRepositoryRequest{RepositoryId: repoID, GitRevision: sha}, send)
	if err != nil || reused.Id != snap.Id {
		t.Fatalf("offline reuse: %+v: %v", reused, err)
	}
}

func TestPrepareBranchResolvesAndCapturesImmutableCommit(t *testing.T) {
	s, root, sha, repoID := prepareFixture(t)
	ctx := context.Background()
	snapshot, err := s.prepareSnapshot(ctx, &pb.MapRepositoryRequest{RepositoryId: repoID, GitRevision: "main"}, func(*pb.MapProgress) {})
	if err != nil || snapshot.GitRevision != sha || snapshot.GitBranch != "main" || snapshot.Provenance != "commit" {
		t.Fatalf("branch capture: %+v: %v", snapshot, err)
	}
	writeFixtureSource(t, root, "sample.go", "package sample\nfunc NewHead() {}\n")
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-m", "advance main")
	saved, err := s.idx.Snapshot(ctx, snapshot.Id)
	if err != nil || saved.GitRevision != sha || saved.GitBranch != "main" {
		t.Fatalf("moving ref changed snapshot provenance: %+v: %v", saved, err)
	}
}

func TestPrepareCommitAheadOfLocalBranchTip(t *testing.T) {
	s, root, _, repoID := prepareFixture(t)
	// Simulate PR review: the base branch advanced on the remote while the
	// local branch stayed behind, so the selected commit is a descendant.
	testGit(t, root, "checkout", "-b", "remote-main")
	testGit(t, root, "commit", "--allow-empty", "-m", "remote advance")
	ahead := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	testGit(t, root, "checkout", "main")
	snap, err := s.prepareSnapshot(context.Background(), &pb.MapRepositoryRequest{RepositoryId: repoID, GitRevision: ahead, GitBranch: "main"}, func(*pb.MapProgress) {})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if snap.Provenance != "commit" || snap.GitRevision != ahead || snap.GitBranch != "main" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestPrepareCommitWithMissingLocalBranch(t *testing.T) {
	s, root, _, repoID := prepareFixture(t)
	// Simulate a fetched PR head: the commit exists but its branch ref does not.
	testGit(t, root, "checkout", "-b", "pr-head")
	testGit(t, root, "commit", "--allow-empty", "-m", "pr head")
	head := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	testGit(t, root, "checkout", "main")
	testGit(t, root, "branch", "-D", "pr-head")
	snap, err := s.prepareSnapshot(context.Background(), &pb.MapRepositoryRequest{RepositoryId: repoID, GitRevision: head, GitBranch: "feature"}, func(*pb.MapProgress) {})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if snap.Provenance != "commit" || snap.GitRevision != head || snap.GitBranch != "feature" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestPrepareWorkingTreeIncludesLocalContents(t *testing.T) {
	s, root, sha, repoID := prepareFixture(t)
	ctx := context.Background()
	send := func(*pb.MapProgress) {}
	writeFixtureSource(t, root, "sample.go", "package sample\nfunc Staged() {}\n")
	testGit(t, root, "add", "sample.go")
	writeFixtureSource(t, root, "sample.go", "package sample\nfunc Unstaged() {}\n")
	writeFixtureSource(t, root, "untracked.go", "package sample\nfunc Local() {}\n")
	req := &pb.MapRepositoryRequest{RepositoryId: repoID, WorkingTree: true}
	snap, err := s.prepareSnapshot(ctx, req, send)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Provenance != "working_tree" || snap.GitRevision != sha || len(snap.Sources) != 2 || snap.ContentFingerprint == "" {
		t.Fatalf("snapshot: %+v", snap)
	}
	reused, err := s.prepareSnapshot(ctx, req, send)
	if err != nil || reused.Id != snap.Id {
		t.Fatalf("reuse: %v", err)
	}
	writeFixtureSource(t, root, "untracked.go", "package sample\nfunc Changed() {}\n")
	changed, err := s.prepareSnapshot(ctx, req, send)
	if err != nil || changed.Id == snap.Id {
		t.Fatalf("changed contents: %v", err)
	}
	if _, err := s.idx.Snapshot(ctx, changed.Id); err != nil {
		t.Fatal("changed snapshot was not persisted")
	}
}

func TestDetachedWorktreeCleanupOnFailureAndCancellation(t *testing.T) {
	root, sha := gitFixture(t)
	for _, cancel := range []bool{false, true} {
		ctx, stop := context.WithCancel(context.Background())
		var checkout string
		_, err := inDetachedWorktree(ctx, root, sha, func(path string) (*pb.Snapshot, error) {
			checkout = path
			if cancel {
				stop()
				return nil, ctx.Err()
			}
			return nil, errors.New("index failed")
		})
		stop()
		if err == nil {
			t.Fatal("expected failure")
		}
		if _, err := os.Stat(checkout); !os.IsNotExist(err) {
			t.Fatalf("checkout still exists: %v", err)
		}
		if strings.Count(testGit(t, root, "worktree", "list", "--porcelain"), "worktree ") != 1 {
			t.Fatal("worktree registration remains")
		}
	}
}

func TestIndexerRejectsInputsChangingDuringCapture(t *testing.T) {
	root, _ := gitFixture(t)
	pipeline := indexer.Pipeline{Config: configbridge.FromGlobal(nil)}
	_, _, err := pipeline.Build(context.Background(), &pb.IndexRequest{Directory: root}, func(p indexer.Progress) {
		if p.Stage == "verify" {
			writeFixtureSource(t, root, "new.go", "package sample\nfunc Late() {}\n")
		}
	})
	if err == nil || !strings.Contains(err.Error(), "changed during indexing") {
		t.Fatalf("want consistency error, got %v", err)
	}
}

func TestPrepareRejectsSnapshotFromAnotherRepository(t *testing.T) {
	s, _, _, repoID := prepareFixture(t)
	other := &pb.Snapshot{Id: "other", RepositoryId: "another-repo"}
	if err := s.idx.Publish(context.Background(), "/another", other, graph.NewGraph(other.RepositoryId, other.Id)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.prepareSnapshot(context.Background(), &pb.MapRepositoryRequest{RepositoryId: repoID, SnapshotId: other.Id}, func(*pb.MapProgress) {}); err == nil {
		t.Fatal("accepted another repository's snapshot")
	}
	if _, err := s.idx.Diff(context.Background(), "legacy-live", other.Id, false); err == nil {
		t.Fatal("accepted a cross-repository diff")
	}
}

func TestRepositoryGitHistoryAndDetails(t *testing.T) {
	s, root, initial, repoID := prepareFixture(t)
	testGit(t, root, "checkout", "-b", "feature")
	writeFixtureSource(t, root, "feature.go", "package sample\nfunc Feature() {}\n")
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-m", "feature change")
	feature := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	testGit(t, root, "tag", "v1")
	svc := &codeIndexRepositoryService{store: s.idx}
	ctx := context.Background()
	history, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repoID, Branch: "main", Limit: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if !history.Msg.IsGit || history.Msg.HeadSha != feature || history.Msg.CurrentBranch != "feature" || len(history.Msg.Commits) != 1 || history.Msg.Commits[0].Sha != initial {
		t.Fatalf("history: %+v", history.Msg)
	}
	if strings.TrimSpace(testGit(t, root, "branch", "--show-current")) != "feature" {
		t.Fatal("browsing history changed checkout branch")
	}
	history, err = svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repoID, Limit: 1}))
	if err != nil || !history.Msg.HasMore {
		t.Fatalf("pagination: %+v: %v", history, err)
	}
	if !strings.Contains(strings.Join(history.Msg.Commits[0].Refs, ","), "v1") {
		t.Fatalf("missing tag: %+v", history.Msg.Commits[0])
	}
	history, err = svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repoID}))
	if err != nil || history.Msg.HasMore || len(history.Msg.Commits) != 2 {
		t.Fatalf("full history: %+v: %v", history, err)
	}
	detail, err := svc.GetCommitDetails(ctx, connect.NewRequest(&pb.GetCommitDetailsRequest{RepositoryId: repoID, Revision: feature}))
	if err != nil || detail.Msg.Commit.Sha != feature || len(detail.Msg.Files) != 1 || detail.Msg.Files[0].Path != "feature.go" || detail.Msg.Files[0].Added != 2 {
		t.Fatalf("details: %+v: %v", detail, err)
	}
	if _, err := svc.GetCommitDetails(ctx, connect.NewRequest(&pb.GetCommitDetailsRequest{RepositoryId: repoID, Revision: "--all"})); err == nil {
		t.Fatal("accepted invalid revision")
	}
}

func TestReadCommitsWithoutLimit(t *testing.T) {
	root, _ := gitFixture(t)
	testGit(t, root, "commit", "--allow-empty", "-m", "second")
	head := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	commits, err := readCommits(context.Background(), root, head, 0)
	if err != nil || len(commits) != 2 {
		t.Fatalf("unlimited history: %d commits: %v", len(commits), err)
	}
}

func TestRepositoryHistoryForNonGitAndUnbornRepositories(t *testing.T) {
	for _, git := range []bool{false, true} {
		root := t.TempDir()
		if git {
			testGit(t, root, "init", "-b", "main")
		}
		ws, _ := newTestServer(t, uuid.New(), nil)
		idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
		snap := &pb.Snapshot{Id: "initial", RepositoryId: "repo"}
		if err := idx.Publish(context.Background(), root, snap, graph.NewGraph("repo", snap.Id)); err != nil {
			t.Fatal(err)
		}
		svc := &codeIndexRepositoryService{store: idx}
		result, err := svc.GetGitHistory(context.Background(), connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: "repo"}))
		if err != nil || result.Msg.IsGit != git || result.Msg.HeadSha != "" || len(result.Msg.Commits) != 0 {
			t.Fatalf("initial Git history (git=%v): %+v: %v", git, result, err)
		}
	}
}
