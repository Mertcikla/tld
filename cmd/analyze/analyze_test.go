package analyze_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/cmd"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestAnalyzeCmd_WatchPipelineWritesYAML(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Foo() {}\nfunc Bar() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(ws, "repository") != 1 || countKind(ws, "file") != 1 || countKind(ws, "function") != 2 {
		t.Fatalf("unexpected analyzed elements: %+v", ws.Elements)
	}
	for ref, element := range ws.Elements {
		if element.Kind == "function" && len(element.Placements) == 0 {
			t.Fatalf("symbol %q (%s) has no placement", element.Name, ref)
		}
	}
}

func TestAnalyzeCmd_PreservesManualYAML(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Manual API", "--ref", "manual-api", "--kind", "service")
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "main.go", "package main\nfunc Main() {}\n")

	if stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none"); err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Elements["manual-api"] == nil {
		t.Fatalf("manual element was not preserved: %+v", ws.Elements)
	}
}

func TestAnalyzeCmd_DryRunDoesNotWriteYAML(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Service() {}\n")
	before, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--dry-run", "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze --dry-run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	after, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("elements.yaml changed during dry-run")
	}
}

func TestAnalyzeCmd_RemovedFlagsFail(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "analyze", dir, "--deep"); err == nil {
		t.Fatal("expected --deep to fail")
	}
	if _, _, err := cmd.RunCmd(t, dir, "analyze", dir, "--changed-since", "HEAD"); err == nil {
		t.Fatal("expected --changed-since to fail")
	}
}

func TestAnalyzeCmd_JSONDryRunUsesWatchDiffShape(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "main.go", "package main\nfunc Main() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "--format", "json", "analyze", repoDir, "--dry-run", "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze --format json --dry-run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var payload struct {
		Changed        bool             `json:"changed"`
		Scan           map[string]any   `json:"scan"`
		Representation map[string]any   `json:"representation"`
		Export         map[string]any   `json:"export"`
		Diffs          []map[string]any `json:"diffs"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if payload.Scan["repository_id"] == nil || payload.Representation["representation_hash"] == nil || payload.Export["elements_written"] == nil {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func countKind(ws *workspace.Workspace, kind string) int {
	count := 0
	for _, element := range ws.Elements {
		if element.Kind == kind {
			count++
		}
	}
	return count
}
