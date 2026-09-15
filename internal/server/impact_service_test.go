package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/pkg/api"
)

func impactGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initImpactRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	impactGit(t, dir, "init", "-b", "main")
	impactGit(t, dir, "config", "user.email", "test@example.com")
	impactGit(t, dir, "config", "user.name", "Test")
	impactGit(t, dir, "remote", "add", "origin", "https://github.com/example/sample.git")
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	impactGit(t, dir, "add", "-A")
	impactGit(t, dir, "commit", "-m", "initial")
}

type impactHarness struct {
	service *impactService
	ctx     context.Context
	store   *localstore.SQLiteStore
}

func newImpactHarness(t *testing.T) impactHarness {
	t.Helper()
	sqliteStore, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })
	adapter := localstore.NewAPIAdapter(sqliteStore)
	ctx := api.WithWorkspaceID(context.Background(), uuid.New())
	if _, err := adapter.EnsureRootViewID(ctx); err != nil {
		if _, createErr := adapter.CreateView(ctx, api.WorkspaceIDFromCtx(ctx), nil, "Workspace", nil, true); createErr != nil {
			t.Fatalf("create root view: %v", createErr)
		}
	}
	return impactHarness{
		service: &impactService{
			store:      adapter,
			watchStore: watch.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect()),
		},
		ctx:   ctx,
		store: sqliteStore,
	}
}

func (h impactHarness) addElement(t *testing.T, name, kind, repo, filePath string) *diagv1.Element {
	t.Helper()
	element, err := h.service.store.CreateElement(h.ctx, api.WorkspaceIDFromCtx(h.ctx), api.ElementInput{
		Name:     name,
		Kind:     strPtr(kind),
		Repo:     strPtr(repo),
		FilePath: strPtr(filePath),
	})
	if err != nil {
		t.Fatalf("create element %s: %v", name, err)
	}
	if rootViewID, err := h.service.store.EnsureRootViewID(h.ctx); err == nil {
		if _, err := h.service.store.AddPlacement(h.ctx, rootViewID, element.GetId(), 0, 0); err != nil {
			t.Fatalf("place element %s: %v", name, err)
		}
	}
	return element
}

func (h impactHarness) addRepository(t *testing.T, path, name string) *diagv1.Repository {
	t.Helper()
	resp, err := h.service.AddRepository(h.ctx, connect.NewRequest(&diagv1.AddRepositoryRequest{
		Path: path, Name: strPtr(name),
	}))
	if err != nil {
		t.Fatalf("AddRepository: %v", err)
	}
	return resp.Msg.GetRepository()
}

func TestImpactServiceAnalyzeImpact(t *testing.T) {
	h := newImpactHarness(t)
	repo := t.TempDir()
	initImpactRepo(t, repo, map[string]string{"src/a.go": "package src\n"})
	h.addRepository(t, repo, "Sample")
	h.addElement(t, "Core", "component", "https://github.com/example/sample.git", "src/**")

	if err := os.WriteFile(filepath.Join(repo, "src", "b.go"), []byte("package src\n"), 0600); err != nil {
		t.Fatal(err)
	}
	impactGit(t, repo, "add", "-A")
	impactGit(t, repo, "commit", "-m", "add b")

	resp, err := h.service.AnalyzeImpact(h.ctx, connect.NewRequest(&diagv1.AnalyzeImpactRequest{
		Path: repo, Base: "HEAD~1", Head: "HEAD",
	}))
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}
	if len(resp.Msg.GetChanged()) != 1 || resp.Msg.GetChanged()[0].GetName() != "Core" {
		t.Fatalf("changed = %+v, want Core", resp.Msg.GetChanged())
	}
	if resp.Msg.GetCoverage().GetPercent() != 100 || !resp.Msg.GetCoverage().GetComplete() {
		t.Fatalf("coverage = %+v, want 100/complete", resp.Msg.GetCoverage())
	}
	if resp.Msg.GetChanged()[0].GetElementId() == 0 {
		t.Fatalf("changed element should carry an element id: %+v", resp.Msg.GetChanged()[0])
	}
}

