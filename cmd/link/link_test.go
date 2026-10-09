package link_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/localserver"
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

func TestLinkCmd_AnchorWritesUnverifiedFileLink(t *testing.T) {
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

func TestLinkCmd_CandidatesNeedIndex(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Payment Service\n  kind: struct\n")

	_, _, err := cmd.RunCmd(t, dir, "link", "svc")
	if err == nil || !strings.Contains(err.Error(), "no local codeindex") {
		t.Fatalf("expected no-index error, got %v", err)
	}
}

func TestLinkCmd_CandidatesPrintRunnableCommands(t *testing.T) {
	dataDir := seedIndexedRepo(t)
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Process Payment\n  kind: struct\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "svc", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("link: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "tld link svc service.go#function:ProcessPayment") {
		t.Fatalf("expected a runnable candidate command, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "(0.") {
		t.Fatalf("candidate scores should not be printed:\n%s", stdout)
	}
}

func TestLinkCmd_ResolvesAgainstIndex(t *testing.T) {
	dataDir := seedIndexedRepo(t)
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	writeElements(t, dir, "svc:\n  name: Process Payment\n  kind: struct\n")

	stdout, _, err := cmd.RunCmd(t, dir, "link", "svc", "ProcessPayment", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("link: %v\n%s", err, stdout)
	}
	content := readElements(t, dir)
	if !strings.Contains(content, "file_path: service.go#function:ProcessPayment") {
		t.Fatalf("symbol anchor not written:\n%s", content)
	}
	if !strings.Contains(content, "repository_id:") {
		t.Fatalf("repository_id not written:\n%s", content)
	}
	if strings.Contains(stdout, "recorded unverified") {
		t.Fatalf("expected verified link, got:\n%s", stdout)
	}
}

// seedIndexedRepo builds and publishes a tiny codeindex snapshot in a data dir
// and returns that dir.
func seedIndexedRepo(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "service.go"), []byte("package main\n\nfunc ProcessPayment() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{
		SQLitePath: localserver.DatabasePath(dataDir),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = handle.Close() }()

	pipeline := indexer.Pipeline{Config: config.Default()}
	snap, graph, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: repoDir}, nil)
	if err != nil {
		t.Fatalf("index build: %v", err)
	}
	store := cstore.NewStoreFromHandle(handle)
	if err := store.Publish(ctx, repoDir, snap, graph); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return dataDir
}
