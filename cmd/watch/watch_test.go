package watch

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanCommandFailsClearlyOutsideGitRepository(t *testing.T) {
	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scan", t.TempDir(), "--data-dir", t.TempDir()})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("expected outside-git error, got %v", err)
	}
}

func TestScanCommandPrintsCountsAndSkipsRepeatScan(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func main() {
	helper()
}

func helper() {}
`)
	dataDir := t.TempDir()

	first := runScanCommand(t, repo, dataDir)
	if !strings.Contains(first, "Files:      1 seen, 1 parsed, 0 skipped") ||
		!strings.Contains(first, "Symbols:    2") ||
		!strings.Contains(first, "References: 1") {
		t.Fatalf("unexpected first scan output:\n%s", first)
	}

	second := runScanCommand(t, repo, dataDir)
	if !strings.Contains(second, "Files:      1 seen, 0 parsed, 1 skipped") {
		t.Fatalf("unexpected repeat scan output:\n%s", second)
	}
}

func TestRepresentCommandPrintsMaterializationCounts(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)
	dataDir := t.TempDir()

	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"represent", repo, "--data-dir", dataDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("represent command: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, expected := range []string{"Filter run:", "Represent run:", "Elements:", "Connectors:", "Representation:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("represent output missing %q:\n%s", expected, text)
		}
	}
}

func runScanCommand(t *testing.T, repo, dataDir string) string {
	t.Helper()
	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scan", repo, "--data-dir", dataDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scan command: %v\n%s", err, out.String())
	}
	return out.String()
}

func initGitRepoNoCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
