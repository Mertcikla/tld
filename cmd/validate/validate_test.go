package validate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
)

func TestValidateCmd_ValidWorkspace(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(stdout, "Workspace valid") {
		t.Errorf("stdout %q does not contain 'Workspace valid'", stdout)
	}
	if !strings.Contains(stdout, "1 elements") || !strings.Contains(stdout, "0 views") {
		t.Errorf("stdout %q does not contain count summary", stdout)
	}
}

func TestValidateCmd_InvalidWorkspace(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte("bad:\n  kind: service\n"), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}

	_, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err == nil {
		t.Fatal("expected error for invalid workspace")
	}
	if !strings.Contains(stderr, "Validation errors") {
		t.Errorf("stderr %q does not contain 'Validation errors'", stderr)
	}
}

func TestValidateCmd_DuplicateNamesAreWarnings(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`
api:
  name: API
  kind: service
api-dup:
  name: API
  kind: service
`), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate should treat duplicate names as warnings: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Workspace valid") || !strings.Contains(stdout, "Validation warnings") || !strings.Contains(stdout, "duplicate element name") {
		t.Fatalf("stdout should include valid summary and duplicate warning, got:\n%s", stdout)
	}
}

func TestValidateCmd_RuleCodeWithViolations(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace", "--technology", "Go"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "add", "Service", "--ref", "svc", "--parent", "sys", "--kind", "service"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC102")
	if err != nil {
		t.Fatalf("validate ARC102: %v", err)
	}
	if !strings.Contains(stdout, "[ARC102]") {
		t.Errorf("stdout %q does not contain [ARC102]", stdout)
	}
	if !strings.Contains(stdout, "Missing Tech") {
		t.Errorf("stdout %q does not contain 'Missing Tech'", stdout)
	}
	if !strings.Contains(stdout, "\"svc\"") {
		t.Errorf("stdout %q does not contain violating element 'svc'", stdout)
	}
	if !strings.Contains(stdout, "How to fix:") {
		t.Errorf("stdout %q does not contain 'How to fix:'", stdout)
	}
}

func TestValidateCmd_RuleCodeNoViolations(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace", "--technology", "Go"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC103")
	if err != nil {
		t.Fatalf("validate ARC103: %v", err)
	}
	if !strings.Contains(stdout, "No violations found for ARC103") {
		t.Errorf("stdout %q does not contain 'No violations found'", stdout)
	}
}

func TestValidateCmd_UnknownRuleCode(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "validate", "INVALID")
	if err == nil {
		t.Fatal("expected error for unknown rule code")
	}
	if !strings.Contains(err.Error(), "unknown rule code") {
		t.Errorf("error %q does not contain 'unknown rule code'", err)
	}
}

func TestValidateCmd_VerboseFlag(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "-v")
	if err != nil {
		t.Fatalf("validate -v: %v", err)
	}
	if !strings.Contains(stdout, "Workspace valid") {
		t.Errorf("stdout %q does not contain 'Workspace valid'", stdout)
	}
}

func TestValidateCmd_ShowsSuppressionGuidance(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(stdout, "validation.exclude_rules") {
		t.Fatalf("expected suppression guidance in output, got:\n%s", stdout)
	}
}

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

// TestValidateCmd_AllChecksPass verifies validation, symbol verification, and
// diagram freshness all pass for a synced workspace.
func TestValidateCmd_AllChecksPass(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Service\n  placements: [ { parent: root } ]\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Workspace valid") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if strings.Contains(stdout, "Outdated diagrams") || strings.Contains(stderr, "Symbol verification errors") {
		t.Fatalf("expected a clean validate, got:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

// TestValidateCmd_BrokenSymbol verifies symbol verification failures abort.
func TestValidateCmd_BrokenSymbol(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Missing\n  placements: [ { parent: root } ]\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err == nil {
		t.Fatalf("expected symbol verification failure\nstderr: %s", stderr)
	}
	if !strings.Contains(stderr, "Validation errors") || !strings.Contains(stderr, `symbol "Missing" not found`) {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

// TestValidateCmd_OutdatedWarn verifies stale diagram metadata is reported as a
// warning without failing by default.
func TestValidateCmd_OutdatedWarn(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Service\n  placements: [ { parent: root } ]\n\n_meta_elements:\n  service:\n    id: 1\n    updated_at: 2000-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("expected warning-only validate\nstdout: %s\nstderr: %s\nerr: %v", stdout, stderr, err)
	}
	if !strings.Contains(stdout, "Outdated diagrams") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

// TestValidateCmd_OutdatedStrict verifies --strict turns outdated diagrams into
// a non-zero exit.
func TestValidateCmd_OutdatedStrict(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.InitGitRepo(t, dir, "service.go", "package main\nfunc Service() {}\n")
	withWorkingDir(t, dir)
	content := "service:\n  name: Service\n  kind: service\n  file_path: service.go\n  symbol: Service\n  placements: [ { parent: root } ]\n\n_meta_elements:\n  service:\n    id: 1\n    updated_at: 2000-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "validate", "--strict")
	if err == nil {
		t.Fatalf("expected strict validate failure\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Outdated diagrams") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestValidateCmd_SkipsForeignRepoSymbols(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	content := `good:
  name: Good Service
  kind: service
  file_path: cmd/init.go
  symbol: newInitCmd
  placements: [ { parent: root } ]
foreign:
  name: Foreign Service
  kind: service
  file_path: /tmp/foreign/foreign.go
  symbol: doesNotExist
  repo: https://example.com/other.git
  placements: [ { parent: root } ]
`
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatalf("write elements.yaml: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Workspace valid") {
		t.Errorf("stdout %q does not contain validation success", stdout)
	}
	if strings.Contains(stderr, "Foreign Service") || strings.Contains(stderr, "doesNotExist") {
		t.Errorf("stderr %q should not mention the foreign repo symbol", stderr)
	}
}
