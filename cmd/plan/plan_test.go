package plan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"

	"github.com/mertcikla/tld/internal/planner"
)

func TestPlanCmd_OutputsMarkdown(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "plan")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(stdout, "# Element Plan") {
		t.Errorf("stdout %q does not contain '# Element Plan'", stdout)
	}
}

func TestPlanCmd_VerboseFlag(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	// Without verbose
	stdout, _, err := cmd.RunCmd(t, dir, "plan")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if strings.Contains(stdout, "## Elements") {
		t.Errorf("stdout contains verbose section when it shouldn't: %q", stdout)
	}
	if !strings.Contains(stdout, "Use '-v' or '--verbose' for detailed element placement and connector reporting") {
		t.Errorf("stdout missing verbose hint: %q", stdout)
	}

	// With verbose
	stdout, _, err = cmd.RunCmd(t, dir, "plan", "-v")
	if err != nil {
		t.Fatalf("plan -v: %v", err)
	}
	if !strings.Contains(stdout, "## Actions") {
		t.Errorf("stdout missing verbose section when -v is used: %q", stdout)
	}
	if strings.Contains(stdout, "Use '-v' or '--verbose' for detailed resource reporting") {
		t.Errorf("stdout contains verbose hint when -v is used: %q", stdout)
	}
}

func TestPlanCmd_OutputToFile(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	outFile := filepath.Join(dir, "plan.md")
	stdout, _, err := cmd.RunCmd(t, dir, "plan", "--output", outFile)
	if err != nil {
		t.Fatalf("plan --output: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty when --output used, got: %q", stdout)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "# Element Plan") {
		t.Errorf("file content %q does not contain '# Element Plan'", string(data))
	}
}

func TestPlanCmd_JSONOutput(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "plan", "--format", "json")
	if err != nil {
		t.Fatalf("plan --format json: %v", err)
	}
	var payload planner.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Command != "plan" || payload.Status != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Summary["created"] == 0 {
		t.Fatalf("expected created resources in summary, got %+v", payload.Summary)
	}
}

func TestPlanCmd_InvalidWorkspaceErrors(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"),
		[]byte("child:\n  name: Child\n  kind: service\n  placements:\n    - parent: nonexistent\n"), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), []byte("org_id: test-org\nserver_url: http://localhost\napi_key: key\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, _, err := cmd.RunCmd(t, dir, "plan")
	if err == nil {
		t.Fatal("expected error for invalid workspace")
	}
}

func TestPlanCmd_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	_, _, err := cmd.RunCmd(t, dir, "plan")
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "load workspace") {
		t.Errorf("error %q does not contain 'load workspace'", err.Error())
	}
}
