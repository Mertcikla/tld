package watch

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
	writeImpactSection(w, "Changed", elementLines(report.Changed))
	writeImpactSection(w, "Candidates", elementLines(report.Candidates))
	writeImpactSection(w, "Related", elementLines(report.Related))
	if len(report.Unmapped) > 0 {
		writeImpactSection(w, "Unmapped", prefixLines(report.Unmapped, "  "))
	}
	if len(report.Findings) > 0 {
		writeImpactSection(w, "Findings", findingLines(report.Findings))
	}
	if strings.TrimSpace(report.Narration) != "" {
		writeImpactSection(w, "Summary", []string{"  " + strings.TrimSpace(report.Narration)})
	}
	return nil
}

// RenderImpactMarkdown writes a PR-comment friendly report with a Mermaid graph.
func RenderImpactMarkdown(w io.Writer, report ImpactReport) error {
	_, _ = fmt.Fprintln(w, "## Architecture Impact")
	_, _ = fmt.Fprintln(w)
	if len(report.Changed) == 0 && len(report.Candidates) == 0 && len(report.Unmapped) == 0 {
		_, _ = fmt.Fprintln(w, "No architecture-bound code changed.")
		return nil
	}
	_, _ = fmt.Fprintln(w, "**Changed**")
	_, _ = fmt.Fprintln(w)
	if len(report.Changed) == 0 {
		_, _ = fmt.Fprintln(w, "- _(none)_")
	} else {
		for _, element := range report.Changed {
			_, _ = fmt.Fprintf(w, "- %s", element.Name)
			if evidence := evidenceSummary(element.Evidence); evidence != "" {
				_, _ = fmt.Fprintf(w, " — %s", evidence)
			}
			_, _ = fmt.Fprintln(w)
		}
	}
	if len(report.Candidates) > 0 {
		_, _ = fmt.Fprintln(w)
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
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "**Related**")
	_, _ = fmt.Fprintln(w)
	if len(report.Related) == 0 {
		_, _ = fmt.Fprintln(w, "- _(none)_")
	} else {
		for _, element := range report.Related {
			_, _ = fmt.Fprintf(w, "- %s\n", element.Name)
		}
	}
	if len(report.Unmapped) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "**Unmapped**")
		_, _ = fmt.Fprintln(w)
		for _, file := range report.Unmapped {
			_, _ = fmt.Fprintf(w, "- `%s`\n", file)
		}
	}
	if len(report.Findings) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "**Findings**")
		_, _ = fmt.Fprintln(w)
		for _, finding := range report.Findings {
			observed := "observed"
			if !finding.Observed {
				observed = "inferred"
			}
			_, _ = fmt.Fprintf(w, "- `%s` (%s, %s): %s\n", finding.Type, finding.Severity, observed, finding.Message)
		}
	}
	if hasObservedEdges(report.Edges) {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "_Dashed edges are observed in code but not declared in the architecture._")
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "```mermaid")
	if err := RenderImpactMermaid(w, report); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "```")
	if strings.TrimSpace(report.Narration) != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, strings.TrimSpace(report.Narration))
	}
	return nil
}

// RenderImpactMermaid writes the impacted architecture as a Mermaid flowchart.
func RenderImpactMermaid(w io.Writer, report ImpactReport) error {
	_, _ = fmt.Fprintln(w, "flowchart TD")
	ids := map[string]string{}
	index := 0
	idFor := func(ref, name string) string {
		if id, ok := ids[ref]; ok {
			return id
		}
		index++
		id := fmt.Sprintf("n%d", index)
		ids[ref] = id
		label := strings.TrimSpace(name)
		if label == "" {
			label = ref
		}
		_, _ = fmt.Fprintf(w, "  %s[\"%s\"]\n", id, escapeMermaidLabel(label))
		return id
	}

	changedIDs := map[string]struct{}{}
	for _, element := range report.Changed {
		id := idFor(element.Ref, element.Name)
		changedIDs[id] = struct{}{}
	}
	for _, element := range report.Candidates {
		idFor(element.Ref, element.Name)
	}
	for _, element := range report.Related {
		idFor(element.Ref, element.Name)
	}
	for _, edge := range report.Edges {
		sourceID, ok := ids[edge.SourceRef]
		if !ok {
			continue
		}
		targetID, ok := ids[edge.TargetRef]
		if !ok {
			continue
		}
		arrow := "-->"
		if edge.Observed {
			arrow = "-.->"
		}
		if strings.TrimSpace(edge.Label) != "" {
			_, _ = fmt.Fprintf(w, "  %s %s|%s| %s\n", sourceID, arrow, escapeMermaidLabel(edge.Label), targetID)
		} else {
			_, _ = fmt.Fprintf(w, "  %s %s %s\n", sourceID, arrow, targetID)
		}
	}
	if len(changedIDs) > 0 {
		sortedIDs := make([]string, 0, len(changedIDs))
		for id := range changedIDs {
			sortedIDs = append(sortedIDs, id)
		}
		sort.Strings(sortedIDs)
		_, _ = fmt.Fprintf(w, "  class %s changed\n", strings.Join(sortedIDs, ","))
		_, _ = fmt.Fprintln(w, "  classDef changed fill:#fde68a,stroke:#b45309,stroke-width:2px;")
	}
	return nil
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

func findingLines(findings []ImpactFinding) []string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		observed := "observed"
		if !finding.Observed {
			observed = "inferred"
		}
		line := fmt.Sprintf("  [%s] %s (%s)", finding.Severity, finding.Message, observed)
		if evidence := evidenceSummary(finding.Evidence); evidence != "" {
			line += " — " + evidence
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
