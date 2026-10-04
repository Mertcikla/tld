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
func TestCaptureQuickDetectsChanges(t *testing.T) {
	ctx := context.Background()
	root := repo(t)
	initial, err := CaptureQuick(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Paths) != 0 || initial.Revision == "" || initial.Branch != "main" {
		t.Fatalf("initial quick state: %+v", initial)
	}
	put(t, root, "a.go", "package b\n")
	put(t, root, "new.go", "package a\n")
	changed, err := CaptureQuick(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Signature() == initial.Signature() {
		t.Fatal("edit did not change the quick signature")
	}
	if !changed.Dirty["a.go"] || !changed.Dirty["new.go"] || len(changed.Paths) != 2 {
		t.Fatalf("dirty paths: %+v", changed)
	}
	if changed.StatusByPath["new.go"] != "??" {
		t.Fatalf("untracked status: %q", changed.StatusByPath["new.go"])
	}
}

func TestNameStatusReportsWorkingChanges(t *testing.T) {
	ctx := context.Background()
	root := repo(t)
	head := fixtureGit(t, root, "rev-parse", "HEAD")
	put(t, root, "a.go", "package b\n")
	if err := os.Remove(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	changes, err := NameStatus(ctx, root, head)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]byte{}
	for _, change := range changes {
		byPath[change.Path] = change.Status
	}
	if byPath["a.go"] != 'M' || byPath[".gitignore"] != 'D' {
		t.Fatalf("name status: %+v", changes)
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

func TestCaptureQuickDetectsRepeatedDirtyEdits(t *testing.T) {
	ctx := context.Background()
	root := repo(t)
	for _, path := range []string{"a.go", "untracked.go"} {
		put(t, root, path, "package b\n")
		before, err := CaptureQuick(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		put(t, root, path, "package c\n")
		if err := os.Chtimes(filepath.Join(root, path), info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		after, err := CaptureQuick(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if before.StatusByPath[path] != after.StatusByPath[path] {
			t.Fatal("fixture status changed")
		}
		if before.Signature() == after.Signature() {
			t.Fatalf("repeated edit to %s was missed", path)
		}
	}
}
