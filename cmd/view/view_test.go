package view_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestViewCmdCreateRenameSetLevelDelete(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "view", "create", "api", "--name", "API Diagram", "--label", "Container")
	el := loadElement(t, dir, "api")
	if !el.HasView {
		t.Fatalf("api has_view = false after view create")
	}
	if el.ViewName != "API Diagram" || el.ViewLabel != "Container" {
		t.Fatalf("view fields = (%q, %q), want (API Diagram, Container)", el.ViewName, el.ViewLabel)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Meta == nil || ws.Meta.Views["api"] == nil {
		t.Fatalf("view metadata for api missing: %+v", ws.Meta)
	}

	cmd.MustRunCmd(t, dir, "view", "rename", "api", "API v2")
	if got := loadElement(t, dir, "api").ViewName; got != "API v2" {
		t.Fatalf("view_name = %q, want API v2", got)
	}

	cmd.MustRunCmd(t, dir, "view", "set-level", "api", "Component")
	if got := loadElement(t, dir, "api").ViewLabel; got != "Component" {
		t.Fatalf("view_label = %q, want Component", got)
	}

	cmd.MustRunCmd(t, dir, "view", "delete", "api")
	el = loadElement(t, dir, "api")
	if el.HasView {
		t.Fatalf("api has_view = true after view delete")
	}
	if el.ViewName != "" || el.ViewLabel != "" {
		t.Fatalf("view fields not cleared: (%q, %q)", el.ViewName, el.ViewLabel)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Meta != nil && ws.Meta.Views["api"] != nil {
		t.Fatalf("view metadata for api still present after delete")
	}
}

func TestViewCmdCreateDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "view", "create", "api", "--dry-run")
	if err != nil {
		t.Fatalf("view create --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Fatalf("stdout = %q, want dry-run confirmation", stdout)
	}
	if loadElement(t, dir, "api").HasView {
		t.Fatalf("dry-run created a view")
	}
}

func TestViewCmdCreateRequiresElement(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "view", "create", "ghost")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want element-not-found error", err)
	}
}

func TestViewCmdDeleteRefusesNonEmptyView(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "view", "delete", "platform")
	if err == nil || !strings.Contains(err.Error(), "still has") {
		t.Fatalf("err = %v, want non-empty view refusal", err)
	}
}

func loadElement(t *testing.T, dir, ref string) *workspace.Element {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	el := ws.Elements[ref]
	if el == nil {
		t.Fatalf("element %q missing", ref)
	}
	return el
}