func TestImpactServiceAnalyzeImpactReportsCoverageGap(t *testing.T) {
	h := newImpactHarness(t)
	repo := t.TempDir()
	initImpactRepo(t, repo, map[string]string{"src/a.go": "package src\n"})
	h.addRepository(t, repo, "Sample")
	h.addElement(t, "Core", "component", "https://github.com/example/sample.git", "src/**")

	if err := os.MkdirAll(filepath.Join(repo, "internal"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal", "risk.go"), []byte("package internal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	impactGit(t, repo, "add", "-A")
	impactGit(t, repo, "commit", "-m", "add risk")

	resp, err := h.service.AnalyzeImpact(h.ctx, connect.NewRequest(&diagv1.AnalyzeImpactRequest{
		Path: repo, Base: "HEAD~1", Head: "HEAD",
	}))
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}
	if resp.Msg.GetCoverage().GetComplete() {
		t.Fatalf("coverage should be incomplete: %+v", resp.Msg.GetCoverage())
	}
	gaps := resp.Msg.GetCoverage().GetGaps()
	if len(gaps) != 1 || gaps[0].GetFile() != "internal/risk.go" {
		t.Fatalf("gaps = %+v, want internal/risk.go", gaps)
	}
}

func TestImpactServicePersistsAndReloadsLatestRun(t *testing.T) {
	h := newImpactHarness(t)
	repo := t.TempDir()
	initImpactRepo(t, repo, map[string]string{"src/a.go": "package src\n"})
	h.addRepository(t, repo, "Sample")
	h.addElement(t, "Core", "component", "https://github.com/example/sample.git", "src/**")

	if err := os.WriteFile(filepath.Join(repo, "src", "b.go"), []byte("package src\n"), 0600); err != nil {
		t.Fatal(err)
	}
	impactGit(t, repo, "add", "-A")
	impactGit(t, repo, "commit", "-m", "add b")

	analyzed, err := h.service.AnalyzeImpact(h.ctx, connect.NewRequest(&diagv1.AnalyzeImpactRequest{
		Path: repo, Base: "HEAD~1", Head: "HEAD",
	}))
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}

	latest, err := h.service.GetLatestImpact(h.ctx, connect.NewRequest(&diagv1.GetLatestImpactRequest{Path: repo}))
	if err != nil {
		t.Fatalf("GetLatestImpact: %v", err)
	}
	if !latest.Msg.GetFound() {
		t.Fatal("expected a persisted run")
	}
	report := latest.Msg.GetReport()
	if report.GetBase() != analyzed.Msg.GetBase() || report.GetHead() != analyzed.Msg.GetHead() {
		t.Fatalf("reloaded range = %s..%s, want %s..%s", report.GetBase(), report.GetHead(), analyzed.Msg.GetBase(), analyzed.Msg.GetHead())
	}
	if len(report.GetChanged()) != 1 || report.GetChanged()[0].GetName() != "Core" {
		t.Fatalf("reloaded changed = %+v, want Core", report.GetChanged())
	}
	if report.GetChanged()[0].GetElementId() == 0 {
		t.Fatalf("reloaded element should keep its element id: %+v", report.GetChanged()[0])
	}
	if report.GetCoverage().GetPercent() != analyzed.Msg.GetCoverage().GetPercent() {
		t.Fatalf("reloaded coverage = %d, want %d", report.GetCoverage().GetPercent(), analyzed.Msg.GetCoverage().GetPercent())
	}
}

func TestImpactServiceLatestRunMissing(t *testing.T) {
	h := newImpactHarness(t)
	latest, err := h.service.GetLatestImpact(h.ctx, connect.NewRequest(&diagv1.GetLatestImpactRequest{Path: t.TempDir()}))
	if err != nil {
		t.Fatalf("GetLatestImpact: %v", err)
	}
	if latest.Msg.GetFound() {
		t.Fatal("expected no persisted run")
	}
}

