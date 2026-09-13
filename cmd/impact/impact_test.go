package impact_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestImpactCmdReportsChangedAndUnmapped(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws.Elements["checkout"] = &workspace.Element{
		Name:     "Checkout Service",
		Kind:     "service",
		FilePath: "backend/checkout/**",
	}
	if err := workspace.Save(ws); err != nil {
		t.Fatal(err)
	}

	repoDir := filepath.Join(dir, "code")
	cmd.InitGitRepo(t, repoDir, "backend/checkout/service.go", "package checkout\n\nfunc Process() {}\n")
	writeAndCommit(t, repoDir, "internal/risk/model.go", "package risk\n")
	writeAndCommit(t, repoDir, "backend/checkout/service.go", "package checkout\n\nfunc Process() {}\nfunc Extra() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "impact", repoDir, "--base", "HEAD~2")
	if err != nil {
		t.Fatalf("impact: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Checkout Service") {
		t.Fatalf("expected changed element in output: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "internal/risk/model.go") {
		t.Fatalf("expected unmapped file in output:\n%s", stdout)
	}
}

func TestImpactCmdJSONOutput(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws.Elements["checkout"] = &workspace.Element{Name: "Checkout Service", Kind: "service", FilePath: "backend/checkout/**"}
	if err := workspace.Save(ws); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(dir, "code")
	cmd.InitGitRepo(t, repoDir, "backend/checkout/service.go", "package checkout\n")
	writeAndCommit(t, repoDir, "backend/checkout/service.go", "package checkout\n\nfunc Extra() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "--format", "json", "impact", repoDir, "--base", "HEAD~1")
	if err != nil {
		t.Fatalf("impact: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var report struct {
		Changed []struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
		} `json:"changed"`
		Base string `json:"base"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if report.Base != "HEAD~1" {
		t.Fatalf("base = %q, want HEAD~1", report.Base)
	}
	if len(report.Changed) != 1 || report.Changed[0].Ref != "checkout" {
		t.Fatalf("changed = %+v, want checkout", report.Changed)
	}
}

func writeAndCommit(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "update "+name)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
