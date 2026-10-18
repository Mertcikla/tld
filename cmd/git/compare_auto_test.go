package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"github.com/mertcikla/tld/v2/internal/mermaid"
)

func autoCompareFixture(changed, contextNodes int) *model.Diagram {
	scene := &model.Assembly{SchemaVersion: impact.PipelineVersion, RequestedRadius: 2,
		Views: map[string]*model.ViewContent{"1": {}}, Provenance: map[string]*model.Metadata{},
		Tree: []*diagv1.View{{Id: 1, Name: "Changes"}}}
	for i := 1; i <= changed+contextNodes; i++ {
		id := int32(i)
		key := impact.PlacementKey(1, int64(id))
		name := fmt.Sprintf("file-%02d.go", i)
		p := &model.Metadata{Key: key, ViewId: 1, ResourceId: int64(id), Role: pb.ImpactRole_IMPACT_ROLE_DIRECT}
		if i > changed {
			p.Role, p.Distance = pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT, 1
		} else {
			scene.Files = append(scene.Files, &pb.ImpactFile{Path: name, Representatives: []*pb.ImpactPlacementRef{{ViewId: 1, ElementId: id}}})
		}
		scene.Provenance[key] = p
		scene.Views["1"].Placements = append(scene.Views["1"].Placements, &model.Placement{Element: &diagv1.PlacedElement{ViewId: 1, ElementId: id, Name: name}})
	}
	return &model.Diagram{Scene: scene}
}

func autoCompareEdge(diagram *model.Diagram, from, to int32) {
	v := diagram.Scene.Views["1"]
	v.Connectors = append(v.Connectors, &diagv1.Connector{Id: int32(len(v.Connectors) + 1), ViewId: 1, SourceElementId: from, TargetElementId: to})
}

func TestAutoCompareExpandsSmallChangesAndHonorsBudgets(t *testing.T) {
	diagram := autoCompareFixture(1, 4)
	autoCompareEdge(diagram, 1, 2)
	autoCompareEdge(diagram, 2, 3)
	diagram.Scene.Provenance[impact.PlacementKey(1, 3)].Distance = 2
	for _, test := range []struct {
		name   string
		opts   compareOptions
		radius uint32
		mode   string
	}{
		{"expand", compareOptions{markdown: true, blastRadius: 2}, 1, "diagram"},
		{"explicit zero", compareOptions{markdown: true}, 0, "summary"},
		{"node budget", compareOptions{markdown: true, blastRadius: 2, maxNodes: 1}, 0, "summary"},
		{"element limit", compareOptions{markdown: true, blastRadius: 2, maxElements: 1}, 0, "summary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			render := compareRenderer{ctx: context.Background(), opts: test.opts}
			result, report, reason, err := selectAutoCompare(diagram, test.opts, render.build)
			if err != nil || reason != "" || result.radius != test.radius || report.Mode != test.mode {
				t.Fatalf("radius=%d report=%+v reason=%q error=%v", result.radius, report, reason, err)
			}
			if len(result.diagram.Scene.Files) != 1 {
				t.Fatal("lost changed file")
			}
		})
	}
}

func TestAutoCompareSplitsAndProducesValidMermaid(t *testing.T) {
	diagram := autoCompareFixture(16, 0)
	for i := int32(1); i < 16; i++ {
		if i != 8 {
			autoCompareEdge(diagram, i, i+1)
		}
	}
	opts := compareOptions{markdown: true, blastRadius: 2}
	render := compareRenderer{ctx: context.Background(), opts: opts}
	result, report, reason, err := selectAutoCompare(diagram, opts, render.build)
	if err != nil || reason != "" || report.Mode != "split" || report.Diagrams != 2 {
		t.Fatalf("report=%+v reason=%s err=%v", report, reason, err)
	}
	blocks := strings.Split(result.text, "```mermaid\n")
	if len(blocks) != 3 {
		t.Fatalf("expected two fenced diagrams: %s", result.text)
	}
	for _, block := range blocks[1:] {
		code, _, _ := strings.Cut(block, "```")
		if _, err := mermaid.Parse(code); err != nil {
			t.Fatalf("invalid Mermaid: %v\n%s", err, code)
		}
	}
	// Raw Mermaid must remain one document, even for disconnected islands.
	opts.markdown, opts.mermaid = false, true
	render.opts = opts
	result, report, reason, err = selectAutoCompare(diagram, opts, render.build)
	if err != nil || reason != "" || report.Diagrams != 1 || strings.Count(result.text, "flowchart LR") != 1 {
		t.Fatalf("raw Mermaid: report=%+v reason=%s err=%v", report, reason, err)
	}
}

