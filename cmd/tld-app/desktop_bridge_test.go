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
