package analyze_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"
)

func TestAnalyzeCmd_RejectsRepoOutsideConfiguredRepositories(t *testing.T) {
	workspaceDir := t.TempDir()
	cmd.MustInitWorkspace(t, workspaceDir)

	workspaceCfg := strings.Join([]string{
		"project_name: Demo",
		"repositories:",
		"  frontend:",
		"    url: github.com/example/frontend",
		"    localDir: frontend",
		"exclude: []",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(workspaceDir, ".tld.yaml"), []byte(workspaceCfg), 0600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	cmd.InitGitRepo(t, filepath.Join(workspaceDir, "frontend"), "frontend.go", "package frontend\nfunc FrontendService() {}\n")

	repoDir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	git("add", "main.go")
	git("commit", "-m", "initial")

	stdout, stderr, err := cmd.RunCmd(t, workspaceDir, "analyze", filepath.Join(repoDir, "main.go"))
	if err == nil {
		t.Fatalf("expected analyze to fail for repo outside configured repositories\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "repo") || !strings.Contains(err.Error(), "repository") {
		t.Fatalf("unexpected error: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.Contains(stdout, "Analyzed:") {
		t.Fatalf("analyze should not write workspace changes\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestAnalyzeCmd_DiscoversConfiguredRepositories(t *testing.T) {
	workspaceDir := t.TempDir()
	cmd.MustInitWorkspace(t, workspaceDir)

	workspaceCfg := strings.Join([]string{
		"project_name: Demo",
		"repositories:",
		"  frontend:",
		"    url: github.com/example/frontend",
		"    localDir: frontend",
		"  backend:",
		"    url: github.com/example/backend",
		"    localDir: backend",
		"exclude: []",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(workspaceDir, ".tld.yaml"), []byte(workspaceCfg), 0600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	cmd.InitGitRepo(t, filepath.Join(workspaceDir, "frontend"), "frontend.go", "package frontend\nfunc FrontendService() {}\n")
	cmd.InitGitRepo(t, filepath.Join(workspaceDir, "backend"), "backend.go", "package backend\nfunc BackendService() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, workspaceDir, "analyze", workspaceDir, "--dry-run")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Analyzing "+workspaceDir+" (shallow)...") {
		t.Fatalf("stdout does not show analyze header\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  2 repositories scanned") {
		t.Fatalf("stdout does not summarize both repos\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run] No files written. Remove --dry-run to apply.") {
		t.Fatalf("stdout missing dry-run guidance\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestAnalyzeCmd_ChangedSinceLimitsScan(t *testing.T) {
	workspaceDir := t.TempDir()
	cmd.MustInitWorkspace(t, workspaceDir)

	workspaceCfg := strings.Join([]string{
		"project_name: Demo",
		"repositories:",
		"  frontend:",
		"    url: github.com/example/frontend",
		"    localDir: frontend",
		"exclude: []",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(workspaceDir, ".tld.yaml"), []byte(workspaceCfg), 0600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	repoDir := filepath.Join(workspaceDir, "frontend")
	cmd.InitGitRepo(t, repoDir, "frontend.go", "package frontend\nfunc FrontendService() {}\n")

	baseCmd := exec.Command("git", "rev-parse", "HEAD")
	baseCmd.Dir = repoDir
	base, err := baseCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "frontend.go"), []byte("package frontend\nfunc FrontendService() {}\nfunc NewFrontendService() {}\n"), 0600); err != nil {
		t.Fatalf("write frontend.go: %v", err)
	}
	commit := exec.Command("git", "commit", "-am", "update")
	commit.Dir = repoDir
	commit.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	stdout, stderr, err := cmd.RunCmd(t, workspaceDir, "analyze", workspaceDir, "--dry-run", "--changed-since", strings.TrimSpace(string(base)))
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  Incremental scan: 1 files changed since ") {
		t.Fatalf("stdout missing incremental summary\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  4 elements written to elements.yaml") {
		t.Fatalf("stdout missing changed-file element count\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  1 repositories scanned") {
		t.Fatalf("stdout missing repository count\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run] No files written. Remove --dry-run to apply.") {
		t.Fatalf("stdout missing dry-run guidance\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}
