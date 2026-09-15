package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func historyGit(t *testing.T, dir string, args ...string) {
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

func historyCommit(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	historyGit(t, dir, "add", name)
	historyGit(t, dir, "commit", "-m", msg)
}

// initHistoryRepo builds: initial -> second (main), branched feature with one
// commit, merged back into main. Returns the dir.
func initHistoryRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	historyGit(t, dir, "init", "-b", "main")
	historyGit(t, dir, "config", "user.email", "test@example.com")
	historyGit(t, dir, "config", "user.name", "Test")
	historyCommit(t, dir, "a.txt", "a\n", "initial commit")
	historyCommit(t, dir, "b.txt", "b\n", "second commit")
	historyGit(t, dir, "checkout", "-b", "feature")
	historyCommit(t, dir, "c.txt", "c\n", "feature work")
	historyGit(t, dir, "checkout", "main")
	historyGit(t, dir, "merge", "--no-ff", "-m", "merge feature", "feature")
	return dir
}

func TestHistoryGraphIncludesMergesWithParentsAndRefs(t *testing.T) {
	dir := initHistoryRepo(t)

	commits, err := HistoryGraph(dir, 50)
	if err != nil {
		t.Fatalf("HistoryGraph: %v", err)
	}
	if len(commits) != 4 {
		t.Fatalf("len(commits) = %d, want 4", len(commits))
	}
	// Newest first: merge commit.
	merge := commits[0]
	if merge.Subject != "merge feature" {
		t.Errorf("commits[0].Subject = %q, want merge feature", merge.Subject)
	}
	if len(merge.Parents) != 2 {
		t.Errorf("merge parents = %v, want 2 parents", merge.Parents)
	}
	if merge.AuthorEmail != "test@example.com" {
		t.Errorf("author email = %q", merge.AuthorEmail)
	}
	if len(commits[3].Parents) != 0 {
		t.Errorf("root parents = %v, want none", commits[3].Parents)
	}
	// Current branch tip (the merge) must carry a HEAD decoration.
	foundHead := false
	for _, ref := range merge.Refs {
		if ref == "HEAD -> main" {
			foundHead = true
		}
	}
	if !foundHead {
		t.Errorf("merge refs = %v, want HEAD -> main", merge.Refs)
	}
	// Feature branch tip still exists with its ref.
	var feature *Commit
	for i := range commits {
		if commits[i].Subject == "feature work" {
			feature = &commits[i]
		}
	}
	if feature == nil {
		t.Fatal("feature work commit missing from graph")
	}
	foundFeature := false
	for _, ref := range feature.Refs {
		if ref == "feature" {
			foundFeature = true
		}
	}
	if !foundFeature {
		t.Errorf("feature refs = %v, want feature", feature.Refs)
	}
	if len(feature.Parents) != 1 {
		t.Fatalf("feature parents = %v, want 1 parent", feature.Parents)
	}
	var secondSHA string
	for i := range commits {
		if commits[i].Subject == "second commit" {
			secondSHA = commits[i].SHA
		}
	}
	if feature.Parents[0] != secondSHA {
		t.Errorf("feature parent = %v, want %s", feature.Parents, secondSHA)
	}
}

func TestHistoryGraphLimit(t *testing.T) {
	dir := initHistoryRepo(t)
	commits, err := HistoryGraph(dir, 2)
	if err != nil {
		t.Fatalf("HistoryGraph: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("len(commits) = %d, want 2", len(commits))
	}
}

func TestShowCommit(t *testing.T) {
	dir := initHistoryRepo(t)

	commits, err := HistoryGraph(dir, 50)
	if err != nil {
		t.Fatal(err)
	}
	var second *Commit
	for i := range commits {
		if commits[i].Subject == "second commit" {
			second = &commits[i]
		}
	}
	if second == nil {
		t.Fatal("second commit missing")
	}
	header, files, added, removed, err := ShowCommit(dir, second.SHA)
	if err != nil {
		t.Fatalf("ShowCommit: %v", err)
	}
	if header.Subject != "second commit" || len(header.Parents) != 1 {
		t.Errorf("header = %+v", header)
	}
	if len(files) != 1 || files[0].Path != "b.txt" || files[0].Change != WorktreeAdded {
		t.Errorf("files = %+v, want b.txt added", files)
	}
	if added != 1 || removed != 0 {
		t.Errorf("added/removed = %d/%d, want 1/0", added, removed)
	}

	merge, _, _, _, err := ShowCommit(dir, commits[0].SHA)
	if err != nil {
		t.Fatalf("ShowCommit merge: %v", err)
	}
	if len(merge.Parents) != 2 {
		t.Errorf("merge parents = %v", merge.Parents)
	}

	if _, _, _, _, err := ShowCommit(dir, "deadbeef"); err == nil {
		t.Error("ShowCommit with bogus sha should fail")
	}
}

func TestRangeCommitCount(t *testing.T) {
	dir := initHistoryRepo(t)

	count, err := RangeCommitCount(dir, "HEAD~2", "HEAD")
	if err != nil {
		t.Fatalf("RangeCommitCount: %v", err)
	}
	// HEAD~2 is the initial commit; reachable and not reachable from it are
	// the second commit, the feature commit, and the merge.
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	if _, err := RangeCommitCount(dir, "", "HEAD"); err == nil {
		t.Error("empty base should fail")
	}
}
