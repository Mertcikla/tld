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
	PlainFallback    bool     `json:"plainFallback,omitempty"`
	AttemptedRadii   []uint32 `json:"attemptedRadii,omitempty"`
	Mode             string   `json:"mode"` // diagram, split, summary, or skipped
	Diagrams         int      `json:"diagrams"`
	PrunedContext    int      `json:"prunedContext"`
	PrunedConnectors int      `json:"prunedConnectors"`
	ListedFiles      int      `json:"listedFiles"`
	Reason           string   `json:"reason"`
}

func runAutoCompare(cmd *cobra.Command, opts compareOptions, diagram *model.Diagram, render compareRenderer) error {
	result, decision, reason, err := selectAutoCompare(diagram, opts, render.build, func(radius uint32) (*model.Diagram, error) {
		return render.atRadius(diagram, radius)
	})
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

// selectAutoCompare starts at the requested ceiling and retries each smaller
// radius when a budget is exceeded. load must assemble that exact radius from
// the pinned snapshots: trimming a larger scene leaves authored views behind.
func selectAutoCompare(diagram *model.Diagram, opts compareOptions, build func(*model.Diagram, uint32) (string, int, error), load func(uint32) (*model.Diagram, error)) (scopedDiagram, autoReport, string, error) {
	stats := comparisonStats(diagram)
	last := scopedDiagram{diagram: impact.Scope(diagram, 0)}
	if stats.Files > autoMaxFiles || stats.LinesAdded+stats.LinesRemoved > autoMaxChurn {
		return last, autoReport{}, fmt.Sprintf("auto: change exceeds review scope (%d files, %d known changed lines; limits %d/%d); use a narrower comparison or omit --auto", stats.Files, stats.LinesAdded+stats.LinesRemoved, autoMaxFiles, autoMaxChurn), nil
	}
	var attempted []uint32
	var zero *model.Diagram
	plainFallback := false
	lastReason := "auto: no readable selection"
	lastHop := 0
	if !opts.plain {
		lastHop = -1
	}
	for hops := opts.blastRadius; hops >= lastHop; hops-- {
		radius := uint32(max(hops, 0))
		plainFallback = hops < 0
		attemptOpts := opts
		attemptOpts.plain = opts.plain || plainFallback
		if !plainFallback {
			attempted = append(attempted, radius)
		}
		assembled := zero
		var err error
		if !plainFallback {
			assembled, err = load(radius)
		}
		if radius == 0 {
			zero = assembled
		}
		if err != nil {
			return last, autoReport{AttemptedRadii: attempted}, "", err
		}
		candidate := attemptOpts.scope(assembled, radius)
		beforeNodes, beforeEdges := impact.SceneNodeCount(candidate.Scene), autoEdgeCount(candidate.Scene)
		candidate.Scene = impact.AutoContext(candidate.Scene)
		// Keep the user's ceiling distinct from the chosen assembly radius.
		candidate.Scene.RequestedRadius = uint32(opts.blastRadius)
		recordSelection(candidate.Scene, opts)
		if err := impact.ValidateCoverage(candidate.Scene); err != nil {
			return last, autoReport{AttemptedRadii: attempted}, "", err
		}
		panels := []*model.Assembly{candidate.Scene}
		if opts.markdown {
			panels = impact.SplitAutoScene(candidate.Scene)
		}
		nodes, edges := impact.SceneNodeCount(candidate.Scene), autoEdgeCount(candidate.Scene)
		next := autoReport{Mode: "diagram", Diagrams: 1, AttemptedRadii: attempted, PlainFallback: plainFallback,
			PrunedContext: beforeNodes - nodes, PrunedConnectors: beforeEdges - edges,
			Reason: fmt.Sprintf("Selected radius %d within review budgets", radius)}
		last = scopedDiagram{diagram: candidate, radius: radius}
		if reason := autoLimitReason(candidate, panels, opts); reason != "" {
			lastReason = reason
			continue
		}
		text, size, err := build(candidate, radius)
		if err != nil {
			return last, next, "", err
		}
		if opts.markdown {
			text, next.Mode, next.Diagrams, next.ListedFiles = autoMarkdown(candidate.Scene, panels, attemptOpts.sceneExportOptions())
			if next.PrunedContext > 0 || next.PrunedConnectors > 0 {
				text = fmt.Sprintf("Context trimmed: %d unchanged elements and %d unchanged relationships omitted.\n\n", next.PrunedContext, next.PrunedConnectors) + text
			}
			size = len(text)
		}
		if opts.maxBytes > 0 && size > opts.maxBytes {
			lastReason = fmt.Sprintf("auto: output needs %d bytes, exceeding --max-bytes %d; changed files were not discarded", size, opts.maxBytes)
			continue
		}
		last.text, last.size = text, size
		if plainFallback {
			next.Reason = "Selected flat Plain output at radius 0 within review budgets"
		}
		if nodes == 0 {
			next.Diagrams = 0
			next.Reason = "No changed placements"
		} else if next.Mode == "summary" {
			next.Reason = "No relationships available in the selected scope; showing the complete changed-file list"
		} else if next.Mode == "split" {
			next.Reason = "Disconnected change groups fit in two diagrams without cutting relationships"
		}
		return last, next, "", nil
	}
	return last, autoReport{AttemptedRadii: attempted, PlainFallback: plainFallback}, lastReason + "; no selection fits at radius 0, including plain mode", nil
}

// atRadius reuses the original snapshots and the radius-keyed comparison cache.
// In particular, it never re-resolves moving Git refs or re-runs preparation.
func (r compareRenderer) atRadius(diagram *model.Diagram, radius uint32) (*model.Diagram, error) {
	if radius == impact.DiagramRadius(diagram) {
		return diagram, nil
	}
	ctx, release, err := r.service.Index.AcquireLease(r.ctx, diagram.RepositoryId)
	if err != nil {
		return nil, err
	}
	defer release()
	return r.service.CompareSnapshots(ctx, diagram.RepositoryId, diagram.Diff.GetFromSnapshotId(), diagram.Diff.GetToSnapshotId(), radius, r.opts.view)
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
			return fmt.Sprintf("auto: diagram has %d elements and %d connectors; readability limits are %d elements and %d connectors per diagram; use a narrower comparison or omit --auto", impact.SceneNodeCount(panel), autoEdgeCount(panel), autoPanelNodes, autoPanelEdges)
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
			var counts []string
			if file.GetLinesAdded() > 0 {
				counts = append(counts, fmt.Sprintf("+%d", file.GetLinesAdded()))
			}
			if file.GetLinesRemoved() > 0 {
				counts = append(counts, fmt.Sprintf("−%d", file.GetLinesRemoved()))
			}
			if len(counts) > 0 {
				fmt.Fprintf(out, " (%s)", strings.Join(counts, " / "))
			}
		}
		out.WriteByte('\n')
	}
}
