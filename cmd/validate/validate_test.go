package validate_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func TestValidateCmd_AbortsWhenCacheOutOfSync(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Hand-edit the YAML cache without pushing it to the target.
	if err := workspace.UpdateElementField(workspace.ResolveDir(dir), "sys", "name", "Renamed System"); err != nil {
		t.Fatalf("edit cache: %v", err)
	}

	_, _, err := cmd.RunCmd(t, dir, "validate")
	if err == nil {
		t.Fatal("expected out-of-sync error")
	}
	if !strings.Contains(err.Error(), "out of sync") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "tld pull") || !strings.Contains(err.Error(), "tld sync") {
		t.Fatalf("error should suggest pull/sync: %v", err)
	}
}

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

func TestValidateCmd_DuplicateNamesAreARC204(t *testing.T) {
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

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "--strictness", "3")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(stdout, "Workspace valid") {
		t.Errorf("stdout should include valid summary, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "ARC204") || !strings.Contains(stdout, "Duplicate Name") {
		t.Errorf("stdout should include ARC204 duplicate-name warning, got:\n%s", stdout)
	}

	// Requesting a rule explicitly runs it regardless of the configured or
	// overridden strictness level.
	for _, args := range [][]string{
		{"validate", "ARC204"},
		{"validate", "ARC204", "--strictness", "1"},
	} {
		stdout, _, err := cmd.RunCmd(t, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(stdout, "[ARC204]") || !strings.Contains(stdout, `"api-dup"`) {
			t.Errorf("%v stdout should include ARC204 violations, got:\n%s", args, stdout)
		}
	}
}

func TestValidateCmd_RuleRequestOverridesExclude(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cfgDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", cfgDir)
	if err := os.WriteFile(filepath.Join(cfgDir, "tld.global.yaml"), []byte("validation:\n  level: 3\n  exclude_rules: [ARC204]\n"), 0600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
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

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC204")
	if err != nil {
		t.Fatalf("validate ARC204: %v", err)
	}
	if !strings.Contains(stdout, "[ARC204]") || !strings.Contains(stdout, `"api-dup"`) {
		t.Errorf("stdout should include ARC204 despite exclude list, got:\n%s", stdout)
	}
}

func TestValidateCmd_GroundingReport(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`
handler:
  name: Handler
  kind: function
model:
  name: Model
  kind: struct
`), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC205")
	if err != nil {
		t.Fatalf("validate ARC205: %v", err)
	}
	for _, want := range []string{
		"[ARC205]", "Low Grounding", "Workspace source grounding: 0/10",
		"Linkable elements: 2", "0 of 2 linkable elements", "Reasoning:", "How to improve:",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q does not contain %q", stdout, want)
		}
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
	if strings.Contains(stdout, "Workspace valid") || strings.Contains(stdout, "Architectural Warnings") {
		t.Errorf("stdout %q should only contain the requested rule output", stdout)
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

func TestValidateCmd_RulesListsByLevel(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "rules")
	if err != nil {
		t.Fatalf("validate rules: %v", err)
	}
	for _, want := range []string{
		"Level 1 (Minimal)", "Level 2 (Standard)", "Level 3 (Strict)",
		"ARC001", "High Density",
		"ARC102", "Missing Tech",
		"ARC203", "Missing Label",
		"ARC206", "Low Link Coverage",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q does not contain %q", stdout, want)
		}
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

func TestValidateCmd_LinkCoverageReport(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`
api:
  name: API
  kind: service
  file_path: A/B/C.go
  placements: [ { parent: root } ]
`), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}

	dataDir := t.TempDir()
	seedLinkCoverageIndex(t, dataDir, dir, []string{"A/B/C.go", "A/D/E.go"})

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC206", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("validate ARC206: %v", err)
	}
	for _, want := range []string{
		"[ARC206]", "Low Link Coverage",
		"Workspace link coverage: 50% (1/2 directories, threshold 75%)",
		"unlinked: A/D", "Reasoning:", "How to improve:",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q does not contain %q", stdout, want)
		}
	}

	// A workspace that pins a lower threshold in .tld.yaml passes with the same links.
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), []byte("validation:\n  link_coverage_percent: 50\n"), 0600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	stdout, _, err = cmd.RunCmd(t, dir, "validate", "ARC206", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("validate ARC206 with lowered threshold: %v", err)
	}
	if !strings.Contains(stdout, "threshold 50%") || strings.Contains(stdout, "unlinked: A/D") {
		t.Errorf("stdout should show no unlinked directories at a 50%% threshold, got:\n%s", stdout)
	}
}

// seedLinkCoverageIndex publishes a codeindex snapshot for root so the ARC206
// link coverage check has an indexed repository to measure against.
func seedLinkCoverageIndex(t *testing.T, dataDir, root string, paths []string) {
	t.Helper()
	ctx := context.Background()
	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{
		SQLitePath: filepath.Join(dataDir, "tld.db"),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = handle.Close() }()

	snapshot := &pb.Snapshot{Id: "snapshot-1", RepositoryId: "repo-1", IngestionStatus: "complete"}
	for _, path := range paths {
		snapshot.Sources = append(snapshot.Sources, &pb.SourceFile{Path: path, Hash: "hash-" + path, Size: 1})
	}
	g := graph.NewGraph("repo-1", snapshot.Id)
	for _, path := range paths {
		g.Sources[path] = &graph.Source{Path: path}
	}
	idx := cstore.NewStoreFromHandle(handle)
	if err := idx.Publish(ctx, root, snapshot, g); err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
}

func TestValidateCmd_LinkCoverageWithoutIndex(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(`
api:
  name: API
  kind: service
  file_path: A/B/C.go
  placements: [ { parent: root } ]
`), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC206", "--data-dir", t.TempDir())
	if err != nil {
		t.Fatalf("validate ARC206: %v", err)
	}
	if !strings.Contains(stdout, "link coverage does not apply") {
		t.Errorf("stdout %q should explain link coverage does not apply", stdout)
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
