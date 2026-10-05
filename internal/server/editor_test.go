package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestWorktreeSourceRPC(t *testing.T) {
	workspaceID := uuid.New()
	sq, routes := newTestServerWithOptions(t, workspaceID, nil, Options{PublicURL: "https://diagram.example.com"})
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside repository"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	if err := idx.EnsureRepositoryIdentity(ctx, "repo", root, "", "", false); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(server.Client(), server.URL+"/api")
	for _, req := range []*codeindexv1.GetWorktreeSourceRequest{
		{RepositoryId: "repo", FilePath: "main.go"},
		{Repo: root, FilePath: "main.go"},
		{RepositoryId: "repo", FilePath: filepath.Join(root, "main.go")},
	} {
		response, err := client.GetWorktreeSource(ctx, connect.NewRequest(req))
		if err != nil {
			t.Fatal(err)
		}
		if response.Msg.Content != "package main\n" || response.Msg.Path != filepath.Join(root, "main.go") {
			t.Fatalf("unexpected source: %+v", response.Msg)
		}
	}
	for _, path := range []string{"", "../secret.txt", filepath.Join(outside, "secret.txt")} {
		_, err := client.GetWorktreeSource(ctx, connect.NewRequest(&codeindexv1.GetWorktreeSourceRequest{RepositoryId: "repo", FilePath: path}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("path %q: expected invalid argument, got %v", path, err)
		}
	}
	t.Run("symlink confinement", func(t *testing.T) {
		for name, target := range map[string]string{"linked.go": filepath.Join(outside, "secret.txt"), "linked-dir": outside, "inside.go": "main.go"} {
			if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}
		for _, path := range []string{"linked.go", "linked-dir/secret.txt"} {
			_, err := client.GetWorktreeSource(ctx, connect.NewRequest(&codeindexv1.GetWorktreeSourceRequest{RepositoryId: "repo", FilePath: path}))
			if err == nil {
				t.Fatalf("source escaped through %s", path)
			}
		}
		response, err := client.GetWorktreeSource(ctx, connect.NewRequest(&codeindexv1.GetWorktreeSourceRequest{RepositoryId: "repo", FilePath: "inside.go"}))
		if err != nil || response.Msg.Content != "package main\n" {
			t.Fatalf("internal symlink: %v", err)
		}
	})
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/editor/source", nil))
	if recorder.Code == http.StatusOK {
		t.Fatal("legacy REST source endpoint is still registered")
	}
}

type mockStore struct {
	repos []repolink.Repository
	err   error
}

func (m *mockStore) Repositories(ctx context.Context) ([]repolink.Repository, error) {
	return m.repos, m.err
}

func TestResolveEditorPath(t *testing.T) {
	repos := []repolink.Repository{
		{Root: "/a/project1"},
		{Root: "/b/project2"},
	}
	if filepath.Separator == '\\' {
		repos = []repolink.Repository{
			{Root: "C:\\a\\project1"},
			{Root: "C:\\b\\project2"},
		}
	}

	store := &mockStore{repos: repos}

	t.Run("absolute path inside repository", func(t *testing.T) {
		path := filepath.Join(repos[0].Root, "src", "main.go")
		got, err := resolveEditorPath(context.Background(), store, "", "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != path {
			t.Errorf("got %q, want %q", got, path)
		}
	})

	t.Run("absolute path matching repository root exactly", func(t *testing.T) {
		path := repos[1].Root
		got, err := resolveEditorPath(context.Background(), store, "", "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != path {
			t.Errorf("got %q, want %q", got, path)
		}
	})

	t.Run("absolute path outside repositories", func(t *testing.T) {
		path := "/etc/passwd"
		if filepath.Separator == '\\' {
			path = "C:\\Windows\\System32\\drivers\\etc\\hosts"
		}

		_, err := resolveEditorPath(context.Background(), store, "", "", path)
		if err == nil {
			t.Fatal("expected error for path outside repository, got nil")
		}
		expectedErr := "absolute file_path must reside within a watched repository"
		if err.Error() != expectedErr {
			t.Errorf("got error %q, want %q", err.Error(), expectedErr)
		}
	})

	t.Run("relative path with single repo", func(t *testing.T) {
		singleStore := &mockStore{repos: repos[:1]}
		path := "src/main.go"
		got, err := resolveEditorPath(context.Background(), singleStore, "", "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(repos[0].Root, "src", "main.go")
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("relative path with multiple repos and explicit repo match", func(t *testing.T) {
		path := "src/main.go"
		got, err := resolveEditorPath(context.Background(), store, "", repos[1].Root, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(repos[1].Root, "src", "main.go")
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("relative path escaping repository", func(t *testing.T) {
		path := "../outside.go"
		_, err := resolveEditorPath(context.Background(), store, "", "", path)
		if err == nil {
			t.Fatal("expected error for escaping path, got nil")
		}
	})
}

func TestResolveEditorPathByRepositoryReference(t *testing.T) {
	repos := []repolink.Repository{
		{ID: "repo-a", Root: "/a/project1", RemoteURL: "https://github.com/owner/alpha"},
		{ID: "repo-b", Root: "/b/project2", RemoteURL: "https://github.com/owner/beta"},
	}
	store := &mockStore{repos: repos}

	t.Run("explicit repository id", func(t *testing.T) {
		got, err := resolveEditorPath(context.Background(), store, "repo-b", "", "src/main.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join("/b/project2", "src", "main.go"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("github slug resolves remote", func(t *testing.T) {
		got, err := resolveEditorPath(context.Background(), store, "", "owner/beta", "src/main.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join("/b/project2", "src", "main.go"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("remote url resolves", func(t *testing.T) {
		got, err := resolveEditorPath(context.Background(), store, "", "git@github.com:owner/alpha.git", "src/main.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join("/a/project1", "src", "main.go"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("unknown reference with multiple repos fails", func(t *testing.T) {
		if _, err := resolveEditorPath(context.Background(), store, "", "owner/missing", "src/main.go"); err == nil {
			t.Fatal("expected error for unresolvable repository reference")
		}
	})
}

func TestReadSourceFile(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "main.go")
	if err := os.WriteFile(text, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readSourceFile(dir, "main.go", maxSourcePreviewBytes)
	if err != nil || got != "package main\n" {
		t.Fatalf("readSourceFile = %q, %v", got, err)
	}

	binary := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(binary, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSourceFile(dir, "blob.bin", maxSourcePreviewBytes); err == nil {
		t.Fatal("expected binary file to be rejected")
	}

	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSourceFile(dir, "large.txt", 2); err == nil {
		t.Fatal("expected oversized file to be rejected")
	}
}
