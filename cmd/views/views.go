package views

import (
	"github.com/mertcikla/diag/tld/internal/cmdutil"
	"fmt"
	"io"
	"sort"

	"github.com/mertcikla/diag/tld/internal/planner"
	"github.com/mertcikla/diag/tld/internal/workspace"
	"github.com/spf13/cobra"
)

const rootViewRef = "root"

type viewSummaryRow struct {
	Ref              string `json:"ref"`
	OwnerRef         string `json:"owner_ref,omitempty"`
	OwnerName        string `json:"owner_name"`
	Depth            int    `json:"depth"`
	DirectElements   int    `json:"direct_elements"`
	DirectChildViews int    `json:"direct_child_views"`
	Connectors       int    `json:"connectors"`
	Path             string `json:"path"`
	Synthetic        bool   `json:"synthetic,omitempty"`
}

func NewViewsCmd(wdir *string) *cobra.Command {
	c := &cobra.Command{
		Use:   "views",
		Short: "Show derived view structure for the workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "views", err)
				}
				return err
			}

			if errs := ws.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}); len(errs) > 0 {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					messages := make([]string, 0, len(errs))
					for _, validationErr := range errs {
						messages = append(messages, validationErr.Error())
					}
					return cmdutil.WriteJSON(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", planner.JSONOutput{
						Command: "views",
						Status:  "error",
						Errors:  messages,
					})
				}
				for _, validationErr := range errs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "validation error: %s\n", validationErr)
				}
				return fmt.Errorf("workspace has %d validation error(s)", len(errs))
			}

			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			if repoCtx.Name != "" && repoCtx.MatchesWorkspaceRepo(ws) {
				ws.ActiveRepo = repoCtx.Name
			}

			rows := summarizeViews(ws)
			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", buildViewsJSONOutput(rows))
			}

			renderViewsTable(cmd.OutOrStdout(), rows)
			return nil
		},
	}

	return c
}

func summarizeViews(ws *workspace.Workspace) []viewSummaryRow {
	included := cmdutil.IncludedElementRefs(ws)
	directElements := make(map[string]map[string]bool)
	directChildViews := make(map[string]map[string]bool)
	adjacency := make(map[string]map[string]bool)
	connectorsByView := make(map[string]int)

	registerPlacement := func(parentRef, ref string, hasView bool) {
		parentRef = normalizeViewRef(parentRef)
		ensureSet(directElements, parentRef)[ref] = true
		if hasView {
			ensureSet(directChildViews, parentRef)[ref] = true
			ensureSet(adjacency, parentRef)[ref] = true
		}
	}

	for ref, element := range ws.Elements {
		if !included[ref] {
			continue
		}
		if len(element.Placements) == 0 {
			registerPlacement(rootViewRef, ref, element.HasView)
			continue
		}
		for _, placement := range element.Placements {
			registerPlacement(placement.ParentRef, ref, element.HasView)
		}
	}

	for _, connector := range ws.Connectors {
		if !included[connector.Source] || !included[connector.Target] {
			continue
		}
		connectorsByView[normalizeViewRef(connector.View)]++
	}

	depthByView, pathByView := buildViewPaths(adjacency)
	rows := []viewSummaryRow{{
		Ref:              rootViewRef,
		OwnerName:        "Synthetic Root",
		Depth:            0,
		DirectElements:   len(directElements[rootViewRef]),
		DirectChildViews: len(directChildViews[rootViewRef]),
		Connectors:       connectorsByView[rootViewRef],
		Path:             rootViewRef,
		Synthetic:        true,
	}}

	for ref, element := range ws.Elements {
		if !included[ref] || !element.HasView {
			continue
		}
		depth, ok := depthByView[ref]
		if !ok {
			depth = -1
		}
		path := pathByView[ref]
		if path == "" {
			path = "unreachable"
		}
		rows = append(rows, viewSummaryRow{
			Ref:              ref,
			OwnerRef:         ref,
			OwnerName:        element.Name,
			Depth:            depth,
			DirectElements:   len(directElements[ref]),
			DirectChildViews: len(directChildViews[ref]),
			Connectors:       connectorsByView[ref],
			Path:             path,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Synthetic != rows[j].Synthetic {
			return rows[i].Synthetic
		}
		if rows[i].Depth != rows[j].Depth {
			return rows[i].Depth < rows[j].Depth
		}
		if rows[i].Path != rows[j].Path {
			return rows[i].Path < rows[j].Path
		}
		return rows[i].Ref < rows[j].Ref
	})

	return rows
}

func renderViewsTable(w io.Writer, rows []viewSummaryRow) {
	ownedViews, maxDepth := summarizeViewMetrics(rows)
	_, _ = fmt.Fprintf(w, "Views: %d total (%d owned + root)\n", len(rows), ownedViews)
	_, _ = fmt.Fprintf(w, "Max depth: %d\n\n", maxDepth)
	_, _ = fmt.Fprintln(w, "| View | Owner | Depth | Elements | Child Views | Connectors | Path |")
	_, _ = fmt.Fprintln(w, "|------|-------|-------|----------|-------------|------------|------|")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %d | %s |\n", row.Ref, row.OwnerName, row.Depth, row.DirectElements, row.DirectChildViews, row.Connectors, row.Path)
	}
}

func buildViewsJSONOutput(rows []viewSummaryRow) planner.JSONOutput {
	ownedViews, maxDepth := summarizeViewMetrics(rows)
	items := make([]planner.JSONItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, planner.JSONItem{
			Ref:          row.Ref,
			ResourceType: "view",
			Action:       "present",
			Name:         row.OwnerName,
		})
	}
	return planner.JSONOutput{
		Command: "views",
		Status:  "ok",
		Summary: map[string]int{
			"total_views": len(rows),
			"owned_views": ownedViews,
			"max_depth":   maxDepth,
		},
		Items: items,
		Extra: map[string]any{
			"views": rows,
		},
	}
}

func summarizeViewMetrics(rows []viewSummaryRow) (int, int) {
	ownedViews := 0
	maxDepth := 0
	for _, row := range rows {
		if row.Synthetic {
			continue
		}
		ownedViews++
		if row.Depth > maxDepth {
			maxDepth = row.Depth
		}
	}
	return ownedViews, maxDepth
}

func buildViewPaths(adjacency map[string]map[string]bool) (map[string]int, map[string]string) {
	depthByView := map[string]int{rootViewRef: 0}
	pathByView := map[string]string{rootViewRef: rootViewRef}
	queue := []string{rootViewRef}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		children := sortedSetKeys(adjacency[current])
		for _, child := range children {
			if _, seen := depthByView[child]; seen {
				continue
			}
			depthByView[child] = depthByView[current] + 1
			pathByView[child] = pathByView[current] + "/" + child
			queue = append(queue, child)
		}
	}

	return depthByView, pathByView
}

func normalizeViewRef(ref string) string {
	if ref == "" {
		return rootViewRef
	}
	return ref
}

func ensureSet(sets map[string]map[string]bool, ref string) map[string]bool {
	if sets[ref] == nil {
		sets[ref] = make(map[string]bool)
	}
	return sets[ref]
}

func sortedSetKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
