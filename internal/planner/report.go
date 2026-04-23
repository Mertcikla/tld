package planner

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
)

// RenderPlanMarkdown writes a human-readable plan report to w.
func RenderPlanMarkdown(w io.Writer, plan *Plan, ws *workspace.Workspace, verbose bool) {
	renderElementPlanMarkdown(w, plan, ws, verbose)
}

func renderElementPlanMarkdown(w io.Writer, plan *Plan, ws *workspace.Workspace, verbose bool) {
	summary := summarizePlanActions(ws)
	_, _ = fmt.Fprintf(w, "Plan: %d to create (+), %d to update (~), %d to delete (-)\n\n", summary.created, summary.updated, summary.deleted)
	_, _ = fmt.Fprintln(w, "# Element Plan")
	_, _ = fmt.Fprintln(w)

	if verbose {
		_, _ = fmt.Fprintln(w, "## View Structure")
		_, _ = fmt.Fprintln(w)
		renderElementTree(w, ws)
		_, _ = fmt.Fprintln(w)

		_, _ = fmt.Fprintln(w, "## Actions")
		_, _ = fmt.Fprintln(w)
		for _, line := range detailedPlanLines(w, ws) {
			_, _ = fmt.Fprintln(w, line)
		}
		_, _ = fmt.Fprintln(w)
	}

	if !verbose {
		_, _ = fmt.Fprintln(w, "Use '-v' or '--verbose' for detailed element placement and connector reporting.")
		_, _ = fmt.Fprintln(w)
	}

	viewCount := 0
	for _, element := range ws.Elements {
		if element.HasView {
			viewCount++
		}
	}

	_, _ = fmt.Fprintln(w, "## Summary")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "| Resource   | Count |")
	_, _ = fmt.Fprintln(w, "|------------|-------|")
	_, _ = fmt.Fprintf(w, "| Elements   | %5d |\n", len(ws.Elements))
	_, _ = fmt.Fprintf(w, "| Views      | %5d |\n", viewCount)
	_, _ = fmt.Fprintf(w, "| Connectors | %5d |\n", len(ws.Connectors))
	_, _ = fmt.Fprintln(w)

}

type planActionSummary struct {
	created int
	updated int
	deleted int
}

func summarizePlanActions(ws *workspace.Workspace) planActionSummary {
	summary := planActionSummary{}
	var elementMeta map[string]*workspace.ResourceMetadata
	var viewMeta map[string]*workspace.ResourceMetadata
	var connectorMeta map[string]*workspace.ResourceMetadata
	if ws.Meta != nil {
		elementMeta = ws.Meta.Elements
		viewMeta = ws.Meta.Views
		connectorMeta = ws.Meta.Connectors
	}
	for ref, element := range ws.Elements {
		if hasMetadata(ws.Meta, elementMeta, ref) {
			summary.updated++
		} else {
			summary.created++
		}
		if element.HasView {
			if hasMetadata(ws.Meta, viewMeta, ref) {
				summary.updated++
			} else {
				summary.created++
			}
		}
	}
	for ref := range ws.Connectors {
		if hasMetadata(ws.Meta, connectorMeta, ref) {
			summary.updated++
		} else {
			summary.created++
		}
	}
	return summary
}

func detailedPlanLines(w io.Writer, ws *workspace.Workspace) []string {
	var lines []string
	var elementMeta map[string]*workspace.ResourceMetadata
	var viewMeta map[string]*workspace.ResourceMetadata
	var connectorMeta map[string]*workspace.ResourceMetadata
	if ws.Meta != nil {
		elementMeta = ws.Meta.Elements
		viewMeta = ws.Meta.Views
		connectorMeta = ws.Meta.Connectors
	}
	refs := make([]string, 0, len(ws.Elements))
	for ref := range ws.Elements {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		element := ws.Elements[ref]
		lines = append(lines, formatPlanActionLine(actionPrefix(w, actionForRef(ws.Meta, elementMeta, ref)), fmt.Sprintf("element %s (%s) [%s]", ref, element.Name, element.Kind)))
		if element.HasView {
			label := element.ViewLabel
			if label == "" {
				label = element.Name
			}
			lines = append(lines, formatPlanActionLine(actionPrefix(w, actionForRef(ws.Meta, viewMeta, ref)), fmt.Sprintf("view %s (%s)", ref, label)))
		}
	}

	connectorRefs := make([]string, 0, len(ws.Connectors))
	for ref := range ws.Connectors {
		connectorRefs = append(connectorRefs, ref)
	}
	sort.Strings(connectorRefs)
	for _, ref := range connectorRefs {
		connector := ws.Connectors[ref]
		lines = append(lines, formatPlanActionLine(actionPrefix(w, actionForRef(ws.Meta, connectorMeta, ref)), fmt.Sprintf("connector %s (%s -> %s)", ref, connector.Source, connector.Target)))
	}
	return lines
}

func formatPlanActionLine(prefix, description string) string {
	return fmt.Sprintf("%s %s", prefix, description)
}

func actionPrefix(w io.Writer, action string) string {
	switch action {
	case "update":
		return term.Colorize(w, term.ColorYellow, "~")
	default:
		return term.Colorize(w, term.ColorGreen, "+")
	}
}

func actionForRef(meta *workspace.Meta, bucket map[string]*workspace.ResourceMetadata, ref string) string {
	if hasMetadata(meta, bucket, ref) {
		return "update"
	}
	return "create"
}

func hasMetadata(meta *workspace.Meta, bucket map[string]*workspace.ResourceMetadata, ref string) bool {
	if meta == nil || bucket == nil {
		return false
	}
	_, ok := bucket[ref]
	return ok
}

func renderElementTree(w io.Writer, ws *workspace.Workspace) {
	children := make(map[string][]string)
	roots := []string{}
	for ref, element := range ws.Elements {
		if len(element.Placements) == 0 {
			roots = append(roots, ref)
			continue
		}
		rooted := false
		for _, placement := range element.Placements {
			if placement.ParentRef == "" || placement.ParentRef == "root" || placement.ParentRef == syntheticRootViewRef {
				rooted = true
				continue
			}
			children[placement.ParentRef] = append(children[placement.ParentRef], ref)
		}
		if rooted {
			roots = append(roots, ref)
		}
	}
	sort.Strings(roots)
	for parent := range children {
		sort.Strings(children[parent])
	}
	visited := make(map[string]bool)
	for _, root := range roots {
		printElementNode(w, ws, children, root, 0, visited)
	}
}

func printElementNode(w io.Writer, ws *workspace.Workspace, children map[string][]string, ref string, depth int, visited map[string]bool) {
	if visited[ref] {
		return
	}
	visited[ref] = true
	element := ws.Elements[ref]
	indent := strings.Repeat("  ", depth)
	viewSuffix := ""
	if element.HasView {
		viewSuffix = " [view]"
	}
	_, _ = fmt.Fprintf(w, "%s- **%s**%s: %s\n", indent, ref, viewSuffix, element.Name)
	for _, child := range children[ref] {
		printElementNode(w, ws, children, child, depth+1, visited)
	}
}
