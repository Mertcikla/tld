package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/tld/cmd"
)

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
}

func TestCheckCmd_AllPass(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Service\n  placements: [ { parent: root } ]\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "check")
	if err != nil {
		t.Fatalf("check: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Validation") || !strings.Contains(stdout, "Symbol Verification") || !strings.Contains(stdout, "Outdated Diagrams") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestCheckCmd_BrokenSymbol(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Missing\n  placements: [ { parent: root } ]\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "check")
	if err == nil {
		t.Fatalf("expected check failure\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stderr, "Symbol Verification") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestCheckCmd_OutdatedStrict(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Service\n  placements: [ { parent: root } ]\n\n_meta_elements:\n  service:\n    id: 1\n    updated_at: 2000-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "check", "--strict")
	if err == nil {
		t.Fatalf("expected strict check failure\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stderr, "Outdated Diagrams") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestCheckCmd_ValidationFail(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  placements:\n    - parent: missing\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "check")
	if err == nil {
		t.Fatalf("expected validation failure\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stderr, "Validation") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestCheckCmd_OutdatedWarn(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Service\n  placements: [ { parent: root } ]\n\n_meta_elements:\n  service:\n    id: 1\n    updated_at: 2000-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "check")
	if err != nil {
		t.Fatalf("expected warning-only check\nstdout: %s\nstderr: %s\nerr: %v", stdout, stderr, err)
	}
	if !strings.Contains(stderr, "Outdated Diagrams") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	_ = time.Now()
}
