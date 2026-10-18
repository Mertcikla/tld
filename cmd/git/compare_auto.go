package git

import (
	"fmt"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"github.com/mertcikla/tld/v2/internal/mermaid"
	"github.com/spf13/cobra"
)

// These are presentation limits, independent of the CLI's larger transport
// budgets. Auto declines diagrams it cannot simplify without hiding changes.
const (
	autoPanelNodes = 24
	autoPanelEdges = 40
	autoMaxFiles   = 50
	autoMaxChurn   = 2000
)

type autoReport struct {
	Mode             string `json:"mode"` // diagram, split, summary, or skipped
	Diagrams         int    `json:"diagrams"`
	PrunedContext    int    `json:"prunedContext"`
	PrunedConnectors int    `json:"prunedConnectors"`
	ListedFiles      int    `json:"listedFiles"`
	Reason           string `json:"reason"`
}

func runAutoCompare(cmd *cobra.Command, opts compareOptions, diagram *model.Diagram, render compareRenderer) error {
	result, decision, reason, err := selectAutoCompare(diagram, opts, render.build)
	if err != nil {
		return err
	}
	report := comparisonReport(result.diagram, opts)
	report.Auto = &decision
	if reason != "" {
		report.Status, report.Reason = "skipped", reason
		report.Auto.Mode, report.Auto.Reason = "skipped", reason
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+reason)
		return writeCompareReport(opts.reportJSON, report)
	}
	if err := writeProvenance(opts, result.diagram.Scene, cmd.ErrOrStderr()); err != nil {
		return err
	}
	return writeCompareOutput(cmd, opts, result, report)
}

// selectAutoCompare tries increasingly distant context only while the current
// diagram lacks useful relationships. Every candidate retains the full changed
// file ledger; budgets are checked after reduction, including actual bytes.
func selectAutoCompare(diagram *model.Diagram, opts compareOptions, build func(*model.Diagram, uint32) (string, int, error)) (scopedDiagram, autoReport, string, error) {
	stats := comparisonStats(diagram)
	initial := scopedDiagram{diagram: impact.Scope(diagram, 0)}
	if stats.Files > autoMaxFiles || stats.LinesAdded+stats.LinesRemoved > autoMaxChurn {
		return initial, autoReport{}, fmt.Sprintf("auto: change exceeds review scope (%d files, %d known changed lines; limits %d/%d); use a narrower comparison or omit --auto", stats.Files, stats.LinesAdded+stats.LinesRemoved, autoMaxFiles, autoMaxChurn), nil
	}
	maxRadius := uint32(opts.blastRadius)
	if stats.Files > 8 {
		maxRadius = 0
	} else if stats.Files > 2 && maxRadius > 1 {
		maxRadius = 1
	}
	best, decision := initial, autoReport{}
	found := false
	lastReason := "auto: no readable selection"
	for radius := uint32(0); radius <= maxRadius; radius++ {
		candidate := impact.Scope(diagram, radius)
		if opts.plain {
			candidate.Scene = impact.SelectScene(candidate.Scene, radius, true)
		}
		beforeNodes, beforeEdges := impact.SceneNodeCount(candidate.Scene), autoEdgeCount(candidate.Scene)
		candidate.Scene = impact.AutoContext(candidate.Scene)
		recordSelection(candidate.Scene, opts)
		if err := impact.ValidateCoverage(candidate.Scene); err != nil {
			return best, decision, "", err
		}
		panels := []*model.Assembly{candidate.Scene}
		if opts.markdown {
			panels = impact.SplitAutoScene(candidate.Scene)
		}
		nodes, edges := impact.SceneNodeCount(candidate.Scene), autoEdgeCount(candidate.Scene)
		if reason := autoLimitReason(candidate, panels, opts); reason != "" {
			lastReason = reason
			continue
		}
		// An extra hop must add relationships, not just disconnected boxes.
		if found && edges <= autoEdgeCount(best.diagram.Scene) {
			continue
		}
		text, size, err := build(candidate, radius)
		if err != nil {
			return best, decision, "", err
		}
		next := autoReport{Mode: "diagram", Diagrams: 1,
			PrunedContext: beforeNodes - nodes, PrunedConnectors: beforeEdges - edges,
			Reason: "Smallest useful dependency neighbourhood within review budgets"}
		if opts.markdown {
			text, next.Mode, next.Diagrams, next.ListedFiles = autoMarkdown(candidate.Scene, panels, opts.sceneExportOptions())
			if next.PrunedContext > 0 || next.PrunedConnectors > 0 {
				text = fmt.Sprintf("Context trimmed: %d unchanged elements and %d unchanged relationships omitted.\n\n", next.PrunedContext, next.PrunedConnectors) + text
			}
			size = len(text)
		}
		if opts.maxBytes > 0 && size > opts.maxBytes {
			lastReason = fmt.Sprintf("auto: output needs %d bytes, exceeding --max-bytes %d; changed files were not discarded", size, opts.maxBytes)
			continue
		}
		best = scopedDiagram{diagram: candidate, radius: radius, text: text, size: size}
		decision, found = next, true
		if nodes == 0 {
			decision.Diagrams = 0
			decision.Reason = "No changed placements"
			break
		}
		// Small changes always get a chance at one hop. Larger changes already
		// showing relationships do not need unrelated context added around them.
		if edges > 0 && (radius > 0 || stats.Files > 2) {
			break
		}
	}
	if !found {
		return initial, decision, lastReason, nil
	}
	switch decision.Mode {
	case "summary":
		decision.Reason = "No relationships available in the selected scope; showing the complete changed-file list"
	case "split":
		decision.Reason = "Disconnected change groups fit in two diagrams without cutting relationships"
	}
	return best, decision, "", nil
}

