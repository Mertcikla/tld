package link_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
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

func TestLinkCmd_AnchorWritesFileLink(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "svc", "internal/api.go#function:Handle")
	if err != nil {
		t.Fatalf("link: %v\n%s", err, stdout)
	}
	content := readElements(t, dir)
	if !strings.Contains(content, "file_path: internal/api.go#function:Handle") {
		t.Fatalf("anchor not written:\n%s", content)
	}
	if !strings.Contains(stdout, "Grounding: workspace 10/10") {
		t.Fatalf("expected fully grounded: %s", stdout)
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