func TestImpactServiceAddListRemoveRepository(t *testing.T) {
	h := newImpactHarness(t)
	repo := t.TempDir()
	initImpactRepo(t, repo, map[string]string{"main.go": "package main\n"})

	added := h.addRepository(t, repo, "Sample Repo")
	if added.GetName() != "Sample Repo" || added.GetRef() == "" || added.GetElementId() == 0 {
		t.Fatalf("unexpected repository: %+v", added)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if added.GetLocalPath() != resolvedRepo {
		t.Fatalf("local path = %q, want %q", added.GetLocalPath(), resolvedRepo)
	}

	listed, err := h.service.ListRepositories(h.ctx, connect.NewRequest(&diagv1.ListRepositoriesRequest{}))
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(listed.Msg.GetRepositories()) != 1 {
		t.Fatalf("repositories = %+v, want 1", listed.Msg.GetRepositories())
	}

	if _, err := h.service.UpdateRepository(h.ctx, connect.NewRequest(&diagv1.UpdateRepositoryRequest{
		Ref: added.GetRef(), Branch: "develop",
	})); err != nil {
		t.Fatalf("UpdateRepository: %v", err)
	}
	status, err := h.service.GetRepositoryStatus(h.ctx, connect.NewRequest(&diagv1.GetRepositoryStatusRequest{Ref: added.GetRef()}))
	if err != nil {
		t.Fatalf("GetRepositoryStatus: %v", err)
	}
	if status.Msg.GetRepository().GetBranch() != "develop" {
		t.Fatalf("branch = %q, want develop", status.Msg.GetRepository().GetBranch())
	}
	if len(status.Msg.GetCommits()) == 0 {
		t.Fatal("expected commits for a local checkout")
	}

	if _, err := h.service.RemoveRepository(h.ctx, connect.NewRequest(&diagv1.RemoveRepositoryRequest{Ref: added.GetRef()})); err != nil {
		t.Fatalf("RemoveRepository: %v", err)
	}
	listed, err = h.service.ListRepositories(h.ctx, connect.NewRequest(&diagv1.ListRepositoriesRequest{}))
	if err != nil {
		t.Fatalf("ListRepositories after remove: %v", err)
	}
	if len(listed.Msg.GetRepositories()) != 0 {
		t.Fatalf("repositories after remove = %+v", listed.Msg.GetRepositories())
	}
}

func TestImpactServiceLinksCheckoutToExistingElement(t *testing.T) {
	h := newImpactHarness(t)
	repo := t.TempDir()
	initImpactRepo(t, repo, map[string]string{"main.go": "package main\n"})
	element := h.addElement(t, "Legacy Repo", "repository", "https://github.com/example/sample.git", "")

	resp, err := h.service.UpdateRepository(h.ctx, connect.NewRequest(&diagv1.UpdateRepositoryRequest{
		Ref: elementRef(element.GetId()), Path: strPtr(repo),
	}))
	if err != nil {
		t.Fatalf("UpdateRepository link: %v", err)
	}
	if resp.Msg.GetRepository().GetLocalPath() == "" {
		t.Fatalf("expected a linked local path: %+v", resp.Msg.GetRepository())
	}
}
func TestImpactServiceListsUnlinkedRepositoryElement(t *testing.T) {
	h := newImpactHarness(t)
	h.addElement(t, "Legacy Repo", "repository", "https://github.com/example/legacy.git", "")

	listed, err := h.service.ListRepositories(h.ctx, connect.NewRequest(&diagv1.ListRepositoriesRequest{}))
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(listed.Msg.GetRepositories()) != 1 || listed.Msg.GetRepositories()[0].GetName() != "Legacy Repo" {
		t.Fatalf("repositories = %+v, want Legacy Repo", listed.Msg.GetRepositories())
	}
	if listed.Msg.GetRepositories()[0].GetLocalPath() != "" {
		t.Fatalf("unlinked repository should have no local path")
	}
}

func impactSecondCommit(t *testing.T, repo string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "src", "b.go"), []byte("package src\n"), 0600); err != nil {
		t.Fatal(err)
	}
	impactGit(t, repo, "add", "-A")
	impactGit(t, repo, "commit", "-m", "add b")
}

func TestImpactServiceCommitGraphDetailsAndRangeStats(t *testing.T) {
	h := newImpactHarness(t)
	repo := t.TempDir()
	initImpactRepo(t, repo, map[string]string{"src/a.go": "package src\n"})
	impactSecondCommit(t, repo)

	graph, err := h.service.ListCommitGraph(h.ctx, connect.NewRequest(&diagv1.ListCommitGraphRequest{Path: repo, Limit: 50}))
	if err != nil {
		t.Fatalf("ListCommitGraph: %v", err)
	}
	commits := graph.Msg.GetCommits()
	if len(commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(commits))
	}
	if len(commits[1].GetParents()) != 0 {
		t.Errorf("root parents = %v, want none", commits[1].GetParents())
	}
	if len(commits[0].GetParents()) != 1 || commits[0].GetParents()[0] != commits[1].GetSha() {
		t.Errorf("head parents = %v, want [%s]", commits[0].GetParents(), commits[1].GetSha())
	}
	if commits[0].GetAuthorEmail() == "" {
		t.Errorf("expected author email: %+v", commits[0])
	}

	details, err := h.service.GetCommitDetails(h.ctx, connect.NewRequest(&diagv1.GetCommitDetailsRequest{Path: repo, Sha: commits[0].GetSha()}))
	if err != nil {
		t.Fatalf("GetCommitDetails: %v", err)
	}
	if len(details.Msg.GetFiles()) != 1 || details.Msg.GetFiles()[0].GetPath() != "src/b.go" {
		t.Fatalf("files = %+v, want src/b.go", details.Msg.GetFiles())
	}
	if details.Msg.GetAdded() != 1 || details.Msg.GetRemoved() != 0 {
		t.Fatalf("added/removed = %d/%d, want 1/0", details.Msg.GetAdded(), details.Msg.GetRemoved())
	}

	stats, err := h.service.GetRangeStats(h.ctx, connect.NewRequest(&diagv1.GetRangeStatsRequest{Path: repo, Base: "HEAD~1", Head: "HEAD"}))
	if err != nil {
		t.Fatalf("GetRangeStats: %v", err)
	}
	if stats.Msg.GetCommits() != 1 || stats.Msg.GetFilesChanged() != 1 || stats.Msg.GetAdded() != 1 {
		t.Fatalf("range stats = %+v, want 1 commit / 1 file / 1 added", stats.Msg)
	}

	if _, err := h.service.GetCommitDetails(h.ctx, connect.NewRequest(&diagv1.GetCommitDetailsRequest{Path: repo, Sha: "deadbeef"})); err == nil {
		t.Error("GetCommitDetails with bogus sha should fail")
	}
	if _, err := h.service.GetRangeStats(h.ctx, connect.NewRequest(&diagv1.GetRangeStatsRequest{Path: repo, Base: "", Head: "HEAD"})); err == nil {
		t.Error("GetRangeStats with empty base should fail")
	}
}
