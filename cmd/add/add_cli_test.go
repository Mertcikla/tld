package add_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestAddCmd_DryRunDoesNotMutateWorkspace(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	elementsPath := filepath.Join(dir, ".tld", "elements.yaml")
	before, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatalf("read elements before: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--dry-run")
	if err != nil {
		t.Fatalf("add --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run: add: api") {
		t.Fatalf("expected dry-run output, got:\n%s", stdout)
	}

	after, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatalf("read elements after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("elements.yaml changed during dry-run\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestAddCmd_CustomKindSucceeds(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "componet"); err != nil {
		t.Fatalf("custom kind should be accepted: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if got := ws.Elements["api"].Kind; got != "componet" {
		t.Fatalf("kind = %q, want custom value", got)
	}
}

func TestAddCmd_TrimsKind(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "  custom kind  "); err != nil {
		t.Fatalf("add with spaced kind: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if got := ws.Elements["api"].Kind; got != "custom kind" {
		t.Fatalf("kind = %q, want trimmed value", got)
	}
}

func TestAddCmd_InvalidKindFails(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{name: "empty", kind: "   "},
		{name: "control", kind: "service\nkind"},
		{name: "too long", kind: strings.Repeat("a", 65)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)

			if _, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", tt.kind); err == nil {
				t.Fatal("expected invalid kind error")
			}
		})
	}
}

func TestAddCmd_ShowsNormalizedTechnology(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "add", "catch2", "--ref", "catch2", "--kind", "system", "--technology", "C++14")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(stdout, "technology normalized") || !strings.Contains(stdout, "C++14") {
		t.Fatalf("expected normalization output, got:\n%s", stdout)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	e := ws.Elements["catch2"]
	if e == nil {
		t.Fatal("missing catch2 element")
	}
	if strings.EqualFold(e.Technology, "C++14") {
		t.Fatalf("expected normalized technology to differ from input, got %q", e.Technology)
	}
	if !strings.Contains(stdout, e.Technology) {
		t.Fatalf("expected output to include normalized value %q, got:\n%s", e.Technology, stdout)
	}
}
