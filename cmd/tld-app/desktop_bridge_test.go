package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeBase64Content(t *testing.T) {
	got, err := decodeBase64Content(base64.StdEncoding.EncodeToString([]byte("hello")))
	if err != nil {
		t.Fatalf("decodeBase64Content returned error: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want hello", string(got))
	}
}

func TestSanitizeDefaultFilename(t *testing.T) {
	if got := sanitizeDefaultFilename("../diagram.md"); got != "diagram.md" {
		t.Fatalf("filename = %q, want diagram.md", got)
	}
	if got := sanitizeDefaultFilename(""); got != "untitled" {
		t.Fatalf("empty filename = %q, want untitled", got)
	}
}

func TestReadTextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "diagram.md")
	if err := os.WriteFile(path, []byte("```mermaid\nflowchart LR\n```\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := readTextFile(path)
	if err != nil {
		t.Fatalf("readTextFile returned error: %v", err)
	}
	if got.Path != path {
		t.Fatalf("path = %q, want %q", got.Path, path)
	}
	if got.Content != "```mermaid\nflowchart LR\n```\n" {
		t.Fatalf("content = %q, want Mermaid Markdown block", got.Content)
	}
}

func TestReadTextFileRejectsDirectory(t *testing.T) {
	if _, err := readTextFile(t.TempDir()); err == nil {
		t.Fatal("readTextFile returned nil error for directory")
	}
}

func TestWritableMarkdownFileAcceptsSupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"notes.md", "notes.markdown", "notes.mdx"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("# Notes\n"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}

		got, err := writableMarkdownFile(path)
		if err != nil {
			t.Fatalf("writableMarkdownFile(%s) returned error: %v", name, err)
		}
		if got.Path != path {
			t.Fatalf("path = %q, want %q", got.Path, path)
		}
		if got.Content != "" {
			t.Fatalf("content = %q, want empty content", got.Content)
		}
	}
}

func TestWritableMarkdownFileRejectsInvalidSelections(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "notes.txt")
	readOnlyPath := filepath.Join(dir, "read-only.md")
	directoryPath := filepath.Join(dir, "directory.md")
	if err := os.WriteFile(textPath, []byte("notes"), 0o600); err != nil {
		t.Fatalf("write text fixture: %v", err)
	}
	if err := os.WriteFile(readOnlyPath, []byte("# Read only\n"), 0o400); err != nil {
		t.Fatalf("write read-only fixture: %v", err)
	}
	if err := os.Mkdir(directoryPath, 0o755); err != nil {
		t.Fatalf("make directory fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyPath, 0o600) })

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "directory", path: directoryPath},
		{name: "non markdown", path: textPath},
		{name: "read only", path: readOnlyPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := writableMarkdownFile(tc.path); err == nil {
				t.Fatal("writableMarkdownFile returned nil error")
			}
		})
	}
}
