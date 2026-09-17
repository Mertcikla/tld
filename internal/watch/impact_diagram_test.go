package watch

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func diagramNode(ref string) ImpactElement {
	return ImpactElement{Ref: ref, Name: ref, Kind: "component"}
}

func diagramEdge(source, target string, observed bool) ImpactEdge {
	return ImpactEdge{SourceRef: source, TargetRef: target, Label: "calls", Observed: observed}
}

func busyDiagramReport() ImpactReport {
	changed := []ImpactElement{diagramNode("a"), diagramNode("b")}
	var related []ImpactElement
	var edges []ImpactEdge
	edges = append(edges, diagramEdge("a", "b", false))
	for i := 1; i <= 20; i++ {
		ref := "r" + itoa(i)
		related = append(related, diagramNode(ref))
		edges = append(edges, diagramEdge("a", ref, false), diagramEdge("b", ref, false))
	}
	return ImpactReport{Changed: changed, Related: related, Edges: edges}
}

func TestBuildImpactDiagramSingleReviewerProjection(t *testing.T) {
	report := ImpactReport{
		Coverage: Coverage{Applicable: true, Percent: 80, Confidence: "medium"},
		Changed:  []ImpactElement{{Ref: "checkout", Name: "Checkout"}},
		Candidates: []ImpactElement{
			{Ref: "weak", Name: "Weak match"},
		},
		Related: []ImpactElement{{Ref: "payment", Name: "Payment"}},
		Edges:   []ImpactEdge{{SourceRef: "checkout", TargetRef: "payment", Label: "calls", Observed: true}},
	}

	diagram := BuildImpactDiagram(report, DiagramStyleFull)
	if diagram.Style != DiagramStyleReview {
		t.Fatalf("style = %q, want review", diagram.Style)
	}
	if diagram.OmittedNodes != 0 || diagram.OmittedEdges != 0 {
		t.Fatalf("small report should have no omissions: %+v", diagram)
	}
	for _, node := range diagram.Nodes {
		if node.Ref == "weak" {
			t.Fatalf("candidates must never be drawn: %+v", diagram.Nodes)
		}
	}
	for _, want := range []string{
		"%% coverage: 80% (medium)",
		"flowchart LR",
		`subgraph lane_changed["Code touched"]`,
		`n1["Checkout"]`,
		`n2["Payment"]`,
		"n1 -.->|calls| n2",
		"class n1 changed",
	} {
		if !strings.Contains(diagram.Code, want) {
			t.Fatalf("reviewer diagram missing %q:\n%s", want, diagram.Code)
		}
	}

	var rendered bytes.Buffer
	if err := RenderImpactMermaidStyle(&rendered, report, DiagramStyleBounded); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimRight(rendered.String(), "\n"); got != diagram.Code {
		t.Fatalf("RenderImpactMermaid diverged from BuildImpactDiagram:\n%s\n---\n%s", got, diagram.Code)
	}
}

func TestBuildImpactDiagramTransformsPreserveValidRelationships(t *testing.T) {
	report := busyDiagramReport()
	before, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	refs := map[string]struct{}{}
	for _, node := range diagram.Nodes {
		refs[node.Ref] = struct{}{}
	}
	for _, edge := range diagram.Edges {
		if _, ok := refs[edge.SourceRef]; !ok {
			t.Fatalf("edge source %q is not drawn", edge.SourceRef)
		}
		if _, ok := refs[edge.TargetRef]; !ok {
			t.Fatalf("edge target %q is not drawn", edge.TargetRef)
		}
	}
	after, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("BuildImpactDiagram mutated the report")
	}
}

func TestBuildImpactDiagramIsOrderIndependent(t *testing.T) {
	report := busyDiagramReport()
	shuffled := ImpactReport{
		Changed: []ImpactElement{report.Changed[1], report.Changed[0]},
		Related: reverseElements(report.Related),
		Edges:   reverseEdges(report.Edges),
	}
	want := BuildImpactDiagram(report, DiagramStyleReview)
	got := BuildImpactDiagram(shuffled, DiagramStyleLanes)
	if want.Code != got.Code {
		t.Fatalf("output depends on input order or style arg:\n%s\n---\n%s", want.Code, got.Code)
	}
}

