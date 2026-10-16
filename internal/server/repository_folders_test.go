package server

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
)

func TestBrowseRepositoryFolders(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z-project", "a-project", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	service := &repositoryService{}
	_, handler := codeindexv1connect.NewRepositoryServiceHandler(service)
	server := httptest.NewServer(handler)
	defer server.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(server.Client(), server.URL)
	ctx := context.Background()
	result, err := client.BrowseRepositoryFolders(ctx, connect.NewRequest(&pb.BrowseRepositoryFoldersRequest{Path: root}))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Msg.Path != canonical || result.Msg.ParentPath != filepath.Dir(canonical) || !slices.Equal(result.Msg.Folders, []string{".hidden", "a-project", "z-project"}) {
		t.Fatalf("unexpected folder listing: %v", result.Msg)
	}
	for _, invalid := range []string{file, filepath.Join(root, "missing")} {
		if _, err := client.BrowseRepositoryFolders(ctx, connect.NewRequest(&pb.BrowseRepositoryFoldersRequest{Path: invalid})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("browse %s: %v", invalid, err)
		}
	}
	t.Run("directory symlink", func(t *testing.T) {
		if err := os.Symlink(filepath.Join(root, "a-project"), filepath.Join(root, "linked-project")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		listing, err := client.BrowseRepositoryFolders(ctx, connect.NewRequest(&pb.BrowseRepositoryFoldersRequest{Path: root}))
		if err != nil || !slices.Contains(listing.Msg.GetFolders(), "linked-project") {
			t.Fatalf("directory symlink missing: %v %v", listing, err)
		}
		selected, err := client.BrowseRepositoryFolders(ctx, connect.NewRequest(&pb.BrowseRepositoryFoldersRequest{Path: filepath.Join(root, "linked-project")}))
		if err != nil || selected.Msg.Path != filepath.Join(canonical, "a-project") {
			t.Fatalf("directory symlink not resolved: %v %v", selected, err)
		}
	})
	t.Run("self-hosted", func(t *testing.T) {
		service.selfHosted = true
		if _, err := client.BrowseRepositoryFolders(ctx, connect.NewRequest(&pb.BrowseRepositoryFoldersRequest{Path: root})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("self-hosted browse should be disabled: %v", err)
		}
	})
}

func TestRepositoryPreflightWarningsAreAdvisory(t *testing.T) {
	root := t.TempDir()
	service := &repositoryService{}
	ctx := context.Background()
	check := func(path string, warningCount int) {
		t.Helper()
		result, err := service.CheckRepositoryIndexers(ctx, connect.NewRequest(&pb.CheckRepositoryIndexersRequest{Path: path}))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Msg.Ready || len(result.Msg.Indexers) != 0 || len(result.Msg.Warnings) != warningCount {
			t.Fatalf("warnings should preserve readiness: %v", result.Msg)
		}
	}
	// Neither Git nor project markers are required by the preflight check.
	check(root, 2)
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	commit := func(root string) {
		t.Helper()
		if out, err := exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "initial").CombinedOutput(); err != nil {
			t.Fatalf("git commit: %s %v", out, err)
		}
	}
	commit(root)
	check(root, 1)
	child := filepath.Join(root, "subdirectory")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	// A Git repository in a parent directory does not satisfy the root check.
	check(child, 2)
	// Git worktrees and separate Git directories use a .git file, not a directory.
	separateRoot := filepath.Join(t.TempDir(), "checkout")
	gitDir := filepath.Join(t.TempDir(), "gitdir")
	if out, err := exec.Command("git", "init", "--separate-git-dir", gitDir, separateRoot).CombinedOutput(); err != nil {
		t.Fatalf("git init with separate git directory: %s %v", out, err)
	}
	commit(separateRoot)
	check(separateRoot, 1)
}
