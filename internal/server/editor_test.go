package server

import (
	"context"
	"path/filepath"
	"testing"
)

func TestResolveEditorPath(t *testing.T) {
	// For now we don't need a real store if we are testing the absolute path branch
	// but store.Repositories(ctx) will be called if we change the logic.
	
	t.Run("absolute path - currently vulnerable", func(t *testing.T) {
		path := "/etc/passwd"
		if filepath.Separator == '\\' {
			path = "C:\\Windows\\System32\\drivers\\etc\\hosts"
		}
		
		got, err := resolveEditorPath(context.Background(), nil, "", path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		if got != filepath.Clean(path) {
			t.Errorf("got %q, want %q", got, path)
		}
	})
}