func TestBuildImpactDiagramRanksNeighbors(t *testing.T) {
	report := ImpactReport{
		Changed: []ImpactElement{diagramNode("a"), diagramNode("b")},
		Related: []ImpactElement{diagramNode("shared"), diagramNode("observed"), diagramNode("alpha"), diagramNode("zeta")},
		Edges: []ImpactEdge{
			diagramEdge("a", "shared", false),
			diagramEdge("b", "shared", false),
			diagramEdge("a", "observed", true),
			diagramEdge("a", "zeta", false),
			diagramEdge("a", "alpha", false),
		},
	}
	graph, _, _, _ := boundedDiagram(report, 4, defaultDiagramEdgeBudget)
	if got := diagramRefs(graph); strings.Join(got, ",") != "a,b,shared,observed" {
		t.Fatalf("node budget 4 = %v, want [a b shared observed]", got)
	}
	graph, _, _, _ = boundedDiagram(report, 5, defaultDiagramEdgeBudget)
	if got := diagramRefs(graph); strings.Join(got, ",") != "a,b,shared,observed,alpha" {
		t.Fatalf("node budget 5 = %v, want [a b shared observed alpha]", got)
	}
}

func TestBoundedDiagramKeepsChangedAndMutualEdgesBeyondBudget(t *testing.T) {
	report := ImpactReport{
		Changed: []ImpactElement{diagramNode("a"), diagramNode("b"), diagramNode("c")},
		Related: []ImpactElement{diagramNode("d"), diagramNode("e")},
		Edges: []ImpactEdge{
			diagramEdge("a", "b", false),
			diagramEdge("b", "c", false),
			diagramEdge("c", "a", false),
			diagramEdge("d", "a", false),
			diagramEdge("b", "e", true),
		},
	}
	tightNodes, tightEdges, _, _ := boundedDiagram(report, 2, 1)
	if len(tightNodes) != 3 {
		t.Fatalf("nodes = %d, want 3 changed nodes retained", len(tightNodes))
	}
	if len(tightEdges) != 3 {
		t.Fatalf("edges = %d, want 3 mutual changed edges retained", len(tightEdges))
	}
	roomyNodes, roomyEdges, omittedNodes, omittedEdges := boundedDiagram(report, 5, 1)
	if len(roomyEdges) != 5 || omittedNodes != 0 || omittedEdges != 0 {
		t.Fatalf("roomy bounded = %d nodes / %d edges / %d omitted nodes; want all 5 edges and no omissions", len(roomyNodes), len(roomyEdges), omittedNodes)
	}
}

func TestBoundedDiagramReducesBusyReport(t *testing.T) {
	report := busyDiagramReport()
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if len(diagram.Nodes) != 10 {
		t.Fatalf("nodes = %d, want 10", len(diagram.Nodes))
	}
	if len(diagram.Edges) != 16 {
		t.Fatalf("edges = %d, want 16", len(diagram.Edges))
	}
	if want := len(report.Changed) + len(report.Related) - len(diagram.Nodes); diagram.OmittedNodes != want {
		t.Fatalf("omittedNodes = %d, want %d", diagram.OmittedNodes, want)
	}
	if want := len(report.Edges) - len(diagram.Edges); diagram.OmittedEdges != want {
		t.Fatalf("omittedEdges = %d, want %d", diagram.OmittedEdges, want)
	}
	if !strings.Contains(diagram.Code, "%% +") || !strings.Contains(diagram.Code, "omitted") {
		t.Fatalf("trimmed diagram should carry an omission comment:\n%s", diagram.Code)
	}
	touched := diagramRefSet(report.Changed)
	for _, node := range diagram.Nodes {
		if _, ok := touched[node.Ref]; ok {
			continue
		}
		if !hasDiagramEdge(diagram.Edges, node.Ref) {
			t.Fatalf("selected neighbor %q has no drawn edge", node.Ref)
		}
	}
}

