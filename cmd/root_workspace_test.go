package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootWorkspaceDefaultPrecedence(t *testing.T) {
	t.Run("prefers .tld over tld", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".tld"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, "tld"), 0o750); err != nil {
			t.Fatal(err)
		}

		cwd, _ := os.Getwd()
		defer func() { _ = os.Chdir(cwd) }()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}

		root := NewRootCmd()
		if got := root.PersistentFlags().Lookup("workspace").DefValue; got != ".tld" {
			t.Fatalf("workspace default = %q, want %q", got, ".tld")
		}
	})

	t.Run("uses tld when .tld is missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "tld"), 0o750); err != nil {
			t.Fatal(err)
		}

		cwd, _ := os.Getwd()
		defer func() { _ = os.Chdir(cwd) }()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}

		root := NewRootCmd()
		if got := root.PersistentFlags().Lookup("workspace").DefValue; got != "tld" {
			t.Fatalf("workspace default = %q, want %q", got, "tld")
		}
	})

	t.Run("uses empty default when no workspace exists", func(t *testing.T) {
		dir := t.TempDir()
		cwd, _ := os.Getwd()
		defer func() { _ = os.Chdir(cwd) }()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}

		root := NewRootCmd()
		if got := root.PersistentFlags().Lookup("workspace").DefValue; got != "" {
			t.Fatalf("workspace default = %q, want empty string", got)
		}
	})
}
