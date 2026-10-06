package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
)

func TestCompareMermaidAndProtoJSON(t *testing.T) {
	dir := compareRepo(t)

	mermaid, stderr, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "--mermaid", "--verbose")
	if err != nil {
		t.Fatalf("mermaid compare: %v\n%s", err, stderr)
	}
	if !strings.Contains(mermaid, "flowchart LR") || !strings.Contains(mermaid, "a.go") {
		t.Fatalf("mermaid output = %q", mermaid)
	}
	if !strings.Contains(stderr, "Parse sources") || !strings.Contains(stderr, "Save diff diagram") {
		t.Fatalf("progress output = %q", stderr)
	}
	baseAt := strings.Index(stderr, "base HEAD~1")
	headAt := strings.Index(stderr, "head HEAD")
	overlayAt := strings.Index(stderr, "Save diff diagram")
	if baseAt < 0 || headAt < 0 || overlayAt < 0 {
		t.Fatalf("comparison sides not labelled: %q", stderr)
	}
	if baseAt > headAt || headAt > overlayAt {
		t.Fatalf("side labels out of order:\n%s", stderr)
	}

	out, stderr, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("protojson compare: %v\n%s", err, stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("protojson output: %v\n%s", err, out)
	}
	if payload["comparisonKey"] == nil || payload["repositoryId"] == nil {
		t.Fatalf("protojson missing keys: %v", payload)
	}
	nodes, ok := payload["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes = %v", payload["nodes"])
	}
	if node := nodes[0].(map[string]any); node["distance"] != float64(0) {
		t.Fatalf("node distance = %v", node["distance"])
	}
}

// Indexing both revisions must stay silent on stderr unless progress is asked
// for, so scripted runs only carry the diagram.
func TestCompareKeepsProgressBehindVerbose(t *testing.T) {
	t.Run("quiet", func(t *testing.T) {
		dir := compareRepo(t)
		out, stderr, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "--mermaid")
		if err != nil {
			t.Fatalf("compare: %v\n%s", err, stderr)
		}
		if !strings.Contains(out, "flowchart LR") {
			t.Fatalf("mermaid output = %q", out)
		}
		if strings.TrimSpace(stderr) != "" {
			t.Fatalf("progress leaked without --verbose: %q", stderr)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		dir := compareRepo(t)
		_, stderr, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "-v")
		if err != nil {
			t.Fatalf("compare: %v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "base HEAD~1") || !strings.Contains(stderr, "head HEAD") {
			t.Fatalf("verbose progress missing sides: %q", stderr)
		}
	})
}

// Budget warnings stay visible without --verbose so CI logs record them.
func TestCompareWarnsWithoutVerbose(t *testing.T) {
	dir := compareRepo(t)
	_, stderr, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "--max-bytes", "1")
	if err != nil {
		t.Fatalf("compare: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "warning: output is") {
		t.Fatalf("expected budget warning on stderr: %q", stderr)
	}
}

// compareRepo isolates the config and data directories and returns a checkout
// with two commits, so a comparison has to index both revisions from scratch.
func compareRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	writeSource(t, dir, "a.go", "package a\nfunc A() { return 1 }\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial")
	writeSource(t, dir, "a.go", "package a\nfunc A() { return 2 }\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "change")
	return dir
}

func TestCompareScopeResolvesDepthAndRadius(t *testing.T) {
	if display, depth := compareScope(compareOptions{depth: 2}, false); display != 2 || depth != 2 {
		t.Fatalf("depth only: display=%d depth=%d", display, depth)
	}
	if display, depth := compareScope(compareOptions{depth: 3, radius: 1}, true); display != 1 || depth != 3 {
		t.Fatalf("radius override: display=%d depth=%d", display, depth)
	}
	if display, depth := compareScope(compareOptions{depth: 1, radius: 2}, true); display != 2 || depth != 2 {
		t.Fatalf("radius deeper: display=%d depth=%d", display, depth)
	}
	if display, depth := compareScope(compareOptions{depth: impact.DefaultContextDepth}, false); display != impact.DefaultContextDepth || depth != impact.DefaultContextDepth {
		t.Fatalf("defaults: display=%d depth=%d", display, depth)
	}
}

