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

func TestBuildImpactDiagramFullMatchesRenderer(t *testing.T) {
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
	if diagram.Style != DiagramStyleFull {
		t.Fatalf("style = %q, want full", diagram.Style)
	}
	if diagram.OmittedNodes != 0 || diagram.OmittedEdges != 0 {
		t.Fatalf("full style omitted context: %+v", diagram)
	}
	for _, want := range []string{"%% coverage: 80% (medium)", "flowchart TD", `n1["Checkout"]`, `n2["Weak match"]`, `n3["Payment"]`, "n1 -.->|calls| n3", "class n1 changed"} {
		if !strings.Contains(diagram.Code, want) {
			t.Fatalf("full diagram missing %q:\n%s", want, diagram.Code)
		}
	}

	var rendered bytes.Buffer
	if err := RenderImpactMermaidStyle(&rendered, report, DiagramStyleFull); err != nil {
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
	for _, style := range []DiagramStyle{DiagramStyleReview, DiagramStyleFull, DiagramStyleBounded, DiagramStyleLanes, DiagramStyleGroups} {
		diagram := BuildImpactDiagram(report, style)
		refs := map[string]struct{}{}
		for _, node := range diagram.Nodes {
			refs[node.Ref] = struct{}{}
		}
		for _, edge := range diagram.Edges {
			if _, ok := refs[edge.SourceRef]; !ok {
				t.Fatalf("%s: edge source %q is not drawn", style, edge.SourceRef)
			}
			if _, ok := refs[edge.TargetRef]; !ok {
				t.Fatalf("%s: edge target %q is not drawn", style, edge.TargetRef)
			}
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
	for _, style := range []DiagramStyle{DiagramStyleReview, DiagramStyleBounded, DiagramStyleLanes, DiagramStyleGroups} {
		want := BuildImpactDiagram(report, style)
		got := BuildImpactDiagram(shuffled, style)
		if want.Code != got.Code {
			t.Fatalf("%s: output depends on input order:\n%s\n---\n%s", style, want.Code, got.Code)
		}
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
	diagram := BuildImpactDiagram(report, DiagramStyleBounded)
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
	for _, style := range []DiagramStyle{DiagramStyleBounded, DiagramStyleLanes} {
		diagram := BuildImpactDiagram(report, style)
		if !strings.Contains(diagram.Code, "Formatter") {
			t.Fatalf("%s: changed node missing:\n%s", style, diagram.Code)
		}
		if strings.Contains(diagram.Code, "Plugin registry") {
			t.Fatalf("%s: disconnected related node should be dropped:\n%s", style, diagram.Code)
		}
	}
	grouped := BuildImpactDiagram(report, DiagramStyleGroups)
	if !strings.Contains(grouped.Code, "Formatter") {
		t.Fatalf("groups: changed node missing:\n%s", grouped.Code)
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

func TestBuildImpactDiagramLanesMatchBoundedSelection(t *testing.T) {
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
	lanes := BuildImpactDiagram(report, DiagramStyleLanes)
	bounded := BuildImpactDiagram(report, DiagramStyleBounded)
	if len(lanes.Edges) != len(bounded.Edges) {
		t.Fatalf("lanes edges = %d, bounded edges = %d", len(lanes.Edges), len(bounded.Edges))
	}
	if !strings.HasPrefix(lanes.Code, "flowchart LR") {
		t.Fatalf("lanes diagram should be left-to-right:\n%s", lanes.Code)
	}
	for _, lane := range []string{"lane_incoming", "lane_changed", "lane_outgoing", "lane_both"} {
		if !strings.Contains(lanes.Code, "subgraph "+lane) {
			t.Fatalf("lanes diagram missing %s:\n%s", lane, lanes.Code)
		}
	}
}

func TestBuildImpactDiagramGroupsByOwner(t *testing.T) {
	report := ImpactReport{
		Changed: []ImpactElement{{Ref: "a", Name: "A", Owner: "Team"}},
		Related: []ImpactElement{
			{Ref: "b", Name: "B", Owner: "Team"},
			{Ref: "external", Name: "External"},
			{Ref: "unowned", Name: "Unowned"},
		},
		Edges: []ImpactEdge{
			diagramEdge("a", "b", false),
			diagramEdge("a", "external", false),
			diagramEdge("b", "external", false),
			diagramEdge("a", "external", true),
			diagramEdge("external", "a", false),
		},
	}
	diagram := BuildImpactDiagram(report, DiagramStyleGroups)
	if diagram.InternalEdges != 1 {
		t.Fatalf("internalEdges = %d, want 1", diagram.InternalEdges)
	}
	if len(diagram.Groups) != 3 {
		t.Fatalf("groups = %+v, want 3 buckets", diagram.Groups)
	}
	assertGroup(t, diagram.Groups, "Team", 1, []string{"A", "B"})
	if len(diagram.Nodes) != 3 {
		t.Fatalf("nodes = %+v, want owner bucket + 2 ungrouped", diagram.Nodes)
	}
	if node := findDiagramNode(diagram.Nodes, "owner:Team"); node == nil || node.Name != "Team (1/2 touched)" {
		t.Fatalf("owner node = %+v, want labelled touched count", node)
	}
	if findDiagramNode(diagram.Nodes, "element:unowned") == nil {
		t.Fatalf("unowned element missing: %+v", diagram.Nodes)
	}
	assertEdgeLabel(t, diagram.Edges, "owner:Team", "element:external", false, "2 declared")
	assertEdgeLabel(t, diagram.Edges, "owner:Team", "element:external", true, "1 observed")
	assertEdgeLabel(t, diagram.Edges, "element:external", "owner:Team", false, "1 declared")
}

func TestBuildImpactDiagramHandlesEmptyAndDangling(t *testing.T) {
	report := ImpactReport{Edges: []ImpactEdge{diagramEdge("missing", "also-missing", false)}}
	for _, style := range []DiagramStyle{DiagramStyleReview, DiagramStyleFull, DiagramStyleBounded, DiagramStyleLanes, DiagramStyleGroups} {
		diagram := BuildImpactDiagram(report, style)
		if len(diagram.Nodes) != 0 || len(diagram.Edges) != 0 {
			t.Fatalf("%s: expected empty diagram, got %+v", style, diagram)
		}
		if strings.Contains(strings.ToLower(diagram.Code), "undefined") {
			t.Fatalf("%s: diagram contains undefined ids:\n%s", style, diagram.Code)
		}
	}
}

func TestNormalizeDiagramStyle(t *testing.T) {
	cases := map[string]DiagramStyle{
		"":             DiagramStyleReview,
		"review":       DiagramStyleReview,
		"REVIEW":       DiagramStyleReview,
		"full":         DiagramStyleFull,
		"all":          DiagramStyleFull,
		"BOUNDED":      DiagramStyleBounded,
		"neighborhood": DiagramStyleBounded,
		"lanes":        DiagramStyleLanes,
		"lane":         DiagramStyleLanes,
		"groups":       DiagramStyleGroups,
		"grouped":      DiagramStyleGroups,
		" nope ":       DiagramStyleReview,
	}
	for input, want := range cases {
		if got := NormalizeDiagramStyle(input); got != want {
			t.Fatalf("NormalizeDiagramStyle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildImpactDiagramReviewAnnotatesChangedSources(t *testing.T) {
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
		`n1["Core<br/>internal/core.go (+12 -3)<br/>internal/util.go"]`,
		`n2["API"]`,
		"n2 -.->|observed| n1",
	} {
		if !strings.Contains(diagram.Code, want) {
			t.Fatalf("review diagram missing %q:\n%s", want, diagram.Code)
		}
	}
	if strings.Contains(diagram.Code, `n2["API<br/>`) {
		t.Fatalf("context node should not be annotated:\n%s", diagram.Code)
	}
	if got := BuildImpactDiagram(report, DiagramStyleReview); got.Code != diagram.Code {
		t.Fatalf("review diagram is not deterministic")
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

func assertGroup(t *testing.T, groups []DiagramGroup, name string, touched int, members []string) {
	t.Helper()
	for _, group := range groups {
		if group.Name != name {
			continue
		}
		if group.Touched != touched || strings.Join(group.Members, ",") != strings.Join(members, ",") {
			t.Fatalf("group %q = %+v, want touched %d members %v", name, group, touched, members)
		}
		return
	}
	t.Fatalf("group %q not found in %+v", name, groups)
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

func findDiagramNode(nodes []ImpactElement, ref string) *ImpactElement {
	for index := range nodes {
		if nodes[index].Ref == ref {
			return &nodes[index]
		}
	}
	return nil
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
