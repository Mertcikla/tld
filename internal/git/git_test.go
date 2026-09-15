package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// initRepo creates a real git repo in dir with one commit touching the given files.
func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_AUTHOR_DATE=2024-01-01T00:00:00+00:00",
			"GIT_COMMITTER_DATE=2024-01-01T00:00:00+00:00",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		git("add", name)
	}
	git("commit", "-m", "initial commit")
}

func TestDetectBranch(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	branch, err := DetectBranch(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", branch)
	}
}

func TestListTrackedFilesStopsAtLimit(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"a.go": "package main",
		"b.go": "package main",
		"c.go": "package main",
	})

	result, err := ListTrackedFiles(dir, 2)
	if err != nil {
		t.Fatalf("ListTrackedFiles: %v", err)
	}
	if !result.Capped {
		t.Fatal("expected capped result")
	}
	if result.Total != 3 {
		t.Fatalf("Total = %d, want 3", result.Total)
	}
	if got := len(result.Files); got != 2 {
		t.Fatalf("len(Files) = %d, want 2", got)
	}
}

func TestFileBlobHashesIncludesTrackedFiles(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"a.go":             "package main\n",
		"dir/file name.go": "package main\n",
	})

	hashes, err := FileBlobHashes(dir)
	if err != nil {
		t.Fatalf("FileBlobHashes: %v", err)
	}
	for _, path := range []string{"a.go", "dir/file name.go"} {
		want, err := FileBlobHash(dir, path)
		if err != nil {
			t.Fatalf("FileBlobHash(%s): %v", path, err)
		}
		if hashes[path] != want {
			t.Fatalf("hashes[%q] = %q, want %q", path, hashes[path], want)
		}
	}
}

func TestRecentChangedFilesReturnsNewestUniqueFilesWithoutDeleted(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"a.go": "package main\n",
		"b.go": "package main\n",
	})
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc A() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "a.go")
	git("commit", "-m", "update a")
	if err := os.WriteFile(filepath.Join(dir, "c.go"), []byte("package main\nfunc C() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "c.go")
	git("commit", "-m", "add c")
	if err := os.Remove(filepath.Join(dir, "b.go")); err != nil {
		t.Fatal(err)
	}
	git("add", "b.go")
	git("commit", "-m", "delete b")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc A2() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "a.go")
	git("commit", "-m", "update a again")

	got, err := RecentChangedFiles(dir, 3)
	if err != nil {
		t.Fatalf("RecentChangedFiles: %v", err)
	}
	want := []string{"a.go", "c.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("RecentChangedFiles = %+v, want %+v", got, want)
	}
}

func TestEnsureDetachedWorktreeCreatesReusableCheckout(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main\nfunc Main() {}\n"})
	head, err := DetectHeadCommit(dir)
	if err != nil {
		t.Fatalf("DetectHeadCommit: %v", err)
	}
	target := filepath.Join(t.TempDir(), "baseline")

	if err := EnsureDetachedWorktree(dir, head, target); err != nil {
		t.Fatalf("EnsureDetachedWorktree: %v", err)
	}
	got, err := DetectHeadCommit(target)
	if err != nil {
		t.Fatalf("DetectHeadCommit(worktree): %v", err)
	}
	if got != head {
		t.Fatalf("worktree HEAD = %s, want %s", got, head)
	}
	if err := EnsureDetachedWorktree(dir, head, target); err != nil {
		t.Fatalf("EnsureDetachedWorktree reuse: %v", err)
	}
}

func TestDetectRemoteURL(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	// Add a remote
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/org/repo.git")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add remote: %v\n%s", err, out)
	}

	url, err := DetectRemoteURL(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://github.com/org/repo.git" {
		t.Errorf("expected url %q, got %q", "https://github.com/org/repo.git", url)
	}
}

func TestDetectRemoteURL_NoRemote(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	_, err := DetectRemoteURL(dir)
	if err == nil {
		t.Error("expected error when no remote configured")
	}
}

func TestFileLastCommitAt(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	ts, err := FileLastCommitAt(dir, "main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !ts.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, ts)
	}
}

func TestFileLastCommitAt_NoCommits(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	_, err := FileLastCommitAt(dir, "nonexistent.go")
	if err == nil {
		t.Error("expected error for file with no commits")
	}
}

