package watch

import (
	"bytes"
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
}

func TestAnalyzeImpactChangedFilesLineStats(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		Base:     "main",
		RepoRoot: t.TempDir(),
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"internal/risk/model.go":  tldgit.WorktreeAdded,
			"backend/checkout/svc.go": tldgit.WorktreeUpdated,
		},
		LineStats: map[string]tldgit.LineDiff{
			"internal/risk/model.go":  {Added: 12, Removed: 0},
			"backend/checkout/svc.go": {Added: 3, Removed: 5},
		},
	})

	want := []ChangedFile{
		{Path: "backend/checkout/svc.go", Change: "updated", Added: 3, Removed: 5},
		{Path: "internal/risk/model.go", Change: "added", Added: 12, Removed: 0},
	}
	if len(report.ChangedFiles) != len(want) {
		t.Fatalf("changed files = %+v, want %+v", report.ChangedFiles, want)
	}
	for i, file := range report.ChangedFiles {
		if file != want[i] {
			t.Errorf("changed file[%d] = %+v, want %+v", i, file, want[i])
		}
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

func TestAnalyzeImpactObservedRelationships(t *testing.T) {
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

func TestRenderImpactMermaidAndMarkdown(t *testing.T) {
	report := ImpactReport{
		Changed:  []ImpactElement{{Ref: "checkout", Name: "Checkout", Evidence: []ImpactEvidence{{Level: EvidenceStrong, Kind: "path", Path: "backend/checkout/service.go", Observed: true}}}},
		Related:  []ImpactElement{{Ref: "payment", Name: "Payment"}},
		Edges:    []ImpactEdge{{SourceRef: "checkout", TargetRef: "payment", Label: "calls", Observed: true}},
		Unmapped: []string{"internal/risk/model.go"},
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
	if !strings.Contains(text.String(), "Changed") || strings.Contains(text.String(), "Findings") {
		t.Fatalf("unexpected text:\n%s", text.String())
	}
}

func TestAnalyzeImpactCoverageComplete(t *testing.T) {
	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
		"risk":     {Name: "Risk", FilePath: "internal/risk/**"},
	}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: elements,
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
			"internal/risk/model.go":      tldgit.WorktreeAdded,
		},
	})
	if !report.Coverage.Applicable || !report.Coverage.Complete {
		t.Fatalf("coverage = %+v, want applicable+complete", report.Coverage)
	}
	if report.Coverage.Percent != 100 || report.Coverage.Confidence != "high" {
		t.Fatalf("coverage = %+v, want 100/high", report.Coverage)
	}
	if len(report.Coverage.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", report.Coverage.Gaps)
	}
}

func TestAnalyzeImpactCoverageGapsIgnoreNonSource(t *testing.T) {
	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout", FilePath: "backend/checkout/**"},
	}
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: elements,
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"backend/checkout/service.go": tldgit.WorktreeUpdated,
			"internal/risk/model.go":      tldgit.WorktreeAdded,
			".github/workflows/ci.yml":    tldgit.WorktreeUpdated,
		},
		Suggestions: []BindingSuggestion{
			{File: "internal/risk/model.go", ElementRef: "checkout", ElementName: "Checkout", Score: 0.8},
		},
	})
	if report.Coverage.SourceFiles != 2 {
		t.Fatalf("source files = %d, want 2 (yaml ignored)", report.Coverage.SourceFiles)
	}
	if report.Coverage.NonSourceFiles != 1 {
		t.Fatalf("non-source files = %d, want 1", report.Coverage.NonSourceFiles)
	}
	if report.Coverage.UnmappedSource != 1 || report.Coverage.Complete {
		t.Fatalf("coverage = %+v, want 1 unmapped and incomplete", report.Coverage)
	}
	if len(report.Coverage.Gaps) != 1 {
		t.Fatalf("gaps = %+v, want 1", report.Coverage.Gaps)
	}
	gap := report.Coverage.Gaps[0]
	if gap.File != "internal/risk/model.go" || gap.Change != "added" {
		t.Fatalf("gap = %+v, want internal/risk/model.go added", gap)
	}
	if gap.SuggestedName != "Checkout" || gap.NewElementName != "Model" {
		t.Fatalf("gap suggestions = %+v, want Checkout/Model", gap)
	}
}

func TestAnalyzeImpactCoverageNonSourceOnly(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot: t.TempDir(),
		Elements: map[string]*workspace.Element{"a": {Name: "A", FilePath: "x/**"}},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			".github/workflows/ci.yml": tldgit.WorktreeUpdated,
		},
	})
	if report.Coverage.Applicable {
		t.Fatalf("coverage = %+v, want not applicable", report.Coverage)
	}
	if report.Coverage.Confidence != "none" {
		t.Fatalf("confidence = %q, want none", report.Coverage.Confidence)
	}
}

func TestAnalyzeImpactCoverageWeakNameMatch(t *testing.T) {
	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:              t.TempDir(),
		IncludeNameHeuristics: true,
		Elements:              map[string]*workspace.Element{"recorder": {Name: "Version Recorder"}},
		ChangedFiles: map[string]tldgit.WorktreeChange{
			"internal/watch/version_recorder.go": tldgit.WorktreeUpdated,
		},
	})
	if report.Coverage.WeakSourceFiles != 1 || report.Coverage.Complete {
		t.Fatalf("coverage = %+v, want 1 weak and incomplete", report.Coverage)
	}
	if len(report.Coverage.Gaps) != 1 || !strings.Contains(report.Coverage.Gaps[0].Reason, "weak") {
		t.Fatalf("gaps = %+v, want one weak-name gap", report.Coverage.Gaps)
	}
}

func TestRenderImpactMarkdownCoverageGaps(t *testing.T) {
	report := ImpactReport{
		Changed:  []ImpactElement{{Ref: "checkout", Name: "Checkout"}},
		Unmapped: []string{"internal/risk/model.go"},
		Coverage: Coverage{
			Applicable: true, Percent: 50, Confidence: "low", SourceFiles: 2, BoundSourceFiles: 1,
			Gaps: []CoverageGap{{
				File: "internal/risk/model.go", Change: "added", Reason: "no architecture element owns this file",
				NewElementName: "Model", NewElementRef: "model",
			}},
		},
	}
	var markdown bytes.Buffer
	if err := RenderImpactMarkdown(&markdown, report); err != nil {
		t.Fatal(err)
	}
	md := markdown.String()
	for _, want := range []string{"**Coverage:**", "analysis may be incomplete", "**Binding gaps**", "internal/risk/model.go", `tld add "Model" --file "internal/risk/model.go"`} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
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
