package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestCompareHardLimits(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Nodes: []*pb.ImpactNode{{Key: "a"}, {Key: "b"}},
		Edges: []*pb.ImpactEdge{{FromKey: "a", ToKey: "b"}, {FromKey: "b", ToKey: "a"}},
	}
	for _, test := range []struct {
		name   string
		opts   compareOptions
		status string
	}{
		{"disabled", compareOptions{}, "ready"},
		{"exact boundary", compareOptions{maxElements: 2, maxConnectors: 2}, "ready"},
		{"elements", compareOptions{maxElements: 1}, "skipped"},
		{"connectors", compareOptions{maxConnectors: 1}, "skipped"},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := comparisonReport(diagram, test.opts)
			if report.Status != test.status || report.Elements != 2 || report.Connectors != 2 {
				t.Fatalf("report = %+v", report)
			}
		})
	}
	if report := comparisonReport(&pb.ImpactDiagram{}, compareOptions{}); report.Status != "empty" {
		t.Fatalf("empty report = %+v", report)
	}
}

func TestComparisonStats(t *testing.T) {
	added, removed := uint32(12), uint32(3)
	diagram := &pb.ImpactDiagram{
		Diff: &pb.SnapshotDiff{
			Sources: []*pb.SourceChange{
				{Path: "internal/api/routes.go", LinesAdded: &added, LinesRemoved: &removed},
				{Path: "internal/api/handlers.go"},
				{Path: "README.md"},
			},
			Facts: &pb.CodeFactDelta{
				Added:    []*pb.CodeFact{{Kind: pb.FactKind_FACT_KIND_FUNCTION}, {Kind: pb.FactKind_FACT_KIND_FILE}},
				Modified: []*pb.CodeFact{{Kind: pb.FactKind_FACT_KIND_METHOD}},
				Removed:  []*pb.CodeFact{{Kind: pb.FactKind_FACT_KIND_FILE}},
			},
		},
		Edges: []*pb.ImpactEdge{
			{FromKey: "file|a.go", ToKey: "file|b.go", Change: pb.ChangeKind_CHANGE_KIND_ADDED},
			{FromKey: "file|b.go", ToKey: "file|c.go", Change: pb.ChangeKind_CHANGE_KIND_REMOVED},
			{FromKey: "file|c.go", ToKey: "file|a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED},
			{FromKey: "file|a.go", ToKey: "file|c.go"},
		},
	}
	stats := comparisonStats(diagram)
	if stats.Files != 3 || stats.Directories != 2 || stats.Subsystems != 2 {
		t.Fatalf("scope = %+v", stats)
	}
	if stats.LinesAdded != 12 || stats.LinesRemoved != 3 {
		t.Fatalf("churn = %+v", stats)
	}
	if stats.SymbolsAdded != 1 || stats.SymbolsModified != 1 || stats.SymbolsRemoved != 0 {
		t.Fatalf("symbols = %+v", stats)
	}
	if stats.EdgesAdded != 1 || stats.EdgesRemoved != 1 || stats.EdgesModified != 1 {
		t.Fatalf("edge churn = %+v", stats)
	}
	if len(stats.Paths) != 3 || stats.PathsTruncated {
		t.Fatalf("paths = %+v", stats)
	}
	if report := comparisonReport(diagram, compareOptions{}); report.Stats.Files != 3 {
		t.Fatalf("report stats = %+v", report)
	}
}

func TestCompareReportAndSkip(t *testing.T) {
	dir := compareRepo(t)
	writeSource(t, dir, "b.go", "package a\nfunc B() {}\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "another file")
	reportPath := filepath.Join(t.TempDir(), "report.json")
	out, stderr, err := runGitCompare(t, "compare", dir, "HEAD~2", "HEAD", "--markdown", "--max-elements", "1", "--report-json", reportPath)
	if err != nil || out != "" || !strings.Contains(stderr, "diagram has 2 elements") {
		t.Fatalf("skip: output=%q stderr=%q error=%v", out, stderr, err)
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report CompareReport
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "skipped" || report.Elements != 2 || len(report.Base) != 40 || len(report.Head) != 40 || report.Warnings == nil {
		t.Fatalf("report = %+v", report)
	}
	// A skipped render still leaves both snapshots reusable.
	out, _, err = runGitCompare(t, "compare", dir, "HEAD~2", "HEAD", "--markdown", "--max-elements", "2")
	if err != nil || !strings.Contains(out, "```mermaid") {
		t.Fatalf("boundary: %q %v", out, err)
	}
}

func TestComparePreparationAndReuse(t *testing.T) {
	dir := compareRepo(t)
	gitRun(t, dir, "remote", "add", "origin", "https://github.com/example/pr-bot-fixture.git")
	args := []string{"compare", dir, "HEAD~1", "HEAD", "--markdown", "--prepare-command", "test -f a.go && echo prepared"}
	_, stderr, err := runGitCompare(t, args...)
	if err != nil || strings.Count(stderr, "prepared") != 2 {
		t.Fatalf("prepare: %q %v", stderr, err)
	}
	_, stderr, err = runGitCompare(t, args...)
	if err != nil || strings.Contains(stderr, "prepared") {
		t.Fatalf("cache hit ran setup: %q %v", stderr, err)
	}
	// Subsequent CI runs can use different checkout paths. Identity by
	// remote must preserve the cached BASE across those checkouts.
	clone := filepath.Join(t.TempDir(), "clone")
	if raw, cloneErr := exec.Command("git", "clone", "--quiet", "--local", dir, clone).CombinedOutput(); cloneErr != nil {
		t.Fatalf("clone: %v %s", cloneErr, raw)
	}
	gitRun(t, clone, "remote", "set-url", "origin", "https://github.com/example/pr-bot-fixture.git")
	gitRun(t, clone, "config", "user.name", "Test")
	gitRun(t, clone, "config", "user.email", "test@example.com")
	cloneArgs := append([]string(nil), args...)
	cloneArgs[1] = clone
	_, stderr, err = runGitCompare(t, cloneArgs...)
	if err != nil || strings.Contains(stderr, "prepared") {
		t.Fatalf("cache moved between checkouts: %q %v", stderr, err)
	}
	writeSource(t, clone, "a.go", "package a\nfunc A() { return 3 }\n")
	gitRun(t, clone, "add", ".")
	gitRun(t, clone, "commit", "-m", "updated PR")
	cloneArgs[2] = "HEAD~2"
	_, stderr, err = runGitCompare(t, cloneArgs...)
	if err != nil || strings.Count(stderr, "prepared") != 1 {
		t.Fatalf("updated PR should reuse BASE: %q %v", stderr, err)
	}
	// An edited setup command must invalidate both snapshots.
	args[len(args)-1] = "test -f a.go && echo prepared-again"
	_, stderr, err = runGitCompare(t, args...)
	if err != nil || strings.Count(stderr, "prepared-again") != 2 {
		t.Fatalf("invalidate: %q %v", stderr, err)
	}
	args[len(args)-1] = "exit 7"
	out, _, err := runGitCompare(t, args...)
	if err == nil || strings.Contains(out, "flowchart LR") || !strings.Contains(err.Error(), "prepare revision") {
		t.Fatalf("failed setup: %q %v", out, err)
	}
}

func TestCompareReportIncludesIndexWarnings(t *testing.T) {
	dir := compareRepo(t)
	t.Setenv("CODEINDEX_SCIP_GO", "missing-scip-for-report-test")
	writeSource(t, dir, "go.mod", "module example.com/report\n\ngo 1.26.2\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "Go project")
	reportPath := filepath.Join(t.TempDir(), "report.json")
	_, _, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "--markdown", "--report-json", reportPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report CompareReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) == 0 || !strings.Contains(strings.Join(report.Warnings, "\n"), "missing-scip-for-report-test") {
		t.Fatalf("warnings missing: %+v", report)
	}
}

func TestComparePreparationCancellation(t *testing.T) {
	dir := compareRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd := NewGitCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"compare", dir, "HEAD~1", "HEAD", "--prepare-command", "exec sleep 20"})
	start := time.Now()
	if err := cmd.Execute(); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("cancellation: %v after %s", err, time.Since(start))
	}
}

