package workspacesource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestViewMarkdown(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	markdown := "```mermaid\n" + strings.TrimSpace(content) + "\n```\n"
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
}

func writeLegacyMMD(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write legacy mmd: %v", err)
	}
}

func TestParseTreeNestedViewsAndDuplicateElementAcrossViews(t *testing.T) {
	root := t.TempDir()
	writeTestViewMarkdown(t, root, "platform/view.md", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform owner=platform
  api["API"]
%% tld-element ref=api kind=service x=120 y=80 file=internal/api.go tags=backend
  db["DB"]
%% tld-element ref=db kind=database x=320 y=80
  api -- "reads" --> db
%% tld-connector ref=9 source=api target=db label=reads rel=query
`)
	writeTestViewMarkdown(t, root, "platform/api/view.md", `flowchart LR
%% tld/v1 view ref=api parent=platform name=API owner=api
  api["API"]
%% tld-element ref=api kind=service x=0 y=0 file=internal/api.go tags=backend
`)

	parsed, err := ParseTree(root)
	if err != nil {
		t.Fatalf("ParseTree() error = %v", err)
	}

	if len(parsed.Views) != 2 {
		t.Fatalf("views = %d, want 2", len(parsed.Views))
	}
	if parsed.Views["api"].ParentRef != "platform" {
		t.Fatalf("child parent = %q, want platform", parsed.Views["api"].ParentRef)
	}
	if len(parsed.Elements) != 2 {
		t.Fatalf("elements = %d, want 2", len(parsed.Elements))
	}
	if got := len(parsed.Placements); got != 3 {
		t.Fatalf("placements = %d, want 3", got)
	}
	if parsed.Elements["api"].FilePath != "internal/api.go" {
		t.Fatalf("api file = %q", parsed.Elements["api"].FilePath)
	}
	if len(parsed.Elements["api"].Tags) != 1 || parsed.Elements["api"].Tags[0] != "backend" {
		t.Fatalf("api tags = %#v", parsed.Elements["api"].Tags)
	}
	if connector := parsed.Connectors["9"]; connector == nil || connector.SourceRef != "api" || connector.TargetRef != "db" {
		t.Fatalf("connector not parsed correctly: %#v", connector)
	}
}

func TestParseTreeRejectsDuplicateRefs(t *testing.T) {
	root := t.TempDir()
	writeTestViewMarkdown(t, root, "one/view.md", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform
`)
	writeTestViewMarkdown(t, root, "two/view.md", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Other
`)

	_, err := ParseTree(root)
	if err == nil || !strings.Contains(err.Error(), `duplicate view ref "platform"`) {
		t.Fatalf("ParseTree() error = %v, want duplicate view ref", err)
	}
}

func TestParseTreeRejectsMarkdownWithProse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "platform", ViewFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Platform\n\n```mermaid\nflowchart LR\n%% tld/v1 view ref=platform parent=root name=Platform\n```\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	_, err := ParseTree(root)
	if err == nil || !strings.Contains(err.Error(), "must contain only one Mermaid block") {
		t.Fatalf("ParseTree() error = %v, want prose rejection", err)
	}
}

func TestParseTreeRequiresExplicitElementRefs(t *testing.T) {
	root := t.TempDir()
	writeTestViewMarkdown(t, root, "platform/view.md", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform
  api["API"]
`)

	_, err := ParseTree(root)
	if err == nil || !strings.Contains(err.Error(), "missing tld-element metadata") {
		t.Fatalf("ParseTree() error = %v, want missing element metadata", err)
	}
}
