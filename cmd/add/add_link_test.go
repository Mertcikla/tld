package add_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func mustLinkElement(t *testing.T, dir, filePath string) *workspace.Element {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	el := ws.Elements["svc"]
	if el == nil {
		t.Fatal("svc element missing")
	}
	if el.FilePath != filePath {
		t.Fatalf("file_path = %q, want %q", el.FilePath, filePath)
	}
	return el
}

func TestAddCmd_LinkFilePath(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", "internal/api.go")
	mustLinkElement(t, dir, "internal/api.go")
}

func TestAddCmd_LinkSymbolAndLine(t *testing.T) {
	for _, tt := range []struct {
		name string
		link string
		want string
	}{
		{name: "symbol", link: "internal/api.go#function:Handle", want: "internal/api.go#function:Handle"},
		{name: "line", link: "internal/api.go#L42", want: "internal/api.go#L42"},
		{name: "folder", link: "internal/api/", want: "internal/api/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)

			cmd.MustRunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", tt.link)
			mustLinkElement(t, dir, tt.want)
		})
	}
}

func TestAddCmd_LinkURLRejected(t *testing.T) {
	for _, link := range []string{"https://status.acme.com", "http://status.acme.com/x"} {
		dir := t.TempDir()
		cmd.MustInitWorkspace(t, dir)

		_, _, err := cmd.RunCmd(t, dir, "add", "Status", "--ref", "svc", "--kind", "service", "--link", link)
		if err == nil {
			t.Fatalf("expected error for --link %q: URLs belong in --url or 'tld link --external'", link)
		}
	}
}

func TestAddCmd_URLFlagIndependentOfLink(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	// --url (element homepage) and --link (source file) are separate fields
	// and must coexist.
	cmd.MustRunCmd(t, dir, "add", "Status", "--ref", "svc", "--kind", "service",
		"--url", "https://acme.com", "--link", "internal/api.go")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	el := ws.Elements["svc"]
	if el == nil {
		t.Fatal("svc element missing")
	}
	if el.URL != "https://acme.com" {
		t.Fatalf("url = %q, want element URL", el.URL)
	}
	if el.FilePath != "internal/api.go" {
		t.Fatalf("file_path = %q, want source link", el.FilePath)
	}
}

func TestAddCmd_LinkBareAnchorFails(t *testing.T) {
	for _, link := range []string{"#function:Handle", "#L42"} {
		dir := t.TempDir()
		cmd.MustInitWorkspace(t, dir)

		if _, _, err := cmd.RunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", link); err == nil {
			t.Fatalf("expected error for --link %q", link)
		}
	}
}

func TestAddCmd_RelinkOverwritesFilePath(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", "a.go")
	cmd.MustRunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", "b.go")
	mustLinkElement(t, dir, "b.go")
}

func TestAddCmd_NoLinkPreservesFilePath(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", "a.go")
	cmd.MustRunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--description", "updated")
	mustLinkElement(t, dir, "a.go")
}

func TestAddCmd_LinkDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	elementsPath := filepath.Join(dir, ".tld", "elements.yaml")
	before, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatalf("read elements before: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "add", "Service", "--ref", "svc", "--kind", "service", "--link", "internal/api.go", "--dry-run")
	if err != nil {
		t.Fatalf("add --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "link=internal/api.go") {
		t.Fatalf("expected link in dry-run output, got:\n%s", stdout)
	}

	after, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatalf("read elements after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("elements.yaml changed during dry-run\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