func TestCompareMermaidAndProtoJSON(t *testing.T) {
	dir := compareRepo(t)

	mermaid, stderr, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "--mermaid", "--verbose")
	if err != nil {
		t.Fatalf("mermaid compare: %v\n%s", err, stderr)
	}
	if !strings.Contains(mermaid, "flowchart LR") || !strings.Contains(mermaid, "a.go") {
		t.Fatalf("mermaid output = %q", mermaid)
	}
	if !strings.Contains(mermaid, "%% grounded") {
		t.Fatalf("grounded mermaid header missing: %q", mermaid)
	}
	// Stats comments must follow the flowchart directive: strict renderers
	// reject comments before the diagram type.
	if flowchartAt := strings.Index(mermaid, "flowchart LR"); flowchartAt < 0 || strings.Index(mermaid, "%% grounded") < flowchartAt {
		t.Fatalf("mermaid comments must follow the flowchart directive: %q", mermaid)
	}
	// The default mermaid render is the change scene: the uncovered file
	// surfaces as a transient placement in the fallback Changes view, exactly
	// as the canvas draws it.
	for _, want := range []string{
		"%% tld-scene",
		`subgraph view_neg1["Changes"]`,
		`el_neg1_neg1["◇ a.go`,
	} {
		if !strings.Contains(mermaid, want) {
			t.Fatalf("scene mermaid missing %q in:\n%s", want, mermaid)
		}
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
	var bundle map[string]any
	if err := json.Unmarshal([]byte(out), &bundle); err != nil {
		t.Fatalf("protojson output: %v\n%s", err, out)
	}
	if bundle["mode"] != "grounded" {
		t.Fatalf("bundle mode = %v, want grounded", bundle["mode"])
	}
	sceneRaw, _ := json.Marshal(bundle["scene"])
	var scene map[string]any
	if err := json.Unmarshal(sceneRaw, &scene); err != nil {
		t.Fatalf("bundle scene: %v\n%s", err, out)
	}
	if scene["comparisonKey"] == nil || scene["repositoryId"] == nil || scene["schemaVersion"] != impact.SceneSchemaVersion {
		t.Fatalf("scene is not self-identifying: %v", scene)
	}
	if _, ok := scene["diff"]; ok {
		t.Fatalf("scene must not carry the index diff: %v", scene["diff"])
	}
	overlay := findOverlay(t, scene)
	if overlay == nil {
		t.Fatalf("changed file has no overlay: %v", scene["views"])
	}
	if overlay["path"] != "a.go" || overlay["distance"] != float64(0) {
		t.Fatalf("overlay = %v", overlay)
	}
	symbols, _ := overlay["symbols"].([]any)
	if len(symbols) == 0 {
		t.Fatalf("overlay carries no symbol details: %v", overlay)
	}
	symbol, _ := symbols[0].(map[string]any)
	if symbol["name"] != "A" || symbol["change"] != "CHANGE_KIND_MODIFIED" {
		t.Fatalf("symbol detail = %v", symbol)
	}
	if _, ok := symbol["snapshotId"]; ok {
		t.Fatalf("symbol detail must stay snapshot-free: %v", symbol)
	}
}

