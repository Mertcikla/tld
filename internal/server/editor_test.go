package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type mockStore struct {
	repos []repositoryRef
	err   error
}

func (m *mockStore) Repositories(ctx context.Context) ([]repositoryRef, error) {
	return m.repos, m.err
}

func TestResolveEditorPath(t *testing.T) {
	repos := []repositoryRef{
		{Root: "/a/project1"},
		{Root: "/b/project2"},
	}
	if filepath.Separator == '\\' {
		repos = []repositoryRef{
			{Root: "C:\\a\\project1"},
			{Root: "C:\\b\\project2"},
		}
	}

	store := &mockStore{repos: repos}

	t.Run("absolute path inside repository", func(t *testing.T) {
		path := filepath.Join(repos[0].Root, "src", "main.go")
		got, err := resolveEditorPath(context.Background(), store, "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != path {
			t.Errorf("got %q, want %q", got, path)
		}
	})

	t.Run("absolute path matching repository root exactly", func(t *testing.T) {
		path := repos[1].Root
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
		expected := filepath.Join(repos[0].Root, "src", "main.go")
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("relative path with multiple repos and explicit repo match", func(t *testing.T) {
		path := "src/main.go"
		got, err := resolveEditorPath(context.Background(), store, repos[1].Root, path)
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
		_, err := resolveEditorPath(context.Background(), store, "", path)
		if err == nil {
			t.Fatal("expected error for escaping path, got nil")
		}
	})
}

func TestReadSourceFile(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "main.go")
	if err := os.WriteFile(text, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readSourceFile(text, maxSourcePreviewBytes)
	if err != nil || got != "package main\n" {
		t.Fatalf("readSourceFile = %q, %v", got, err)
	}

	binary := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(binary, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSourceFile(binary, maxSourcePreviewBytes); err == nil {
		t.Fatal("expected binary file to be rejected")
	}

	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSourceFile(large, 2); err == nil {
		t.Fatal("expected oversized file to be rejected")
	}
}
