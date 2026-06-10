package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
)

func TestRenderCmd_MermaidToStdout(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")

	stdout, _, err := cmd.RunCmd(t, dir, "render", "platform")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(stdout, "flowchart LR") {
		t.Fatalf("expected mermaid header, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "n_api") || !strings.Contains(stdout, "n_db") {
		t.Fatalf("expected node ids in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "reads") {
		t.Fatalf("expected connector label in output, got:\n%s", stdout)
	}
}

func TestRenderCmd_MermaidToFile(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")

	outPath := filepath.Join(dir, "diagram.mmd")
	_, _, err := cmd.RunCmd(t, dir, "render", "root", "-o", outPath)
	if err != nil {
		t.Fatalf("render -o: %v", err)
	}
	content, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("read output file: %v", readErr)
	}
	if !strings.Contains(string(content), "flowchart LR") {
		t.Fatalf("expected mermaid content, got:\n%s", string(content))
	}
}

func TestRenderCmd_MermaidEscapesQuotesAndFlattensNewlines(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API \"Gateway\"\nLine", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads \"from\"\nprimary")

	stdout, _, err := cmd.RunCmd(t, dir, "render", "root")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(stdout, `API &quot;Gateway&quot; Line`) {
		t.Fatalf("expected escaped flattened element label, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `reads &quot;from&quot; primary`) {
		t.Fatalf("expected escaped flattened connector label, got:\n%s", stdout)
	}
	if strings.Contains(stdout, `\"`) {
		t.Fatalf("expected quote entities instead of backslash-escaped quotes, got:\n%s", stdout)
	}
}

func TestRenderCmd_MissingViewFails(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "render", "missing-view")
	if err == nil {
		t.Fatal("expected missing view error")
	}
	if !strings.Contains(err.Error(), "view \"missing-view\" not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