func TestAutoCompareDeclinesUnreadableChangesWithoutDroppingFiles(t *testing.T) {
	for _, test := range []struct {
		name  string
		nodes int
		opts  compareOptions
	}{
		{"node density", 25, compareOptions{markdown: true}},
		{"file scope", 51, compareOptions{markdown: true}},
		{"bytes", 2, compareOptions{markdown: true, maxBytes: 1}},
		{"hard connector limit", 4, compareOptions{markdown: true, maxConnectors: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagram := autoCompareFixture(test.nodes, 0)
			for i := int32(1); i < int32(test.nodes); i++ {
				autoCompareEdge(diagram, i, i+1)
			}
			render := compareRenderer{ctx: context.Background(), opts: test.opts}
			result, _, reason, err := selectAutoCompare(diagram, test.opts, render.build)
			if err != nil || reason == "" || result.text != "" || len(result.diagram.Scene.Files) != test.nodes {
				t.Fatalf("reason=%q err=%v files=%d", reason, err, len(result.diagram.Scene.Files))
			}
		})
	}
}

func TestAutoCompareCLIReportAndExplicitRadius(t *testing.T) {
	dir := compareRepo(t)
	writeSource(t, dir, "go.mod", "module example.com/auto\n\ngo 1.24\n")
	writeSource(t, dir, "a.go", "package a\nfunc A() int { return B() }\n")
	writeSource(t, dir, "b.go", "package a\nfunc B() int { return 1 }\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "connected baseline")
	writeSource(t, dir, "a.go", "package a\nfunc A() int { return B()+1 }\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "change caller")
	reportPath := filepath.Join(t.TempDir(), "report.json")
	args := []string{"compare", dir, "HEAD^", "HEAD", "--auto", "--markdown", "--report-json", reportPath}
	out, _, err := runGitCompare(t, args...)
	if err != nil || !strings.Contains(out, "```mermaid") || !strings.Contains(out, "b.go") {
		t.Fatalf("auto did not add context: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report CompareReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Auto == nil || report.Auto.Mode != "diagram" || report.RequestedRadius != 2 || report.EffectiveRadius != 1 || report.Stats.Files != 1 || report.Elements != 2 {
		t.Fatalf("report=%+v", report)
	}
	out, _, err = runGitCompare(t, append(args, "--blast-radius", "0")...)
	if err != nil || strings.Contains(out, "b.go") || !strings.Contains(out, "Changed files") {
		t.Fatalf("explicit radius ignored: %v\n%s", err, out)
	}
	out, stderr, err := runGitCompare(t, append(args, "--max-bytes", "1")...)
	if err != nil || out != "" || !strings.Contains(stderr, "--max-bytes") {
		t.Fatalf("auto budget skip: %v stdout=%s stderr=%s", err, out, stderr)
	}
}

func TestAutoSummaryEscapesPathsAndKeepsUnknownCounts(t *testing.T) {
	diagram := autoCompareFixture(1, 0)
	diagram.Scene.Files[0].Path = "a</code><script>oops</script>`\nfile.go"
	text, mode, count, _ := autoMarkdown(diagram.Scene, []*model.Assembly{diagram.Scene}, (compareOptions{}).sceneExportOptions())
	if mode != "summary" || count != 0 || strings.Contains(text, "<script>") || strings.Contains(text, "`\n") || strings.Contains(text, "+0") {
		t.Fatalf("unsafe or misleading summary: %s", text)
	}
}
