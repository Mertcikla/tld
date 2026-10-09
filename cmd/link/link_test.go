package link_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func writeElements(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write elements: %v", err)
	}
}

func readElements(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read elements: %v", err)
	}
	return string(data)
}

func TestLinkCmd_ExternalLinkAddsTagAndURL(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "svc", "--external", "https://status.acme.com")
	if err != nil {
		t.Fatalf("link: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, `Linked "svc"`) || !strings.Contains(stdout, "https://status.acme.com") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	content := readElements(t, dir)
	if !strings.Contains(content, "url: https://status.acme.com") {
		t.Fatalf("url not written:\n%s", content)
	}
	if !strings.Contains(content, "external") {
		t.Fatalf("external tag not written:\n%s", content)
	}
}

func TestLinkCmd_ExternalLinkExemptsFromGrounding(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	if _, _, err := cmd.RunCmd(t, dir, "link", "svc", "--external", "https://github.com/acme/other"); err != nil {
		t.Fatalf("link: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate", "ARC205")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(stdout, "Exempt (external links): 1") {
		t.Fatalf("external element not exempt:\n%s", stdout)
	}
}

func TestLinkCmd_KeepsCacheInSync(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "link", "sys", "--external", "https://status.acme.com"); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "validate"); err != nil {
		t.Fatalf("validate after link: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "link", "sys", "--unlink"); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "validate"); err != nil {
		t.Fatalf("validate after unlink: %v", err)
	}
}

func TestLinkCmd_UnlinkClearsSourceLink(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	if _, _, err := cmd.RunCmd(t, dir, "link", "svc", "internal/api.go#function:Handle"); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "link", "svc", "--unlink"); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	content := readElements(t, dir)
	if strings.Contains(content, "file_path:") {
		t.Fatalf("file_path not cleared:\n%s", content)
	}
}

func TestLinkCmd_ExplicitFileAndSymbol(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	if _, _, err := cmd.RunCmd(t, dir, "link", "svc", "--file", "internal/api.go", "--symbol", "Handle"); err != nil {
		t.Fatalf("link: %v", err)
	}
	content := readElements(t, dir)
	if !strings.Contains(content, "file_path: internal/api.go#symbol:Handle") {
		t.Fatalf("explicit symbol anchor not written:\n%s", content)
	}
}

func TestLinkCmd_RepoLink(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	if _, _, err := cmd.RunCmd(t, dir, "link", "svc", "--repo", "https://github.com/acme/app"); err != nil {
		t.Fatalf("link: %v", err)
	}
	content := readElements(t, dir)
	if !strings.Contains(content, "repo: acme/app") {
		t.Fatalf("repo not written:\n%s", content)
	}
}

func TestLinkCmd_RequiresTarget(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	_, _, err := cmd.RunCmd(t, dir, "link", "svc")
	if err == nil || !strings.Contains(err.Error(), "a target is required") {
		t.Fatalf("expected target-required error, got %v", err)
	}
}

func TestLinkCmd_NextOrdersByViewDepth(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, `platform:
  name: Platform
  kind: workspace
  has_view: true
  file_path: platform.go
  placements: [ { parent: root } ]
nested:
  name: Nested
  kind: struct
  placements: [ { parent: platform } ]
top:
  name: Top
  kind: struct
  placements: [ { parent: root } ]
`)

	stdout, _, err := cmd.RunCmd(t, dir, "link", "--next")
	if err != nil {
		t.Fatalf("link --next: %v", err)
	}
	if !strings.Contains(stdout, "ref top") {
		t.Fatalf("expected top-level element first, got:\n%s", stdout)
	}
}

func TestLinkCmd_NextWhenAllGrounded(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n  file_path: internal/api.go\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "--next")
	if err != nil {
		t.Fatalf("link --next: %v", err)
	}
	if !strings.Contains(stdout, "All linkable elements are grounded.") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestLinkCmd_IgnoreExemptsFromGrounding(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "svc", "--ignore")
	if err != nil {
		t.Fatalf("link --ignore: %v\n%s", err, stdout)
	}
	if !strings.Contains(readElements(t, dir), "$ignored") {
		t.Fatalf("ignore marker not written:\n%s", readElements(t, dir))
	}

	out, _, err := cmd.RunCmd(t, dir, "validate", "ARC205")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(out, "Exempt (ignored): 1") {
		t.Fatalf("ignored element not exempt:\n%s", out)
	}
}

