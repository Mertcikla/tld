package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd/version"

	"github.com/mertcikla/tld/cmd"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestRootCmd_HelpMatchesReferenceSurface(t *testing.T) {
	stdout, _, err := cmd.RunCmd(t, ".", "--help")
	if err != nil {
		t.Fatalf("root --help: %v", err)
	}
	if !strings.Contains(stdout, "tld manages software architecture diagrams as code") {
		t.Fatalf("help output missing CLI description:\n%s", stdout)
	}
}

func TestRootCmd_VersionFlagMatchesCurrentVersion(t *testing.T) {
	stdout, _, err := cmd.RunCmd(t, ".", "--version")
	if err != nil {
		t.Fatalf("root --version: %v", err)
	}
	if strings.TrimSpace(stdout) != "tld version "+version.Version {
		t.Fatalf("unexpected version output %q", stdout)
	}
}

func TestAddCmd_HelpIncludesRefFlag(t *testing.T) {
	stdout, _, err := cmd.RunCmd(t, ".", "add", "--help")
	if err != nil {
		t.Fatalf("add --help: %v", err)
	}
	if !strings.Contains(stdout, "Add or update an element") {
		t.Fatalf("help output missing add summary:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--ref") {
		t.Fatalf("help output missing --ref:\n%s", stdout)
	}
}

func TestAddCmd_RefOverridesGeneratedSlug(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "add", "System Overview", "--ref", "system-root", "--kind", "workspace")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	elements, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read elements.yaml: %v", err)
	}
	content := string(elements)
	if !strings.Contains(content, "system-root:") {
		t.Fatalf("elements.yaml missing explicit ref:\n%s", content)
	}
	if !strings.Contains(content, "name: System Overview") {
		t.Fatalf("elements.yaml missing element name:\n%s", content)
	}
	if strings.Contains(content, "system-overview:") {
		t.Fatalf("elements.yaml should not include slugified fallback ref:\n%s", content)
	}
}

func TestConnectCmd_HelpHidesStyleFlag(t *testing.T) {
	stdout, _, err := cmd.RunCmd(t, ".", "connect", "--help")
	if err != nil {
		t.Fatalf("connect --help: %v", err)
	}
	if !strings.Contains(stdout, "--direction") {
		t.Fatalf("connect help missing --direction:\n%s", stdout)
	}
	if strings.Contains(stdout, "--style") {
		t.Fatalf("connect help should hide --style:\n%s", stdout)
	}
	if strings.Contains(stdout, "--view") {
		t.Fatalf("connect help should hide legacy --view:\n%s", stdout)
	}
}

func TestAnalyzeCmd_EmptyGoFileDoesNotChangeWorkspaceContents(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	repoDir := filepath.Join(dir, "repo")
	cmd.InitGitRepo(t, repoDir, "empty.go", "package main\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze empty.go: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Connectors) != 0 {
		t.Fatalf("empty Go file should not create connectors: %+v", ws.Connectors)
	}
	if len(ws.Elements) == 0 {
		t.Fatalf("watch-backed analyze should materialize repository/file context")
	}
}
