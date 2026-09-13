package watch

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestBindingPatternMatch(t *testing.T) {
	cases := []struct {
		pattern string
		file    string
		want    bool
	}{
		{"backend/checkout/service.go", "backend/checkout/service.go", true},
		{"backend/checkout/service.go", "backend/checkout/other.go", false},
		{"backend/checkout/**", "backend/checkout/service.go", true},
		{"backend/checkout/**", "backend/checkout", true},
		{"backend/checkout/**", "backend/other/service.go", false},
		{"backend/checkout/", "backend/checkout/service.go", true},
		{"**/service.go", "backend/checkout/service.go", true},
		{"internal/*/service.go", "internal/checkout/service.go", true},
		{"internal/*/service.go", "internal/checkout/nested/service.go", false},
	}
	for _, tc := range cases {
		if got := bindingPatternMatch(tc.pattern, tc.file); got != tc.want {
			t.Errorf("bindingPatternMatch(%q, %q) = %v, want %v", tc.pattern, tc.file, got, tc.want)
		}
	}
}

func TestNormalizeRepoIdentity(t *testing.T) {
	cases := map[string]string{
		"git@github.com:org/checkout.git": "checkout",
		"https://github.com/org/checkout": "checkout",
		"checkout":                        "checkout",
		"":                                "",
	}
	for input, want := range cases {
		if got := normalizeRepoIdentity(input); got != want {
			t.Errorf("normalizeRepoIdentity(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAnalyzeImpactStrongBindingsAndUnmapped(t *testing.T) {
	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout Service", Kind: "service", FilePath: "backend/checkout/**"},
		"web":      {Name: "Web App", Kind: "service"},
	}
	report := AnalyzeImpact(ImpactOptions{
		Base:     "main",
		RepoRoot: t.TempDir(),
		Elements: elements,
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
			"internal/risk/model.go":      tldgit.WorktreeAdded,
		},
	})

	if len(report.Changed) != 1 || report.Changed[0].Ref != "checkout" {
		t.Fatalf("changed = %+v, want checkout", report.Changed)
	}
	if len(report.Candidates) != 0 {
		t.Fatalf("candidates = %+v, want none", report.Candidates)
	}
	if len(report.Unmapped) != 1 || report.Unmapped[0] != "internal/risk/model.go" {
		t.Fatalf("unmapped = %+v, want internal/risk/model.go", report.Unmapped)
	}
	if report.Changed[0].Evidence[0].Level != EvidenceStrong {
		t.Fatalf("evidence level = %q, want strong", report.Changed[0].Evidence[0].Level)
	}
	if !hasFinding(report.Findings, "unmapped_code") {
		t.Fatalf("missing unmapped_code finding: %+v", report.Findings)
	}
}

func TestAnalyzeImpactNameHeuristicsProduceCandidates(t *testing.T) {
	elements := map[string]*workspace.Element{
		"recorder": {Name: "Version Recorder", Kind: "component"},
	}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:              t.TempDir(),
		Elements:              elements,
		IncludeNameHeuristics: true,
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"internal/watch/version_recorder.go": tldgit.WorktreeUpdated,
		},
	})

	if len(report.Changed) != 0 {
		t.Fatalf("changed = %+v, want none", report.Changed)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Ref != "recorder" {
		t.Fatalf("candidates = %+v, want recorder", report.Candidates)
	}
	if report.Candidates[0].Evidence[0].Observed {
		t.Fatalf("name match should be inferred, got observed")
	}
	if !hasFinding(report.Findings, "candidate_element") {
		t.Fatalf("missing candidate_element finding: %+v", report.Findings)
	}
}

func TestAnalyzeImpactRelatedAndEdges(t *testing.T) {
	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout Service", FilePath: "backend/checkout/**"},
		"payment":  {Name: "Payment Service"},
		"orders":   {Name: "Orders DB"},
	}
	connectors := map[string]*workspace.Connector{
		"c1": {View: "architecture", Source: "checkout", Target: "payment", Label: "calls"},
		"c2": {View: "architecture", Source: "checkout", Target: "orders", Label: "reads"},
		"c3": {View: "architecture", Source: "payment", Target: "orders", Label: "unrelated"},
	}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:     t.TempDir(),
		Elements:     elements,
		Connectors:   connectors,
		ChangedFiles: map[string]tldgit.WorktreeChange{"backend/checkout/service.go": tldgit.WorktreeUpdated},
	})

	if len(report.Related) != 2 {
		t.Fatalf("related = %+v, want payment and orders", report.Related)
	}
	if len(report.Edges) != 2 {
		t.Fatalf("edges = %+v, want 2", report.Edges)
	}
}

func TestAnalyzeImpactRelationshipFindings(t *testing.T) {
	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
		"fraud":    {Name: "Fraud", FilePath: "backend/fraud/**"},
	}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:     t.TempDir(),
		Elements:     elements,
		ChangedFiles: map[string]tldgit.WorktreeChange{"backend/checkout/service.go": tldgit.WorktreeUpdated},
		Relationships: []RelationshipEvidence{{
			SourceRef: "checkout",
			TargetRef: "fraud",
			File:      "backend/checkout/service.go",
			Line:      81,
			Kind:      "call",
			Level:     EvidenceStrong,
			Observed:  true,
		}},
	})
	if !hasFinding(report.Findings, "possible_new_relationship") {
		t.Fatalf("missing possible_new_relationship: %+v", report.Findings)
	}
	if !hasObservedEdge(report.Edges, "checkout", "fraud") {
		t.Fatalf("expected observed checkout->fraud edge, got %+v", report.Edges)
	}

	withConnector := AnalyzeImpact(ImpactOptions{
		RepoRoot:      t.TempDir(),
		Elements:      elements,
		ChangedFiles:  map[string]tldgit.WorktreeChange{"backend/checkout/service.go": tldgit.WorktreeUpdated},
		Connectors:    map[string]*workspace.Connector{"c": {Source: "checkout", Target: "fraud", Label: "calls"}},
		Relationships: []RelationshipEvidence{{SourceRef: "checkout", TargetRef: "fraud", File: "backend/checkout/service.go", Line: 81, Level: EvidenceStrong, Observed: true}},
	})
	if hasFinding(withConnector.Findings, "possible_new_relationship") {
		t.Fatalf("existing connector should suppress possible_new_relationship: %+v", withConnector.Findings)
	}
	if !hasFinding(withConnector.Findings, "observed_relationship") {
		t.Fatalf("missing observed_relationship: %+v", withConnector.Findings)
	}
	if hasObservedEdge(withConnector.Edges, "checkout", "fraud") {
		t.Fatalf("declared connector should not add an observed edge: %+v", withConnector.Edges)
	}
}

func TestAnalyzeImpactObservedRelationshipAddsNode(t *testing.T) {
	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
		"fraud":    {Name: "Fraud", FilePath: "backend/fraud/**"},
	}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:     t.TempDir(),
		Elements:     elements,
		ChangedFiles: map[string]tldgit.WorktreeChange{"backend/checkout/service.go": tldgit.WorktreeUpdated},
		Relationships: []RelationshipEvidence{{
			SourceRef: "checkout",
			TargetRef: "fraud",
			File:      "backend/checkout/service.go",
			Line:      81,
			Kind:      "call",
			Level:     EvidenceStrong,
			Observed:  true,
		}},
	})
	// Fraud is not changed and not connected by an authored connector, so it
	// must still appear as a related node for the diagram.
	if !hasElement(report.Related, "fraud") {
		t.Fatalf("expected fraud in related nodes, got %+v", report.Related)
	}
}

func TestAnalyzeImpactSuggestionFindings(t *testing.T) {
	elements := map[string]*workspace.Element{"payments": {Name: "Payments"}}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:     t.TempDir(),
		Elements:     elements,
		ChangedFiles: map[string]tldgit.WorktreeChange{"internal/risk/refund_worker.go": tldgit.WorktreeAdded},
		Suggestions:  []BindingSuggestion{{File: "internal/risk/refund_worker.go", ElementRef: "payments", ElementName: "Payments", Score: 0.91}},
	})
	if !hasFinding(report.Findings, "unmapped_code_suggestion") {
		t.Fatalf("missing unmapped_code_suggestion: %+v", report.Findings)
	}
	for _, finding := range report.Findings {
		if finding.Type == "unmapped_code_suggestion" && finding.Observed {
			t.Fatalf("suggestion must be inferred, got observed")
		}
	}
}

func TestAnalyzeImpactStaleBinding(t *testing.T) {
	elements := map[string]*workspace.Element{"ghost": {Name: "Ghost", FilePath: "does/not/exist.go"}}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: elements,
	})
	if !hasFinding(report.Findings, "stale_binding") {
		t.Fatalf("missing stale_binding: %+v", report.Findings)
	}
}

func TestRenderImpactMermaidAndMarkdown(t *testing.T) {
	report := ImpactReport{
		Changed:  []ImpactElement{{Ref: "checkout", Name: "Checkout", Evidence: []ImpactEvidence{{Level: EvidenceStrong, Kind: "path", Path: "backend/checkout/service.go", Observed: true}}}},
		Related:  []ImpactElement{{Ref: "payment", Name: "Payment"}},
		Edges:    []ImpactEdge{{SourceRef: "checkout", TargetRef: "payment", Label: "calls", Observed: true}},
		Unmapped: []string{"internal/risk/model.go"},
		Findings: []ImpactFinding{{Type: "unmapped_code", Severity: "warning", Message: "internal/risk/model.go changed but no architecture element is bound to it", Observed: true}},
	}

	var mermaid bytes.Buffer
	if err := RenderImpactMermaid(&mermaid, report); err != nil {
		t.Fatal(err)
	}
	out := mermaid.String()
	if !strings.Contains(out, "flowchart TD") || !strings.Contains(out, "Checkout") || !strings.Contains(out, "Payment") {
		t.Fatalf("unexpected mermaid:\n%s", out)
	}

	var markdown bytes.Buffer
	if err := RenderImpactMarkdown(&markdown, report); err != nil {
		t.Fatal(err)
	}
	md := markdown.String()
	for _, want := range []string{"## Architecture Impact", "**Changed**", "**Unmapped**", "```mermaid", "internal/risk/model.go"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}

	var text bytes.Buffer
	if err := RenderImpactText(&text, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Changed") || !strings.Contains(text.String(), "Findings") {
		t.Fatalf("unexpected text:\n%s", text.String())
	}
}

func TestNarrateImpactReportDeterministicFallback(t *testing.T) {
	report := ImpactReport{
		Changed:  []ImpactElement{{Ref: "checkout", Name: "Checkout"}},
		Unmapped: []string{"internal/risk/model.go"},
	}
	out, err := NarrateImpactReport(context.Background(), NarrateOptions{Report: report})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Checkout") {
		t.Fatalf("narration missing changed element: %q", out)
	}
	if !strings.Contains(out, "1 changed file") {
		t.Fatalf("narration missing unmapped count: %q", out)
	}
}

func hasFinding(findings []ImpactFinding, findingType string) bool {
	for _, finding := range findings {
		if finding.Type == findingType {
			return true
		}
	}
	return false
}

func hasObservedEdge(edges []ImpactEdge, source, target string) bool {
	for _, edge := range edges {
		if edge.Observed && edge.SourceRef == source && edge.TargetRef == target {
			return true
		}
	}
	return false
}

func hasElement(elements []ImpactElement, ref string) bool {
	for _, element := range elements {
		if element.Ref == ref {
			return true
		}
	}
	return false
}