func TestScopeToBudgetNarrowsRadius(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Nodes: []*pb.ImpactNode{
			{Key: "file|a.go", Path: "a.go", Name: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED},
			{Key: "context|1", Path: "b.go", Name: "b.go", Distance: 1, ElementId: 1},
			{Key: "context|2", Path: "c.go", Name: "c.go", Distance: 2, ElementId: 2},
		},
	}
	result := scopeToBudget(diagram, 2, compareOptions{maxNodes: 2}, outputSize(compareOptions{}))
	if !result.limited || result.radius != 1 || len(result.diagram.GetNodes()) != 2 {
		t.Fatalf("scopeToBudget: radius=%d limited=%v nodes=%d", result.radius, result.limited, len(result.diagram.GetNodes()))
	}
	result = scopeToBudget(diagram, 2, compareOptions{maxNodes: 2, maxBytes: 1}, outputSize(compareOptions{}))
	if !result.limited || result.radius != 0 || len(result.diagram.GetNodes()) != 1 {
		t.Fatalf("byte budget: radius=%d limited=%v nodes=%d", result.radius, result.limited, len(result.diagram.GetNodes()))
	}
}

// The byte budget follows the selected output format, so a Mermaid run is not
// measured (or narrowed) against the much larger protojson encoding.
func TestScopeToBudgetMeasuresSelectedFormat(t *testing.T) {
	facts := make([]*pb.CodeFact, 40)
	for i := range facts {
		facts[i] = &pb.CodeFact{Name: fmt.Sprintf("SymbolWithAVeryLongName%02d", i), BodyHash: strings.Repeat("a", 32)}
	}
	diagram := &pb.ImpactDiagram{
		Nodes: []*pb.ImpactNode{
			{Key: "file|a.go", Path: "a.go", Name: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED,
				Symbols: &pb.CodeFactDelta{Modified: facts}},
			{Key: "context|1", Path: "b.go", Name: "b.go", Distance: 1, ElementId: 1},
		},
		MaxRadius: 1,
	}
	opts := compareOptions{maxNodes: 10, maxBytes: 2048}
	protojson := scopeToBudget(diagram, 1, opts, outputSize(opts))
	mermaid := scopeToBudget(diagram, 1, opts, outputSize(compareOptions{mermaid: true}))
	if !protojson.overBudget || protojson.size <= 2048 {
		t.Fatalf("protojson size %d should exceed the budget", protojson.size)
	}
	if mermaid.overBudget || mermaid.limited || mermaid.radius != 1 || mermaid.size >= 2048 {
		t.Fatalf("mermaid size %d narrowed the diagram: %+v", mermaid.size, mermaid)
	}
}

func TestRenderDiagramSizesTheEmittedPayload(t *testing.T) {
	diagram := &pb.ImpactDiagram{Nodes: []*pb.ImpactNode{{Key: "file|a.go", Path: "a.go", Name: "a.go"}}}
	for _, opts := range []compareOptions{{}, {mermaid: true}, {markdown: true}} {
		payload, size, err := renderDiagram(diagram, opts, 0)
		if err != nil {
			t.Fatal(err)
		}
		if want := len(payload) + protoJSONNewline(opts); size != want {
			t.Fatalf("size %d, want %d for %+v", size, want, opts)
		}
	}
}

// protoJSONNewline reports the trailing newline the protojson writer adds on
// top of the marshalled payload.
func protoJSONNewline(opts compareOptions) int {
	if opts.mermaid || opts.markdown {
		return 0
	}
	return 1
}

func TestCompareTargetLabelNamesSideAndRevision(t *testing.T) {
	out := &bytes.Buffer{}
	if got := compareTargetLabel(out, impact.TargetBase, "HEAD~1"); got != "base HEAD~1" {
		t.Fatalf("label = %q", got)
	}
	if got := compareTargetLabel(out, impact.TargetHead, ""); got != "head" {
		t.Fatalf("label without revision = %q", got)
	}
}

func runGitCompare(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewGitCmd()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	raw, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, raw)
	}
	return strings.TrimSpace(string(raw))
}

func writeSource(t *testing.T, dir, name, code string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
}
