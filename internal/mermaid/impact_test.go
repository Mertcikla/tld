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
		Radius:        2,
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
			{Key: "context|42", Path: "src/db.go", Name: "db.go", Context: true, ElementId: 42},
		},
		Edges: []*codeindexv1.ImpactEdge{
			{FromKey: "file|src/api.go", ToKey: "file|src/new.go", Change: codeindexv1.ChangeKind_CHANGE_KIND_ADDED, Weight: 2},
			{FromKey: "file|src/new.go", ToKey: "context|42", Change: codeindexv1.ChangeKind_CHANGE_KIND_UNSPECIFIED, Weight: 1},
		},
	}
}

func TestExportImpactDiagram(t *testing.T) {
	t.Parallel()

	got := ExportImpactDiagram(impactFixture(), ImpactExportOptions{IncludeMetadata: true})
	for _, want := range []string{
		"flowchart LR",
		"%% tld-impact repo=repo-1 key=key-1 radius=2",
		"node_file_src_api_go[\"api.go<br/>+4 \u22121\"]",
		"node_file_src_new_go[\"new.go<br/>+7 \u22120\"]",
		`node_context_42["db.go<br/>(context)"]`,
		`node_file_src_api_go -- "2 dependencies" --> node_file_src_new_go`,
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
