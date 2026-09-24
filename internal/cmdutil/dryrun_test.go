package cmdutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithWorkspaceDryRunEmptyDirUsesCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte("elements: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	ran := false
	err := WithWorkspaceDryRun("", func(cloneDir string) error {
		ran = true
		if filepath.Clean(cloneDir) == filepath.Clean(dir) {
			t.Fatalf("expected a clone directory, got the original %q", cloneDir)
		}
		if _, statErr := os.Stat(filepath.Join(cloneDir, "elements.yaml")); statErr != nil {
			t.Fatalf("expected cloned workspace file: %v", statErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithWorkspaceDryRun: %v", err)
	}
	if !ran {
		t.Fatal("mutate was not called")
	}
}

func TestWithWorkspaceDryRunClonesWorkspaceDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte("elements: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	err := WithWorkspaceDryRun(dir, func(cloneDir string) error {
		if filepath.Clean(cloneDir) == filepath.Clean(dir) {
			t.Fatalf("expected a clone directory, got the original %q", cloneDir)
		}
		if _, statErr := os.Stat(filepath.Join(cloneDir, "elements.yaml")); statErr != nil {
			t.Fatalf("expected cloned workspace file: %v", statErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithWorkspaceDryRun: %v", err)
	}
}
