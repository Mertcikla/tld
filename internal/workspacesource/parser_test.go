package workspacesource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestMMD(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write mmd: %v", err)
	}
}

func TestParseTreeNestedViewsAndDuplicateElementAcrossViews(t *testing.T) {
	root := t.TempDir()
	writeTestMMD(t, root, "platform/view.mmd", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform owner=platform
  api["API"]
%% tld-element ref=api kind=service x=120 y=80 file=internal/api.go tags=backend
  db["DB"]
%% tld-element ref=db kind=database x=320 y=80
  api -- "reads" --> db
%% tld-connector ref=9 source=api target=db label=reads rel=query
`)
	writeTestMMD(t, root, "platform/api/view.mmd", `flowchart LR
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
	writeTestMMD(t, root, "one/view.mmd", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform
`)
	writeTestMMD(t, root, "two/view.mmd", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Other
`)

	_, err := ParseTree(root)
	if err == nil || !strings.Contains(err.Error(), `duplicate view ref "platform"`) {
		t.Fatalf("ParseTree() error = %v, want duplicate view ref", err)
	}
}

func TestParseTreeRejectsMultipleMMDsInFolder(t *testing.T) {
	root := t.TempDir()
	writeTestMMD(t, root, "platform/view.mmd", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform
`)
	writeTestMMD(t, root, "platform/extra.mmd", `flowchart LR
%% tld/v1 view ref=extra parent=root name=Extra
`)

	_, err := ParseTree(root)
	if err == nil || !strings.Contains(err.Error(), "multiple .mmd files") {
		t.Fatalf("ParseTree() error = %v, want multiple .mmd files", err)
	}
}

func TestParseTreeRequiresExplicitElementRefs(t *testing.T) {
	root := t.TempDir()
	writeTestMMD(t, root, "platform/view.mmd", `flowchart LR
%% tld/v1 view ref=platform parent=root name=Platform
  api["API"]
`)

	_, err := ParseTree(root)
	if err == nil || !strings.Contains(err.Error(), "missing tld-element metadata") {
		t.Fatalf("ParseTree() error = %v, want missing element metadata", err)
	}
}
