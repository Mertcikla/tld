// Package reporter renders execution result summaries for apply operations.
package reporter

import (
	"fmt"
	"io"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/planner"
)

// RenderExecutionMarkdown writes an apply execution report comparing plan vs result.
func RenderExecutionMarkdown(w io.Writer, _ *planner.Plan, resp *diagv1.ApplyPlanResponse, success bool, verbose bool) {
	status := "SUCCESS"
	if !success {
		status = "ROLLED BACK"
	}

	_, _ = fmt.Fprintf(w, "# Apply Report\n\n")
	_, _ = fmt.Fprintf(w, "## Status: %s\n\n", status)

	if resp == nil {
		return
	}

	s := resp.GetSummary()
	if s != nil {
		_, _ = fmt.Fprintln(w, "## Planned vs Created")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "| Resource | Planned | Created |")
		_, _ = fmt.Fprintln(w, "|----------|---------|---------|")
		_, _ = fmt.Fprintf(w, "| Elements | %d | %d |\n", s.ElementsPlanned, s.ElementsCreated)
		_, _ = fmt.Fprintf(w, "| Views | %d | %d |\n", s.ViewsPlanned, s.ViewsCreated)
		_, _ = fmt.Fprintf(w, "| Connectors | %d | %d |\n", s.ConnectorsPlanned, s.ConnectorsCreated)
		_, _ = fmt.Fprintln(w)
	}

	if success && verbose {
		_, _ = fmt.Fprintln(w, "## Created Resources")
		_, _ = fmt.Fprintln(w)

		if len(resp.CreatedViews) > 0 {
			_, _ = fmt.Fprintln(w, "### Views")
			_, _ = fmt.Fprintln(w, "| ID | Name |")
			_, _ = fmt.Fprintln(w, "|----|------|")
			for _, d := range resp.CreatedViews {
				_, _ = fmt.Fprintf(w, "| %d | %s |\n", d.Id, d.Name)
			}
			_, _ = fmt.Fprintln(w)
		}

		if len(resp.CreatedElements) > 0 {
			_, _ = fmt.Fprintln(w, "### Elements")
			_, _ = fmt.Fprintln(w, "| ID | Name | Kind |")
			_, _ = fmt.Fprintln(w, "|----|------|------|")
			for _, o := range resp.CreatedElements {
				kind := ""
				if o.Kind != nil {
					kind = *o.Kind
				}
				_, _ = fmt.Fprintf(w, "| %d | %s | %s |\n", o.Id, o.Name, kind)
			}
			_, _ = fmt.Fprintln(w)
		}

		if len(resp.CreatedConnectors) > 0 {
			_, _ = fmt.Fprintln(w, "### Connectors")
			_, _ = fmt.Fprintln(w, "| ID | Source -> Target |")
			_, _ = fmt.Fprintln(w, "|----|-----------------|")
			for _, e := range resp.CreatedConnectors {
				_, _ = fmt.Fprintf(w, "| %d | %d -> %d |\n", e.Id, e.SourceElementId, e.TargetElementId)
			}
			_, _ = fmt.Fprintln(w)
		}
	}

	if len(resp.Drift) > 0 {
		_, _ = fmt.Fprintln(w, "## Drift")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "| Resource | Ref | Reason |")
		_, _ = fmt.Fprintln(w, "|----------|-----|--------|")
		for _, d := range resp.Drift {
			_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", d.ResourceType, d.Ref, d.Reason)
		}
		_, _ = fmt.Fprintln(w)
	}
}