func TestBuildImpactDiagramLeavesSparseChangedDisconnected(t *testing.T) {
	report := ImpactReport{
		Changed: []ImpactElement{{Ref: "formatter", Name: "Formatter"}},
		Related: []ImpactElement{{Ref: "plugin", Name: "Plugin registry"}},
	}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if !strings.Contains(diagram.Code, "Formatter") {
		t.Fatalf("changed node missing:\n%s", diagram.Code)
	}
	if strings.Contains(diagram.Code, "Plugin registry") {
		t.Fatalf("disconnected related node should be dropped:\n%s", diagram.Code)
	}
}

func TestLaneForClassifiesRoles(t *testing.T) {
	changed := map[string]struct{}{"core": {}}
	edges := []ImpactEdge{
		diagramEdge("in", "core", false),
		diagramEdge("core", "out", false),
		diagramEdge("both", "core", false),
		diagramEdge("core", "both", false),
	}
	cases := map[string]string{"core": "changed", "in": "incoming", "out": "outgoing", "both": "both"}
	for ref, want := range cases {
		if got := laneFor(ref, changed, edges); got != want {
			t.Fatalf("laneFor(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestBuildImpactDiagramGroupsChangedByDirection(t *testing.T) {
	report := ImpactReport{
		Changed: []ImpactElement{diagramNode("core")},
		Related: []ImpactElement{diagramNode("in"), diagramNode("out"), diagramNode("both")},
		Edges: []ImpactEdge{
			diagramEdge("in", "core", false),
			diagramEdge("core", "out", false),
			diagramEdge("both", "core", false),
			diagramEdge("core", "both", false),
		},
	}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if !strings.HasPrefix(diagram.Code, "%% coverage:") && !strings.HasPrefix(diagram.Code, "flowchart LR") {
		t.Fatalf("reviewer diagram should start with the header and a left-to-right graph:\n%s", diagram.Code)
	}
	if !strings.Contains(diagram.Code, "flowchart LR") {
		t.Fatalf("reviewer diagram should be left-to-right:\n%s", diagram.Code)
	}
	for _, lane := range []string{"lane_incoming", "lane_changed", "lane_outgoing", "lane_both"} {
		if !strings.Contains(diagram.Code, "subgraph "+lane) {
			t.Fatalf("reviewer diagram missing %s:\n%s", lane, diagram.Code)
		}
	}
}

func TestBuildImpactDiagramHandlesEmptyAndDangling(t *testing.T) {
	report := ImpactReport{Edges: []ImpactEdge{diagramEdge("missing", "also-missing", false)}}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if len(diagram.Nodes) != 0 || len(diagram.Edges) != 0 {
		t.Fatalf("expected empty diagram, got %+v", diagram)
	}
	if strings.Contains(strings.ToLower(diagram.Code), "undefined") {
		t.Fatalf("diagram contains undefined ids:\n%s", diagram.Code)
	}
}

func TestNormalizeDiagramStyleAlwaysReturnsReview(t *testing.T) {
	for _, input := range []string{"", "review", "REVIEW", "full", "all", "BOUNDED", "neighborhood", "lanes", "lane", "groups", "grouped", " nope "} {
		if got := NormalizeDiagramStyle(input); got != DiagramStyleReview {
			t.Fatalf("NormalizeDiagramStyle(%q) = %q, want review", input, got)
		}
	}
}

func TestBuildImpactDiagramBadgesChangedNodes(t *testing.T) {
	report := ImpactReport{
		Changed: []ImpactElement{{
			Ref:  "core",
			Name: "Core",
			Evidence: []ImpactEvidence{
				{Level: EvidenceStrong, Kind: "path", Path: "internal/core.go"},
				{Level: EvidenceStrong, Kind: "path", Path: "internal/util.go"},
			},
		}},
		Related: []ImpactElement{{Ref: "api", Name: "API"}},
		Edges: []ImpactEdge{
			{SourceRef: "core", TargetRef: "api"},
			{SourceRef: "api", TargetRef: "core", Observed: true},
		},
		ChangedFiles: []ChangedFile{
			{Path: "internal/core.go", Change: "updated", Added: 12, Removed: 3},
			{Path: "internal/util.go", Change: "updated"},
		},
	}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if diagram.Style != DiagramStyleReview {
		t.Fatalf("style = %q, want review", diagram.Style)
	}
	for _, want := range []string{
		`n1["Core<br/>2 files (+12 -3)"]`,
		`n2["API"]`,
		"n2 -.->|observed| n1",
	} {
		if !strings.Contains(diagram.Code, want) {
			t.Fatalf("reviewer diagram missing %q:\n%s", want, diagram.Code)
		}
	}
	if strings.Contains(diagram.Code, "internal/core.go") {
		t.Fatalf("diagram must not leak file paths into node labels:\n%s", diagram.Code)
	}
	if strings.Contains(diagram.Code, `n2["API<br/>`) {
		t.Fatalf("context node should not be badged:\n%s", diagram.Code)
	}
	if got := BuildImpactDiagram(report, DiagramStyleBounded); got.Code != diagram.Code {
		t.Fatalf("reviewer diagram is not style-independent")
	}
}

func TestChangeBadgeShapes(t *testing.T) {
	stats := map[string]ChangedFile{
		"solo.go": {Path: "solo.go", Change: "updated"},
		"new.go":  {Path: "new.go", Change: "added"},
	}
	node := ImpactElement{Ref: "a", Name: "A", Evidence: []ImpactEvidence{{Path: "solo.go"}}}
	if got := changeBadge(node, stats); got != "1 file" {
		t.Fatalf("badge = %q, want %q", got, "1 file")
	}
	node = ImpactElement{Ref: "b", Name: "B", Evidence: []ImpactEvidence{{Path: "new.go"}}}
	if got := changeBadge(node, stats); got != "1 file (added)" {
		t.Fatalf("badge = %q, want %q", got, "1 file (added)")
	}
	node = ImpactElement{Ref: "c", Name: "C"}
	if got := changeBadge(node, stats); got != "" {
		t.Fatalf("badge = %q, want empty", got)
	}
}

func TestAnalyzeImpactEmitsFolderContainment(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{
			"backend":  {Name: "Backend", FilePath: "backend/**"},
			"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
		},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
		},
	})
	if len(report.Changed) != 1 || report.Changed[0].Ref != "checkout" {
		t.Fatalf("changed = %+v, want only the nested owner", report.Changed)
	}
	if len(report.Related) != 1 || report.Related[0].Ref != "backend" {
		t.Fatalf("related = %+v, want the broader folder rolled up into context", report.Related)
	}
	foundContains := false
	for _, evidence := range report.Related[0].Evidence {
		if evidence.Kind == "contains" {
			foundContains = true
		}
	}
	if !foundContains {
		t.Fatalf("rolled-up ancestor should carry contains evidence: %+v", report.Related[0])
	}
	found := false
	for _, edge := range report.Edges {
		if edge.SourceRef == "backend" && edge.TargetRef == "checkout" && edge.Label == "contains" && !edge.Observed {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing backend --contains--> checkout edge: %+v", report.Edges)
	}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if !strings.Contains(diagram.Code, "--o|contains|") {
		t.Fatalf("diagram should render containment distinctly:\n%s", diagram.Code)
	}
}

func TestAnalyzeImpactContainmentKeepsDirectParentsOnly(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{
			"root": {Name: "Root", FilePath: "backend/**"},
			"mid":  {Name: "Mid", FilePath: "backend/mid/**"},
			"leaf": {Name: "Leaf", FilePath: "backend/mid/leaf/**"},
		},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/mid/leaf/service.go": tldgit.WorktreeUpdated,
		},
	})
	if len(report.Changed) != 1 || report.Changed[0].Ref != "leaf" {
		t.Fatalf("changed = %+v, want only the deepest owner", report.Changed)
	}
	if len(report.Related) != 2 {
		t.Fatalf("related = %+v, want both ancestors rolled up", report.Related)
	}
	var contains []ImpactEdge
	for _, edge := range report.Edges {
		if edge.Label == "contains" {
			contains = append(contains, edge)
		}
	}
	assertEdgeLabel(t, contains, "root", "mid", false, "contains")
	assertEdgeLabel(t, contains, "mid", "leaf", false, "contains")
	if len(contains) != 2 {
		t.Fatalf("contains edges = %+v, want exactly root->mid and mid->leaf (no transitive root->leaf)", contains)
	}
}

