package render_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	mcheck "github.com/sammcj/mermaid-check"
)

func TestRenderCmd_PreservesConnectorDirections(t *testing.T) {
	for _, tt := range []struct {
		direction string
		from      string
		arrow     string
		to        string
	}{
		{direction: "", from: "n_api", arrow: "-->", to: "n_db"},
		{direction: "forward", from: "n_api", arrow: "-->", to: "n_db"},
		{direction: "backward", from: "n_db", arrow: "-->", to: "n_api"},
		{direction: "both", from: "n_api", arrow: "<-->", to: "n_db"},
		{direction: "bidirectional", from: "n_api", arrow: "<-->", to: "n_db"},
		{direction: "none", from: "n_api", arrow: "---", to: "n_db"},
		{direction: " Both ", from: "n_api", arrow: "<-->", to: "n_db"},
	} {
		for _, label := range []string{"", "reads"} {
			t.Run(tt.direction+"/"+label, func(t *testing.T) {
				dir := t.TempDir()
				cmd.MustInitWorkspace(t, dir)
				if err := os.WriteFile(filepath.Join(dir, ".tld", "elements.yaml"), []byte("api:\n  name: API\ndb:\n  name: DB\n"), 0600); err != nil {
					t.Fatal(err)
				}
				connectors := fmt.Sprintf("- view: root\n  source: api\n  target: db\n  label: %q\n  direction: %q\n", label, tt.direction)
				if err := os.WriteFile(filepath.Join(dir, ".tld", "connectors.yaml"), []byte(connectors), 0600); err != nil {
					t.Fatal(err)
				}
				stdout, _ := cmd.MustRunCmd(t, dir, "render", "root")
				labelSyntax := ""
				if label != "" {
					labelSyntax = "|" + label + "|"
				}
				want := fmt.Sprintf("  %s %s%s %s\n", tt.from, tt.arrow, labelSyntax, tt.to)
				if !strings.Contains(stdout, want) {
					t.Fatalf("render changed connector direction; want %q in:\n%s", want, stdout)
				}
				if _, err := mcheck.Parse(stdout); err != nil {
					t.Fatalf("invalid Mermaid: %v\n%s", err, stdout)
				}
				outPath := filepath.Join(t.TempDir(), "diagram.mmd")
				cmd.MustRunCmd(t, dir, "render", "root", "-o", outPath)
				data, err := os.ReadFile(outPath)
				if err != nil || string(data) != stdout {
					t.Fatalf("file export differs from stdout: %v\n%s", err, data)
				}
			})
		}
	}
}

func TestRenderCmd_PreservesDatabaseConnectorDirections(t *testing.T) {
	dir := t.TempDir()
	cmd.MustRunCmd(t, dir, "add", "API")
	cmd.MustRunCmd(t, dir, "add", "DB")
	for _, direction := range []string{"forward", "backward", "both", "none"} {
		cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--direction", direction, "--label", direction)
	}
	stdout, _ := cmd.MustRunCmd(t, dir, "render", "root")
	for _, want := range []string{
		"n_api -->|forward| n_db",
		"n_db -->|backward| n_api",
		"n_api <-->|both| n_db",
		"n_api ---|none| n_db",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("render lost database connector direction %q:\n%s", want, stdout)
		}
	}
}

func TestRenderCmd_UnknownDirectionFailsWithoutOverwritingOutput(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API")
	cmd.MustRunCmd(t, dir, "add", "DB")
	connectors := "- view: root\n  source: api\n  target: db\n  direction: sideways\n"
	if err := os.WriteFile(filepath.Join(dir, ".tld", "connectors.yaml"), []byte(connectors), 0600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "diagram.mmd")
	if err := os.WriteFile(outPath, []byte("existing diagram"), 0600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := cmd.RunCmd(t, dir, "render", "root", "-o", outPath)
	if err == nil || !strings.Contains(err.Error(), `unsupported direction "sideways"`) || stdout != "" {
		t.Fatalf("expected direction error with no stdout, got %v\n%s", err, stdout)
	}
	if data, err := os.ReadFile(outPath); err != nil || string(data) != "existing diagram" {
		t.Fatalf("failed render overwrote existing output: %v\n%s", err, data)
	}
}

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

	outPath := filepath.Join(dir, "diagram.md")
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

func TestRenderCmd_ViewFlagMatchesPositional(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")

	stdout, _, err := cmd.RunCmd(t, dir, "render", "--view", "root")
	if err != nil {
		t.Fatalf("render --view: %v", err)
	}
	if !strings.Contains(stdout, "flowchart LR") {
		t.Fatalf("expected mermaid header, got:\n%s", stdout)
	}
}

func TestRenderCmd_RequiresView(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "render")
	if err == nil {
		t.Fatal("expected error when no view is provided")
	}
	if !strings.Contains(err.Error(), "view is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
