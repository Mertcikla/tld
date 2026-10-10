package mermaid

import (
	"strings"
	"testing"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func ptrUint32(value uint32) *uint32 { return &value }

func impactFixture() *codeindexv1.ImpactDiagram {
	return &codeindexv1.ImpactDiagram{
		RepositoryId:  "repo-1",
		ComparisonKey: "key-1",
		MaxRadius:     3,
		Diff: &codeindexv1.SnapshotDiff{
			Sources: []*codeindexv1.SourceChange{
				{Path: "src/api.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED, LinesAdded: ptrUint32(4), LinesRemoved: ptrUint32(1)},
				{Path: "src/new.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_ADDED, LinesAdded: ptrUint32(7), LinesRemoved: ptrUint32(0)},
			},
		},
		Nodes: []*codeindexv1.ImpactNode{
			{Key: "file|src/api.go", Path: "src/api.go", Name: "api.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED},
			{Key: "file|src/new.go", Path: "src/new.go", Name: "new.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_ADDED},
			{Key: "context|42", Path: "src/db.go", Name: "db.go", Distance: 1, ElementId: 42},
		},
		Edges: []*codeindexv1.ImpactEdge{
			{FromKey: "file|src/api.go", ToKey: "file|src/new.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_ADDED, Weight: 2},
			{FromKey: "file|src/new.go", ToKey: "context|42", Change: codeindexv1.ChangeKind_CHANGE_KIND_UNSPECIFIED, Weight: 1},
		},
	}
}

func TestExportImpactDiagram(t *testing.T) {
	t.Parallel()

	got := ExportImpactDiagram(impactFixture(), ImpactExportOptions{IncludeMetadata: true, Radius: 2})
	for _, want := range []string{
		"flowchart LR",
		"%% tld-impact repo=repo-1 key=key-1 radius=2",
		"node_file_src_api_go[\"api.go<br/>+4 \u22121\"]",
		"node_file_src_new_go[\"new.go<br/>+7 \u22120\"]",
		`node_context_42["db.go<br/>(context)"]`,
		`node_file_src_api_go ==>|"+2 dependencies"| node_file_src_new_go`,
		`node_file_src_new_go -- "1 dependency" --> node_context_42`,
		"classDef modified",
		"classDef added",
		"classDef context",
		"class node_file_src_api_go modified",
		"class node_file_src_new_go added",
		"class node_context_42 context",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactDiagram() missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "classDef removed") {
		t.Fatalf("ExportImpactDiagram() should not emit unused classes:\n%s", got)
	}
}

func TestExportImpactDiagramWithoutMetadataSkipsComment(t *testing.T) {
	t.Parallel()

	got := ExportImpactDiagram(impactFixture(), ImpactExportOptions{})
	if strings.Contains(got, "%% tld-impact") {
		t.Fatalf("ExportImpactDiagram() unexpected metadata comment:\n%s", got)
	}
}

func TestExportImpactDiagramEscapesLabels(t *testing.T) {
	t.Parallel()

	diagram := &codeindexv1.ImpactDiagram{
		Nodes: []*codeindexv1.ImpactNode{
			{Key: `file|a"b`, Path: "a\"b.go", Name: `a "quoted" & name`},
		},
	}
	got := ExportImpactDiagram(diagram, ImpactExportOptions{})
	if !strings.Contains(got, `node_file_a_b["a &quot;quoted&quot; &amp; name"]`) {
		t.Fatalf("ExportImpactDiagram() did not escape label:\n%s", got)
	}
}

func TestExportImpactDiagramNilIsEmptyFlowchart(t *testing.T) {
	t.Parallel()

	if got := ExportImpactDiagram(nil, ImpactExportOptions{}); got != "flowchart LR\n" {
		t.Fatalf("ExportImpactDiagram(nil) = %q", got)
	}
}

func TestExportImpactDiagramMarksEdgeChurn(t *testing.T) {
	t.Parallel()

	diagram := &codeindexv1.ImpactDiagram{
		Nodes: []*codeindexv1.ImpactNode{
			{Key: "file|a.go", Path: "a.go", Name: "a.go"},
			{Key: "file|b.go", Path: "b.go", Name: "b.go"},
			{Key: "file|c.go", Path: "c.go", Name: "c.go"},
			{Key: "file|d.go", Path: "d.go", Name: "d.go"},
		},
		Edges: []*codeindexv1.ImpactEdge{
			{FromKey: "file|a.go", ToKey: "file|b.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_REMOVED, Weight: 2},
			{FromKey: "file|c.go", ToKey: "file|d.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED, Weight: 3},
		},
	}
	got := ExportImpactDiagram(diagram, ImpactExportOptions{})
	for _, want := range []string{
		`node_file_a_go--x|"-2 dependencies"|node_file_b_go`,
		`node_file_c_go -- "~3 dependencies" --> node_file_d_go`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactDiagram() missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactDiagramIgnoresDanglingEdges(t *testing.T) {
	t.Parallel()

	diagram := &codeindexv1.ImpactDiagram{
		Nodes: []*codeindexv1.ImpactNode{{Key: "file|a", Name: "a"}},
		Edges: []*codeindexv1.ImpactEdge{{FromKey: "file|a", ToKey: "missing", Weight: 1}},
	}
	got := ExportImpactDiagram(diagram, ImpactExportOptions{})
	if strings.Contains(got, "-->") {
		t.Fatalf("ExportImpactDiagram() rendered a dangling edge:\n%s", got)
	}
}

func TestExportImpactDiagramCustomColors(t *testing.T) {
	t.Parallel()

	diagram := &codeindexv1.ImpactDiagram{
		Nodes: []*codeindexv1.ImpactNode{{Key: "file|a", Name: "a", Change: codeindexv1.ChangeKind_CHANGE_KIND_REMOVED}},
	}
	got := ExportImpactDiagram(diagram, ImpactExportOptions{Colors: ImpactExportColors{Removed: "#ff0000"}})
	if !strings.Contains(got, "stroke:#ff0000") {
		t.Fatalf("ExportImpactDiagram() ignored custom color:\n%s", got)
	}
}

func groupedImpactFixture() *codeindexv1.ImpactDiagram {
	return &codeindexv1.ImpactDiagram{
		Nodes: []*codeindexv1.ImpactNode{
			{Key: "file|pkg/a.go", Path: "pkg/a.go", Name: "a.go"},
			{Key: "file|pkg/b.go", Path: "pkg/b.go", Name: "b.go"},
			{Key: "file|lib/c.go", Path: "lib/c.go", Name: "c.go"},
			{Key: "file|other/d.go", Path: "other/d.go", Name: "d.go"},
		},
		Edges: []*codeindexv1.ImpactEdge{
			{FromKey: "file|pkg/a.go", ToKey: "file|pkg/b.go", Weight: 1},
			{FromKey: "file|pkg/a.go", ToKey: "file|lib/c.go", Weight: 2},
			{FromKey: "file|lib/c.go", ToKey: "file|other/d.go", Weight: 3},
		},
		Groups: []*codeindexv1.ImpactGroup{
			{
				Key: "view:1", Name: "Repo", Source: "view", ViewId: 1,
				Children: []*codeindexv1.ImpactGroup{
					{Key: "view:2", Name: "pkg", Source: "view", ViewId: 2, NodeKeys: []string{"file|pkg/a.go", "file|pkg/b.go"}},
					{Key: "view:3", Name: "lib", Source: "view", ViewId: 3, NodeKeys: []string{"file|lib/c.go"}},
				},
			},
			{Key: "view:4", Name: "other", Source: "view", ViewId: 4, NodeKeys: []string{"file|other/d.go"}},
		},
	}
}

func TestExportImpactDiagramNestsSubgraphs(t *testing.T) {
	t.Parallel()

	got := ExportImpactDiagram(groupedImpactFixture(), ImpactExportOptions{})
	for _, want := range []string{
		`  subgraph group_view_1["Repo"]`,
		`    subgraph group_view_2["pkg"]`,
		`      node_file_pkg_a_go["a.go"]`,
		`      node_file_pkg_b_go["b.go"]`,
		`    end`,
		`    subgraph group_view_3["lib"]`,
		`      node_file_lib_c_go["c.go"]`,
		`    end`,
		`  end`,
		`  subgraph group_view_4["other"]`,
		`    node_file_other_d_go["d.go"]`,
		`  end`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactDiagram() missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactDiagramNestsEdgesAtDeepestGroup(t *testing.T) {
	t.Parallel()

	got := ExportImpactDiagram(groupedImpactFixture(), ImpactExportOptions{})
	for _, want := range []string{
		// Siblings inside pkg share the deepest group.
		`      node_file_pkg_a_go -- "1 dependency" --> node_file_pkg_b_go`,
		// pkg and lib diverge, so the edge lands in their parent Repo.
		`    node_file_pkg_a_go -- "2 dependencies" --> node_file_lib_c_go`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactDiagram() missing %q in:\n%s", want, got)
		}
	}
	// lib and other are separate roots, so the edge is emitted at the top level.
	if !strings.Contains(got, "\n  node_file_lib_c_go -- \"3 dependencies\" --> node_file_other_d_go\n") {
		t.Fatalf("ExportImpactDiagram() did not place cross-root edge at top level:\n%s", got)
	}
}
