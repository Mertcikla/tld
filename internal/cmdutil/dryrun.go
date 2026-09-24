package cmdutil

import (
	"fmt"
	"os"
)

// WithWorkspaceDryRun clones the workspace into a temporary directory and
// executes mutate against that clone.
func WithWorkspaceDryRun(wdir string, mutate func(cloneDir string) error) error {
	// An empty workspace dir means "current directory"; os.DirFS rejects it.
	if wdir == "" {
		wdir = "."
	}
	cloneDir, err := os.MkdirTemp("", "tld-dry-run-*")
	if err != nil {
		return fmt.Errorf("create dry-run workspace: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(cloneDir)
	}()

	if err := os.CopyFS(cloneDir, os.DirFS(wdir)); err != nil {
		return fmt.Errorf("prepare dry-run workspace: %w", err)
	}
	return mutate(cloneDir)
}
