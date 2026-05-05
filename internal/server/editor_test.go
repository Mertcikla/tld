package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/internal/watch"
)

type mockStore struct {
	repos []watch.Repository
	err   error
}

func (m *mockStore) Repositories(ctx context.Context) ([]watch.Repository, error) {
	return m.repos, m.err
}

func TestResolveEditorPath(t *testing.T) {
	repos := []watch.Repository{
		{RepoRoot: "/a/project1"},
		{RepoRoot: "/b/project2"},
	}
	if filepath.Separator == '\\' {
		repos = []watch.Repository{
			{RepoRoot: "C:\\a\\project1"},
			{RepoRoot: "C:\\b\\project2"},
		}
	}

	store := &mockStore{repos: repos}

	t.Run("absolute path inside repository", func(t *testing.T) {
		path := filepath.Join(repos[0].RepoRoot, "src", "main.go")
		got, err := resolveEditorPath(context.Background(), store, "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != path {
			t.Errorf("got %q, want %q", got, path)
		}
	})

	t.Run("absolute path matching repository root exactly", func(t *testing.T) {
		path := repos[1].RepoRoot
		got, err := resolveEditorPath(context.Background(), store, "", path)
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

		_, err := resolveEditorPath(context.Background(), store, "", path)
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
		got, err := resolveEditorPath(context.Background(), singleStore, "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(repos[0].RepoRoot, "src", "main.go")
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("relative path with multiple repos and explicit repo match", func(t *testing.T) {
		path := "src/main.go"
		got, err := resolveEditorPath(context.Background(), store, repos[1].RepoRoot, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(repos[1].RepoRoot, "src", "main.go")
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("relative path escaping repository", func(t *testing.T) {
		path := "../outside.go"
		_, err := resolveEditorPath(context.Background(), store, "", path)
		if err == nil {
			t.Fatal("expected error for escaping path, got nil")
		}
	})
}