// findOverlay returns the first overlay in a serialized scene, or nil.
func findOverlay(t *testing.T, scene map[string]any) map[string]any {
	t.Helper()
	views, _ := scene["views"].(map[string]any)
	for _, content := range views {
		view, _ := content.(map[string]any)
		placements, _ := view["placements"].([]any)
		for _, item := range placements {
			placement, _ := item.(map[string]any)
			overlay, _ := placement["overlay"].(map[string]any)
			if overlay != nil {
				return overlay
			}
		}
	}
	return nil
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

func TestScopeToBudgetNarrowsRadius(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Nodes: []*pb.ImpactNode{
			{Key: "file|a.go", Path: "a.go", Name: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED},
			{Key: "context|1", Path: "b.go", Name: "b.go", Distance: 1, ElementId: 1},
			{Key: "context|2", Path: "c.go", Name: "c.go", Distance: 2, ElementId: 2},
		},
	}
	// A payload proportional to the node count lets the budget walk the radius
	// down the same way a real render does.
	build := func(scoped *pb.ImpactDiagram, radius uint32) (string, int, error) {
		size := len(scoped.GetNodes()) * 1000
		return fmt.Sprintf("radius=%d nodes=%d", radius, len(scoped.GetNodes())), size, nil
	}
	result, err := scopeToBudget(diagram, 2, compareOptions{maxNodes: 2}, build)
	if err != nil {
		t.Fatal(err)
	}
	if !result.limited || result.radius != 1 || len(result.diagram.GetNodes()) != 2 {
		t.Fatalf("node budget: radius=%d limited=%v nodes=%d", result.radius, result.limited, len(result.diagram.GetNodes()))
	}
	if result.text != "radius=1 nodes=2" || result.size != 2000 {
		t.Fatalf("payload does not match the chosen radius: %q (%d bytes)", result.text, result.size)
	}
	result, err = scopeToBudget(diagram, 2, compareOptions{maxNodes: 2, maxBytes: 1500}, build)
	if err != nil {
		t.Fatal(err)
	}
	if !result.limited || result.radius != 0 || result.size != 1000 || result.overBudget {
		t.Fatalf("byte budget: radius=%d limited=%v size=%d over=%v", result.radius, result.limited, result.size, result.overBudget)
	}
	result, err = scopeToBudget(diagram, 2, compareOptions{maxNodes: 2, maxBytes: 1}, build)
	if err != nil {
		t.Fatal(err)
	}
	if !result.overBudget || result.radius != 0 {
		t.Fatalf("unmeetable budget: radius=%d over=%v size=%d", result.radius, result.overBudget, result.size)
	}
}

