package watch

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RenderImpactJSON writes the report as machine-readable JSON.
func RenderImpactJSON(w io.Writer, report ImpactReport) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// RenderImpactText writes the default human-readable report.
func RenderImpactText(w io.Writer, report ImpactReport) error {
	if line := coverageSummaryLine(report.Coverage); line != "" {
		writeImpactSection(w, "Coverage", []string{"  " + line})
	}
	writeImpactSection(w, "Changed", elementLines(report.Changed))
	writeImpactSection(w, "Candidates", elementLines(report.Candidates))
	writeImpactSection(w, "Related", elementLines(report.Related))
	if len(report.Unmapped) > 0 {
		writeImpactSection(w, "Unmapped", prefixLines(report.Unmapped, "  "))
	}
	if len(report.Coverage.Gaps) > 0 {
		writeImpactSection(w, "Binding Gaps", coverageGapLines(report.Coverage.Gaps, "  "))
	}
	return nil
}

// RenderImpactMarkdown writes a PR-comment friendly report with a Mermaid graph.
func RenderImpactMarkdown(w io.Writer, report ImpactReport) error {
	return RenderImpactMarkdownStyle(w, report, DiagramStyleReview)
}

// RenderImpactMarkdownStyle writes a PR-comment friendly report with the
// single reviewer Mermaid diagram. The style argument is ignored and kept
// only for compatibility.
func RenderImpactMarkdownStyle(w io.Writer, report ImpactReport, _ DiagramStyle) error {
	_, _ = fmt.Fprintln(w, "## Architecture Impact")
	_, _ = fmt.Fprintln(w)
	if line := coverageSummaryLine(report.Coverage); line != "" {
		_, _ = fmt.Fprintln(w, "**Coverage:** "+line)
		_, _ = fmt.Fprintln(w)
	}
	if len(report.Changed) == 0 && len(report.Candidates) == 0 && len(report.Unmapped) == 0 {
		_, _ = fmt.Fprintln(w, "No architecture-bound code changed.")
		return nil
	}
	if len(report.Candidates) > 0 {
		_, _ = fmt.Fprintln(w, "**Candidates** _(weak name matches)_")
		_, _ = fmt.Fprintln(w)
		for _, element := range report.Candidates {
			_, _ = fmt.Fprintf(w, "- %s", element.Name)
			if evidence := evidenceSummary(element.Evidence); evidence != "" {
				_, _ = fmt.Fprintf(w, " — %s", evidence)
			}
			_, _ = fmt.Fprintln(w)
		}
	}
	if len(report.Coverage.Gaps) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "**Binding gaps**")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "_Add or extend a binding so impact analysis covers these files, then re-run `tld impact`._")
		_, _ = fmt.Fprintln(w)
		for _, gap := range report.Coverage.Gaps {
			_, _ = fmt.Fprintf(w, "- `%s`", gap.File)
			if gap.Change != "" {
				_, _ = fmt.Fprintf(w, " (%s)", gap.Change)
			}
			_, _ = fmt.Fprintf(w, " — %s\n", gap.Reason)
			switch {
			case gap.SuggestedRef != "" && gap.CurrentPattern == "":
				_, _ = fmt.Fprintf(w, "  - suggested owner: %s (%.2f) — `tld bind %s --file %q`\n", gap.SuggestedName, gap.SuggestedScore, gap.SuggestedRef, gap.File)
			case gap.SuggestedRef != "":
				_, _ = fmt.Fprintf(w, "  - suggested owner: %s (%.2f) already owns `%s`; extend that binding if this file belongs to it\n", gap.SuggestedName, gap.SuggestedScore, gap.CurrentPattern)
			}
			if gap.NewElementName != "" {
				_, _ = fmt.Fprintf(w, "  - or create an element: `tld add %q --file %q`\n", gap.NewElementName, gap.File)
			}
		}
	}
	diagram := BuildImpactDiagram(report, DiagramStyleReview)
	if diagram.OmittedNodes > 0 || diagram.OmittedEdges > 0 {
		parts := []string{}
		if diagram.OmittedNodes > 0 {
			parts = append(parts, pluralizeDiagram(diagram.OmittedNodes, "node"))
		}
		if diagram.OmittedEdges > 0 {
			parts = append(parts, pluralizeDiagram(diagram.OmittedEdges, "edge"))
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "_Graph trimmed for readability: +%s omitted._\n", strings.Join(parts, ", +"))
	}
	if hasObservedEdges(report.Edges) {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "_Dashed edges are observed in code but not declared in the architecture._")
	}
	if hasContainmentEdge(report.Edges) {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "_–o edges show folder containment between bound elements._")
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "```mermaid")
	if err := RenderImpactMermaidStyle(w, report, DiagramStyleReview); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "```")
	return nil
}