func TestLinkCmd_UnignoreRestoresGrounding(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n  tags: [\"ui\", \"$ignored\"]\n")

	if _, _, err := cmd.RunCmd(t, dir, "link", "svc", "--unignore"); err != nil {
		t.Fatalf("unignore: %v", err)
	}
	content := readElements(t, dir)
	if strings.Contains(content, "$ignored") {
		t.Fatalf("ignore marker not cleared:\n%s", content)
	}
	if !strings.Contains(content, "ui") {
		t.Fatalf("other tags not preserved:\n%s", content)
	}
}

func TestLinkCmd_NextSkipsIgnored(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n  tags: [\"$ignored\"]\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "--next")
	if err != nil {
		t.Fatalf("link --next: %v", err)
	}
	if !strings.Contains(stdout, "All linkable elements are grounded.") {
		t.Fatalf("ignored element should not be suggested:\n%s", stdout)
	}
}

func TestLinkCmd_IgnoreConflictsWithLinkTarget(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	_, _, err := cmd.RunCmd(t, dir, "link", "svc", "--ignore", "--external", "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestLinkCmd_MarksLocalMetadataChanged(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, _, err := cmd.RunCmd(t, dir, "link", "sys", "internal/api.go#function:Handle"); err != nil {
		t.Fatalf("link: %v", err)
	}

	lockFile, err := workspace.LoadLockFile(workspace.ResolveDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if lockFile == nil {
		t.Fatal("no lock file after link")
	}
	current := lockFile.CurrentElements["sys"]
	lastSync := lockFile.Metadata.Elements["sys"]
	if current == nil || lastSync == nil {
		t.Fatalf("missing metadata: current=%+v lastSync=%+v", current, lastSync)
	}
	if !current.UpdatedAt.After(lastSync.UpdatedAt) {
		t.Fatalf("link did not mark a local change: current=%s lastSync=%s", current.UpdatedAt, lastSync.UpdatedAt)
	}
	workspaceDir := workspace.ResolveDir(dir)
	if !strings.Contains(readElements(t, workspaceDir), "file_path: internal/api.go#function:Handle") {
		t.Fatalf("link not written to YAML:\n%s", readElements(t, workspaceDir))
	}
}

// TestLinkCmd_NextSkipsCodeindexElements guards against link suggestions
// diverging from `tld validate ARC205`: codeindex-materialized elements are
// excluded from the score and therefore must not be suggested for linking.
func TestLinkCmd_NextSkipsCodeindexElements(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, `mapped:
  name: frontend/src
  kind: component
manual:
  name: Manual
  kind: component
`)

	dataDir := t.TempDir()
	ctx := context.Background()
	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{
		SQLitePath: filepath.Join(dataDir, "tld.db"),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO elements (id, name, kind, created_at, updated_at)
		VALUES (101, 'frontend/src', 'component', 'now', 'now')`); err != nil {
		t.Fatalf("insert element: %v", err)
	}
	if err := cstore.NewStoreFromHandle(handle).SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|group|repo|101", Kind: cstore.MappingElement, ResourceID: 101, RepositoryID: "repo", SnapshotID: "snap"},
	}); err != nil {
		t.Fatalf("save mappings: %v", err)
	}
	_ = handle.Close()
	t.Setenv("TLD_DATA_DIR", dataDir)

	stdout, _, err := cmd.RunCmd(t, dir, "link", "--next")
	if err != nil {
		t.Fatalf("link --next: %v\n%s", err, stdout)
	}
	if strings.Contains(stdout, "ref mapped") {
		t.Fatalf("codeindex element should not be suggested:\n%s", stdout)
	}
	if !strings.Contains(stdout, "ref manual") {
		t.Fatalf("user-authored element should be suggested:\n%s", stdout)
	}
}