// The scene render feeds the size accounting, so its byte size always
// describes the payload the caller receives.
func TestCompareRendererMermaidSizes(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	snap := &pb.Snapshot{Id: "head", RepositoryId: "repo", GitRevision: "head", Provenance: "commit", IngestionStatus: "complete"}
	if err := idx.Publish(ctx, "/repo", snap, graph.NewGraph("repo", "head")); err != nil {
		t.Fatal(err)
	}
	service := impact.Service{Workspace: ws, Index: idx}
	diagram := &pb.ImpactDiagram{
		RepositoryId:  "repo",
		ComparisonKey: "key",
		Diff: &pb.SnapshotDiff{
			ToSnapshotId: "head",
			Sources:      []*pb.SourceChange{{Path: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED}},
		},
		Nodes: []*pb.ImpactNode{{Key: "file|a.go", Path: "a.go", Name: "a.go", Change: pb.ChangeKind_CHANGE_KIND_MODIFIED}},
	}
	for _, opts := range []compareOptions{{mermaid: true}, {markdown: true}} {
		text, size, err := compareRenderer{ctx: ctx, service: service, opts: opts}.build(diagram, 0)
		if err != nil {
			t.Fatal(err)
		}
		if size != len(text) {
			t.Fatalf("size %d vs %d bytes for %+v", size, len(text), opts)
		}
		if !strings.HasPrefix(text, "```mermaid") && !strings.HasPrefix(text, "flowchart") {
			t.Fatalf("unexpected mermaid payload for %+v: %q", opts, text)
		}
		// The scene render carries the uncovered file as a transient, exactly
		// as the canvas draws it.
		if !strings.Contains(text, `subgraph view_neg1["Changes"]`) {
			t.Fatalf("scene mermaid missing fallback view for %+v:\n%s", opts, text)
		}
	}
}

func TestCompareRendererFallsBackToFactGraph(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	snap := &pb.Snapshot{Id: "head", RepositoryId: "repo", GitRevision: "head", Provenance: "commit", IngestionStatus: "complete"}
	if err := idx.Publish(ctx, "/repo", snap, graph.NewGraph("repo", "head")); err != nil {
		t.Fatal(err)
	}
	service := impact.Service{Workspace: ws, Index: idx}
	diagram := &pb.ImpactDiagram{
		RepositoryId:  "repo",
		ComparisonKey: "key",
		Diff:          &pb.SnapshotDiff{ToSnapshotId: "head"},
	}
	text, _, err := compareRenderer{ctx: ctx, service: service, opts: compareOptions{mermaid: true}}.build(diagram, 0)
	if err != nil {
		t.Fatal(err)
	}
	// No view survived: the file-level fact graph renders instead of a blank diagram.
	if !strings.Contains(text, "%% tld-impact") {
		t.Fatalf("empty scene must fall through to the fact graph:\n%s", text)
	}
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

func TestCompareHasNoScopeOrRawFlags(t *testing.T) {
	cmd := NewGitCmd()
	commands := cmd.Commands()
	if len(commands) == 0 {
		t.Fatal("compare subcommand not found")
	}
	found := commands[0]
	if found.Flags().Lookup("raw-impact") != nil {
		t.Fatal("--raw-impact flag must be gone; grounded is the only diagram")
	}
	if found.Flags().Lookup("scope") != nil {
		t.Fatal("--scope flag must be gone; grounded is the only diagram")
	}
	if found.Flags().Lookup("view") == nil || found.Flags().Lookup("all-edges") == nil {
		t.Fatal("--view and --all-edges flags are required")
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