func TestAnalyzeImpactContainmentYieldsToDeclaredEdges(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{
			"backend":  {Name: "Backend", FilePath: "backend/**"},
			"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
		},
		Connectors: map[string]*workspace.Connector{
			"calls": {Source: "backend", Target: "checkout", Label: "calls"},
		},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
		},
	})
	for _, edge := range report.Edges {
		if edge.Label == "contains" {
			t.Fatalf("declared edge should suppress containment: %+v", report.Edges)
		}
	}
}

func TestAnalyzeImpactNestingAttributesExclusiveFiles(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{
			"backend":  {Name: "Backend", FilePath: "backend/**"},
			"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
		},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/other.go":            tldgit.WorktreeUpdated,
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
		},
		LineStats: map[string]tldgit.LineDiff{
			"backend/other.go":            {Added: 4, Removed: 1},
			"backend/checkout/service.go": {Added: 10, Removed: 2},
		},
	})
	if len(report.Changed) != 2 {
		t.Fatalf("changed = %+v, want both elements (each owns an exclusive file)", report.Changed)
	}
	badges := map[string]string{}
	stats := map[string]ChangedFile{}
	for _, file := range report.ChangedFiles {
		stats[file.Path] = file
	}
	for _, node := range report.Changed {
		badges[node.Ref] = changeBadge(node, stats)
	}
	if badges["backend"] != "1 file (+4 -1)" {
		t.Fatalf("backend badge = %q, want exclusive file only", badges["backend"])
	}
	if badges["checkout"] != "1 file (+10 -2)" {
		t.Fatalf("checkout badge = %q, want exclusive file only", badges["checkout"])
	}
}