func autoEdgeCount(scene *model.Assembly) int {
	count := 0
	for _, view := range scene.GetViews() {
		count += len(view.Connectors)
	}
	return count
}

func autoLimitReason(diagram *model.Diagram, panels []*model.Assembly, opts compareOptions) string {
	report := comparisonReport(diagram, opts)
	if report.Status == "skipped" {
		return "auto: " + report.Reason
	}
	if opts.maxNodes > 0 && report.Elements > opts.maxNodes {
		return fmt.Sprintf("auto: %d changed/context elements exceed --max-nodes %d", report.Elements, opts.maxNodes)
	}
	for _, panel := range panels {
		if opts.markdown {
			if autoEdgeCount(diagram.Scene) == 0 {
				continue // A file summary has no diagram to constrain.
			}
			if impact.SceneNodeCount(diagram.Scene) > 12 {
				panel, _ = impact.SeparateAutoFiles(panel)
			}
		}
		if impact.SceneNodeCount(panel) > autoPanelNodes || autoEdgeCount(panel) > autoPanelEdges {
			return fmt.Sprintf("auto: diagram exceeds readability limits (%d elements or %d connectors per diagram); use a narrower comparison or omit --auto", autoPanelNodes, autoPanelEdges)
		}
	}
	return ""
}

func autoMarkdown(scene *model.Assembly, panels []*model.Assembly, exportOpts mermaid.SceneExportOptions) (string, string, int, int) {
	if len(scene.GetFiles()) > 0 && autoEdgeCount(scene) == 0 {
		var out strings.Builder
		out.WriteString("No relationships available in the selected scope. Changed files:\n\n")
		writeAutoFiles(&out, scene.Files)
		return out.String(), "summary", 0, len(scene.Files)
	}
	var listed []*pb.ImpactFile
	if impact.SceneNodeCount(scene) > 12 {
		selected := make([]*model.Assembly, 0, len(panels))
		for _, panel := range panels {
			part, files := impact.SeparateAutoFiles(panel)
			listed = append(listed, files...)
			if impact.SceneNodeCount(part) > 0 {
				selected = append(selected, part)
			}
		}
		panels = selected
	}
	var out strings.Builder
	exportOpts.SubgraphDirection = "LR"
	for i, panel := range panels {
		if len(panels) > 1 {
			fmt.Fprintf(&out, "Change group %d of %d (disconnected in the selected scope):\n\n", i+1, len(panels))
		}
		out.WriteString(mermaid.MermaidBlock(mermaid.ExportImpactScene(impact.PublicScene(panel, false), exportOpts)))
		if i+1 < len(panels) {
			out.WriteByte('\n')
		}
	}
	if len(listed) > 0 {
		out.WriteString("\nAlso changed (no relationships in the selected scope):\n\n")
		sort.Slice(listed, func(i, j int) bool { return listed[i].Path < listed[j].Path })
		writeAutoFiles(&out, listed)
	}
	mode := "diagram"
	if len(panels) > 1 {
		mode = "split"
	}
	return out.String(), mode, len(panels), len(listed)
}

func writeAutoFiles(out *strings.Builder, files []*pb.ImpactFile) {
	// Escape all markup in repository-controlled paths, including newlines.
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "`", "&#96;", "\n", "&#10;", "\r", "&#13;", "*", "&#42;", "_", "&#95;", "[", "&#91;", "]", "&#93;", "\\", "&#92;", "|", "&#124;", "~", "&#126;")
	for _, file := range files {
		fmt.Fprintf(out, "- <code>%s</code>", escape.Replace(file.Path))
		switch file.Change {
		case pb.ChangeKind_CHANGE_KIND_ADDED:
			out.WriteString(" — added")
		case pb.ChangeKind_CHANGE_KIND_REMOVED:
			out.WriteString(" — removed")
		case pb.ChangeKind_CHANGE_KIND_MODIFIED:
			out.WriteString(" — modified")
		}
		if file.LinesAdded != nil && file.LinesRemoved != nil {
			fmt.Fprintf(out, " (+%d / −%d)", file.GetLinesAdded(), file.GetLinesRemoved())
		}
		out.WriteByte('\n')
	}
}
