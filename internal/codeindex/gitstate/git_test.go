package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	raw, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, raw)
	}
	return strings.TrimSpace(string(raw))
}
func put(t *testing.T, root, name, code string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
}
func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	fixtureGit(t, root, "init", "-b", "main")
	fixtureGit(t, root, "config", "user.name", "Test")
	fixtureGit(t, root, "config", "user.email", "test@example.com")
	put(t, root, "a.go", "package a\n")
	put(t, root, ".gitignore", "ignored.go\n")
	fixtureGit(t, root, "add", ".")
	fixtureGit(t, root, "commit", "-m", "initial")
	return root
}
func TestCaptureGitChangesAndRepeatedEdits(t *testing.T) {
	ctx := context.Background()
	root := repo(t)
	initial, err := Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "a.go", "package b\n")
	first, err := Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "a.go", "package c\n")
	if err := os.Chtimes(filepath.Join(root, "a.go"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	second, err := Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Signature == second.Signature || initial.Signature == first.Signature {
		t.Fatal("same-status edit was missed")
	}
	fixtureGit(t, root, "add", "a.go")
	put(t, root, "a.go", "package d\n")
	put(t, root, "strange\nname.go", "package a\n")
	put(t, root, "ignored.go", "package a\n")
	state, err := Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty["a.go"] || !state.Dirty["strange\nname.go"] || state.Dirty["ignored.go"] {
		t.Fatalf("dirty paths: %v", state.Dirty)
	}
	fixtureGit(t, root, "reset", "--hard", "HEAD")
	if err := os.Remove(filepath.Join(root, "strange\nname.go")); err != nil {
		t.Fatal(err)
	}
	reverted, err := Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.Signature != initial.Signature {
		t.Fatal("revert differs from initial state")
	}
}
func TestCommitsIncludesBurstAndWorktreeCleanup(t *testing.T) {
	ctx := context.Background()
	root := repo(t)
	before := fixtureGit(t, root, "rev-parse", "HEAD")
	for _, name := range []string{"One", "Two"} {
		put(t, root, "a.go", "package a\nfunc "+name+"(){}\n")
		fixtureGit(t, root, "add", ".")
		fixtureGit(t, root, "commit", "-m", name)
	}
	head := fixtureGit(t, root, "rev-parse", "HEAD")
	commits, err := Commits(ctx, root, before, head)
	if err != nil || len(commits) != 2 || commits[1] != head {
		t.Fatalf("commits: %v %v", commits, err)
	}
	err = WithCommit(ctx, root, before, func(checkout string) error {
		raw, e := os.ReadFile(filepath.Join(checkout, "a.go"))
		if e != nil {
			return e
		}
		if string(raw) != "package a\n" {
			t.Fatal("wrong committed content")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(fixtureGit(t, root, "worktree", "list", "--porcelain"), "worktree ") != 1 {
		t.Fatal("worktree leaked")
	}
}