func TestAnalyzeImpactSameSpecificityTieStaysDuplicated(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{
			"a": {Name: "A", FilePath: "backend/**"},
			"b": {Name: "B", FilePath: "backend/**"},
		},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/service.go": tldgit.WorktreeUpdated,
		},
	})
	if len(report.Changed) != 2 {
		t.Fatalf("changed = %+v, want both elements: identical bindings are genuine overlap, not nesting", report.Changed)
	}
	if len(report.Related) != 0 {
		t.Fatalf("related = %+v, want none", report.Related)
	}
}

func TestAnalyzeImpactPopulatesOwner(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{
			"checkout": {Name: "Checkout", FilePath: "backend/checkout/**", Owner: "Commerce"},
			"cart":     {Name: "Cart", Owner: "Commerce"},
		},
		Connectors: map[string]*workspace.Connector{
			"cart": {Source: "checkout", Target: "cart", Label: "calls"},
		},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
		},
	})
	if len(report.Changed) != 1 || report.Changed[0].Owner != "Commerce" {
		t.Fatalf("changed owner = %+v, want Commerce", report.Changed)
	}
	if len(report.Related) != 1 || report.Related[0].Owner != "Commerce" {
		t.Fatalf("related owner = %+v, want Commerce", report.Related)
	}
}

func assertEdgeLabel(t *testing.T, edges []ImpactEdge, source, target string, observed bool, label string) {
	t.Helper()
	for _, edge := range edges {
		if edge.SourceRef == source && edge.TargetRef == target && edge.Observed == observed {
			if edge.Label != label {
				t.Fatalf("edge %s->%s label = %q, want %q", source, target, edge.Label, label)
			}
			return
		}
	}
	t.Fatalf("edge %s->%s (observed=%v) not found in %+v", source, target, observed, edges)
}

func diagramRefs(nodes []ImpactElement) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.Ref)
	}
	return out
}

func hasDiagramEdge(edges []ImpactEdge, ref string) bool {
	for _, edge := range edges {
		if edge.SourceRef == ref || edge.TargetRef == ref {
			return true
		}
	}
	return false
}

func reverseElements(elements []ImpactElement) []ImpactElement {
	out := make([]ImpactElement, len(elements))
	for i, element := range elements {
		out[len(elements)-1-i] = element
	}
	return out
}

func reverseEdges(edges []ImpactEdge) []ImpactEdge {
	out := make([]ImpactEdge, len(edges))
	for i, edge := range edges {
		out[len(edges)-1-i] = edge
	}
	return out
}