func TestRepoRoot(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	// Create a subdirectory
	subdir := filepath.Join(dir, "pkg", "foo")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	root, err := RepoRoot(subdir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On macOS t.TempDir() may return a symlinked path; compare with Eval
	evalDir, _ := filepath.EvalSymlinks(dir)
	evalRoot, _ := filepath.EvalSymlinks(root)
	if evalRoot != evalDir {
		t.Errorf("expected root %q, got %q", evalDir, evalRoot)
	}
}

func TestFilesChangedSince(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main"})

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	head, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	base := strings.TrimSpace(string(head))

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commit := exec.Command("git", "commit", "-am", "update")
	commit.Dir = dir
	commit.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	files, err := FilesChangedSince(dir, base)
	if err != nil {
		t.Fatalf("FilesChangedSince: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "main.go" {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestFileChangesSinceClassifiesCommittedChanges(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"main.go":   "package main\nfunc Main() {}\n",
		"delete.go": "package main\nfunc DeleteMe() {}\n",
	})

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	head, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	base := strings.TrimSpace(string(head))

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("package main\nfunc Added() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "delete.go")); err != nil {
		t.Fatal(err)
	}
	commit := exec.Command("git", "add", "-A")
	commit.Dir = dir
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	commit = exec.Command("git", "commit", "-m", "mixed changes")
	commit.Dir = dir
	commit.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	changes, err := FileChangesSince(dir, base)
	if err != nil {
		t.Fatalf("FileChangesSince: %v", err)
	}
	if changes["main.go"] != WorktreeUpdated || changes["added.go"] != WorktreeAdded || changes["delete.go"] != WorktreeDeleted {
		t.Fatalf("unexpected changes: %#v", changes)
	}
}

func TestParseLineHunks(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -2 +2,2 @@ func A() {
-	old
+	new
+	next
@@ -8,2 +9 @@ func B() {
-	remove
-	again
+	replace
`
	hunks := ParseLineHunks(diff)
	got := hunks["main.go"]
	if len(got) != 2 {
		t.Fatalf("expected 2 hunks, got %+v", got)
	}
	if strings.Join(intsToStrings(got[0].AddedLines), ",") != "2,3" || strings.Join(intsToStrings(got[0].RemovedLines), ",") != "2" {
		t.Fatalf("unexpected first hunk lines: %+v", got[0])
	}
	if strings.Join(intsToStrings(got[1].AddedLines), ",") != "9" || strings.Join(intsToStrings(got[1].RemovedLines), ",") != "8,9" {
		t.Fatalf("unexpected second hunk lines: %+v", got[1])
	}
}

func intsToStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitCommitAll(t *testing.T, dir, message string) string {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", message}, {"rev-parse", "HEAD"}} {
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
		if args[0] == "rev-parse" {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func TestFileChangesBetweenTwoCommits(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"main.go":   "package main\nfunc Main() {}\n",
		"delete.go": "package main\nfunc DeleteMe() {}\n",
	})
	base := gitHead(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("package main\nfunc Added() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "delete.go")); err != nil {
		t.Fatal(err)
	}
	head := gitCommitAll(t, dir, "changes")

	changes, err := FileChangesBetween(dir, base, head)
	if err != nil {
		t.Fatalf("FileChangesBetween: %v", err)
	}
	if changes["main.go"] != WorktreeUpdated || changes["added.go"] != WorktreeAdded || changes["delete.go"] != WorktreeDeleted {
		t.Fatalf("unexpected changes: %#v", changes)
	}

	if _, err := FileChangesBetween(dir, base, ""); err == nil {
		t.Fatal("expected error for empty head ref")
	}
}

func TestFileLineStatsBetween(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"main.go":   "package main\nfunc Main() {}\n",
		"delete.go": "package main\nfunc DeleteMe() {}\n",
	})
	base := gitHead(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("package main\nfunc Added() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "delete.go")); err != nil {
		t.Fatal(err)
	}
	head := gitCommitAll(t, dir, "changes")

	stats, err := FileLineStatsBetween(dir, base, head)
	if err != nil {
		t.Fatalf("FileLineStatsBetween: %v", err)
	}
	if stats["main.go"] != (LineDiff{Added: 1, Removed: 1}) {
		t.Fatalf("main.go stats = %#v", stats["main.go"])
	}
	if stats["added.go"] != (LineDiff{Added: 2, Removed: 0}) {
		t.Fatalf("added.go stats = %#v", stats["added.go"])
	}
	if stats["delete.go"] != (LineDiff{Added: 0, Removed: 2}) {
		t.Fatalf("delete.go stats = %#v", stats["delete.go"])
	}

	if _, err := FileLineStatsBetween(dir, base, ""); err == nil {
		t.Fatal("expected error for empty head ref")
	}
}

func TestMergeBaseBetween(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main\n"})
	common := gitHead(t, dir)

	gitCheckoutNew := func(branch string) {
		cmd := exec.Command("git", "checkout", "-b", branch)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("checkout %s: %v\n%s", branch, err, out)
		}
	}
	gitCheckoutNew("feature")
	if err := os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir, "feature work")

	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout main: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "main2.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir, "main work")

	base, err := MergeBaseBetween(dir, "main", "feature")
	if err != nil {
		t.Fatalf("MergeBaseBetween: %v", err)
	}
	if base != common {
		t.Fatalf("merge base = %q, want %q", base, common)
	}
}

func TestRecentCommits(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"main.go": "package main\n"})
	if err := os.WriteFile(filepath.Join(dir, "second.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	head := gitCommitAll(t, dir, "second commit")

	commits, err := RecentCommits(dir, 10)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(commits))
	}
	if commits[0].SHA != head || commits[0].Subject != "second commit" {
		t.Fatalf("unexpected newest commit: %+v", commits[0])
	}
	if commits[0].ShortSHA == "" || commits[0].Author != "Test" || commits[0].Date == "" {
		t.Fatalf("incomplete commit metadata: %+v", commits[0])
	}
}
