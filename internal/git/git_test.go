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