// RenderImpactMermaid writes the impacted architecture as a Mermaid flowchart.
func RenderImpactMermaid(w io.Writer, report ImpactReport) error {
	return RenderImpactMermaidStyle(w, report, DiagramStyleReview)
}

// RenderImpactMermaidStyle writes the impacted architecture as the single
// reviewer Mermaid flowchart. The style argument is ignored and kept only
// for compatibility.
func RenderImpactMermaidStyle(w io.Writer, report ImpactReport, style DiagramStyle) error {
	code := BuildImpactDiagram(report, style).Code
	if strings.TrimSpace(code) == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, code)
	return err
}

func writeImpactSection(w io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", title)
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
	_, _ = fmt.Fprintln(w)
}

func elementLines(elements []ImpactElement) []string {
	lines := make([]string, 0, len(elements))
	for _, element := range elements {
		line := "  " + element.Name
		if evidence := evidenceSummary(element.Evidence); evidence != "" {
			line += " (" + evidence + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

func coverageSummaryLine(coverage Coverage) string {
	if !coverage.Applicable {
		return "No source code changed — nothing to reconcile."
	}
	status := "all changed source files are covered"
	switch {
	case !coverage.Complete:
		status = "analysis may be incomplete"
	case coverage.Confidence == "low":
		status = "coverage is limited to broad bindings"
	}
	return fmt.Sprintf("%d%% (%s) — %d/%d source files covered; %s",
		coverage.Percent, coverage.Confidence, coverage.BoundSourceFiles, coverage.SourceFiles, status)
}

func coverageGapLines(gaps []CoverageGap, prefix string) []string {
	lines := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		line := prefix + gap.File
		if gap.Change != "" {
			line += " (" + gap.Change + ")"
		}
		line += " — " + gap.Reason
		switch {
		case gap.SuggestedRef != "" && gap.CurrentPattern == "":
			line += fmt.Sprintf("; suggested owner %q (%.2f): tld bind %s --file %q", gap.SuggestedName, gap.SuggestedScore, gap.SuggestedRef, gap.File)
		case gap.SuggestedRef != "":
			line += fmt.Sprintf("; suggested owner %q (%.2f) already owns %q", gap.SuggestedName, gap.SuggestedScore, gap.CurrentPattern)
		}
		if gap.NewElementName != "" {
			line += fmt.Sprintf("; or add: tld add %q --file %q", gap.NewElementName, gap.File)
		}
		lines = append(lines, line)
	}
	return lines
}

func hasObservedEdges(edges []ImpactEdge) bool {
	for _, edge := range edges {
		if edge.Observed {
			return true
		}
	}
	return false
}

func hasContainmentEdge(edges []ImpactEdge) bool {
	for _, edge := range edges {
		if isContainmentEdge(edge) {
			return true
		}
	}
	return false
}

func evidenceSummary(evidence []ImpactEvidence) string {
	if len(evidence) == 0 {
		return ""
	}
	parts := make([]string, 0, len(evidence))
	seen := map[string]struct{}{}
	for _, item := range evidence {
		value := strings.TrimSpace(item.Detail)
		if value == "" {
			value = strings.TrimSpace(item.Path)
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		parts = append(parts, value)
		if len(parts) >= 3 {
			break
		}
	}
	return strings.Join(parts, ", ")
}

func prefixLines(values []string, prefix string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, prefix+value)
	}
	return out
}

func escapeMermaidLabel(value string) string {
	value = strings.ReplaceAll(value, `"`, "'")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
