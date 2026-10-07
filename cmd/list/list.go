package list

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewListCmd groups read-only inventory commands mirroring the frontend's
// element/connector/view lists, backed by the local YAML cache.
func NewListCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "List workspace resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return cobra.NoArgs(cmd, args)
		},
	}
	c.AddCommand(
		newElementsCmd(wdir, format, compact),
		newConnectorsCmd(wdir, format, compact),
		newViewsCmd(wdir, format, compact),
	)
	return c
}

type elementRow struct {
	Ref        string   `json:"ref"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Technology string   `json:"technology,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	HasView    bool     `json:"has_view"`
	ID         int32    `json:"id,omitempty"`
}

type connectorRow struct {
	Key          string `json:"key"`
	View         string `json:"view"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	Label        string `json:"label,omitempty"`
	Relationship string `json:"relationship,omitempty"`
	Direction    string `json:"direction,omitempty"`
	ID           int32  `json:"id,omitempty"`
}

type viewRow struct {
	Ref        string `json:"ref"`
	Name       string `json:"name"`
	Label      string `json:"label,omitempty"`
	Parent     string `json:"parent,omitempty"`
	Depth      int    `json:"depth"`
	Path       string `json:"path"`
	Elements   int    `json:"direct_elements"`
	ChildViews int    `json:"child_views"`
	Connectors int    `json:"connectors"`
	ID         int32  `json:"id,omitempty"`
	Synthetic  bool   `json:"synthetic,omitempty"`
}

func newElementsCmd(wdir, format *string, compact *bool) *cobra.Command {
	var search string
	var kind string
	c := &cobra.Command{
		Use:   "elements",
		Short: "List elements",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return failf(cmd, *format, *compact, "list elements", err)
			}
			rows := buildElementRows(ws)
			filtered := rows[:0]
			for _, row := range rows {
				if kind != "" && row.Kind != kind {
					continue
				}
				if !matches(search, row.Ref, row.Name, row.Kind, row.Technology, row.Owner, strings.Join(row.Tags, " ")) {
					continue
				}
				filtered = append(filtered, row)
			}
			if cmdutil.WantsJSON(*format) {
				items := make([]cmdutil.JSONItem, 0, len(filtered))
				for _, row := range filtered {
					items = append(items, cmdutil.JSONItem{Ref: row.Ref, ResourceType: "element", Action: "present", Name: row.Name})
				}
				return cmdutil.WriteJSON(cmd.OutOrStdout(), *compact, cmdutil.JSONOutput{
					Command: "list elements",
					Status:  "ok",
					Summary: map[string]int{"count": len(filtered), "total": len(rows)},
					Items:   items,
					Extra:   map[string]any{"elements": filtered},
				})
			}
			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "REF\tNAME\tKIND\tTECHNOLOGY\tOWNER\tTAGS\tVIEW\tID")
			for _, row := range filtered {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					row.Ref, row.Name, row.Kind, row.Technology, row.Owner,
					strings.Join(row.Tags, ","), yesNo(row.HasView), idString(row.ID))
			}
			_ = w.Flush()
			term.Infof(out, "%d of %d element(s)", len(filtered), len(rows))
			return nil
		},
	}
	c.Flags().StringVar(&search, "search", "", "substring filter across ref, name, kind, technology, owner, and tags")
	c.Flags().StringVar(&kind, "kind", "", "only show elements of this kind")
	return c
}

func newConnectorsCmd(wdir, format *string, compact *bool) *cobra.Command {
	var search string
	var view string
	c := &cobra.Command{
		Use:   "connectors",
		Short: "List connectors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return failf(cmd, *format, *compact, "list connectors", err)
			}
			rows := buildConnectorRows(ws)
			filtered := rows[:0]
			for _, row := range rows {
				if view != "" && row.View != view {
					continue
				}
				if !matches(search, row.Key, row.View, row.Source, row.Target, row.Label, row.Relationship) {
					continue
				}
				filtered = append(filtered, row)
			}
			if cmdutil.WantsJSON(*format) {
				items := make([]cmdutil.JSONItem, 0, len(filtered))
				for _, row := range filtered {
					items = append(items, cmdutil.JSONItem{Ref: row.Key, ResourceType: "connector", Action: "present", Name: row.Label})
				}
				return cmdutil.WriteJSON(cmd.OutOrStdout(), *compact, cmdutil.JSONOutput{
					Command: "list connectors",
					Status:  "ok",
					Summary: map[string]int{"count": len(filtered), "total": len(rows)},
					Items:   items,
					Extra:   map[string]any{"connectors": filtered},
				})
			}
			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "VIEW\tSOURCE\tTARGET\tLABEL\tRELATIONSHIP\tDIRECTION\tID")
			for _, row := range filtered {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					row.View, row.Source, row.Target, row.Label, row.Relationship, row.Direction, idString(row.ID))
			}
			_ = w.Flush()
			term.Infof(out, "%d of %d connector(s)", len(filtered), len(rows))
			return nil
		},
	}
	c.Flags().StringVar(&search, "search", "", "substring filter across view, source, target, label, and relationship")
	c.Flags().StringVar(&view, "view", "", "only show connectors bound to this view ref")
	return c
}

func newViewsCmd(wdir, format *string, compact *bool) *cobra.Command {
	var search string
	var parent string
	var tree bool
	c := &cobra.Command{
		Use:   "views",
		Short: "List views (diagrams)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return failf(cmd, *format, *compact, "list views", err)
			}
			rows := buildViewRows(ws)
			filtered := rows[:0]
			for _, row := range rows {
				if parent != "" && row.Parent != parent {
					continue
				}
				if !matches(search, row.Ref, row.Name, row.Label, row.Parent) {
					continue
				}
				filtered = append(filtered, row)
			}
			if tree {
				sortViewTreeRows(filtered)
			}
			if cmdutil.WantsJSON(*format) {
				items := make([]cmdutil.JSONItem, 0, len(filtered))
				for _, row := range filtered {
					items = append(items, cmdutil.JSONItem{Ref: row.Ref, ResourceType: "view", Action: "present", Name: row.Name})
				}
				return cmdutil.WriteJSON(cmd.OutOrStdout(), *compact, cmdutil.JSONOutput{
					Command: "list views",
					Status:  "ok",
					Summary: map[string]int{"count": len(filtered), "total": len(rows)},
					Items:   items,
					Extra:   map[string]any{"views": filtered},
				})
			}
			out := cmd.OutOrStdout()
			if tree {
				renderViewTree(out, filtered)
				term.Infof(out, "%d of %d view(s)", len(filtered), len(rows))
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "REF\tNAME\tLABEL\tPARENT\tELEMENTS\tCHILD VIEWS\tCONNECTORS\tID")
			for _, row := range filtered {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
					row.Ref, row.Name, row.Label, row.Parent, row.Elements, row.ChildViews, row.Connectors, idString(row.ID))
			}
			_ = w.Flush()
			term.Infof(out, "%d of %d view(s)", len(filtered), len(rows))
			return nil
		},
	}
	c.Flags().StringVar(&search, "search", "", "substring filter across ref, name, label, and parent")
	c.Flags().StringVar(&parent, "parent", "", "only show views placed under this parent ref")
	c.Flags().BoolVar(&tree, "tree", false, "render the derived view hierarchy as a tree")
	return c
}

func buildElementRows(ws *workspace.Workspace) []elementRow {
	rows := make([]elementRow, 0, len(ws.Elements))
	for ref, el := range ws.Elements {
		if el == nil {
			continue
		}
		rows = append(rows, elementRow{
			Ref:        ref,
			Name:       el.Name,
			Kind:       el.Kind,
			Technology: el.Technology,
			Owner:      el.Owner,
			Tags:       el.Tags,
			HasView:    el.HasView,
			ID:         metaID(ws, "elements", ref),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ref < rows[j].Ref })
	return rows
}

func buildConnectorRows(ws *workspace.Workspace) []connectorRow {
	rows := make([]connectorRow, 0, len(ws.Connectors))
	for key, connector := range ws.Connectors {
		if connector == nil {
			continue
		}
		rows = append(rows, connectorRow{
			Key:          key,
			View:         connector.View,
			Source:       connector.Source,
			Target:       connector.Target,
			Label:        connector.Label,
			Relationship: connector.Relationship,
			Direction:    connector.Direction,
			ID:           metaID(ws, "connectors", key),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	return rows
}

func buildViewRows(ws *workspace.Workspace) []viewRow {
	directElements := map[string]int{}
	childViews := map[string]int{}
	parentOf := map[string]string{}
	connectorsByView := map[string]int{}
	adjacency := map[string]map[string]bool{}

	registerView := func(parent, ref string) {
		if parent == "" {
			parent = "root"
		}
		if adjacency[parent] == nil {
			adjacency[parent] = map[string]bool{}
		}
		adjacency[parent][ref] = true
	}

	for ref, el := range ws.Elements {
		if el == nil {
			continue
		}
		placements := el.Placements
		if len(placements) == 0 {
			directElements["root"]++
			if el.HasView {
				childViews["root"]++
				registerView("root", ref)
			}
			continue
		}
		for _, placement := range placements {
			parent := placement.ParentRef
			if parent == "" {
				parent = "root"
			}
			directElements[parent]++
			if el.HasView {
				childViews[parent]++
				registerView(parent, ref)
			}
			if parentOf[ref] == "" {
				parentOf[ref] = parent
			}
		}
	}
	for _, connector := range ws.Connectors {
		if connector == nil {
			continue
		}
		view := connector.View
		if view == "" {
			view = "root"
		}
		connectorsByView[view]++
	}

	depthByView, pathByView := buildViewPaths(adjacency)

	rows := []viewRow{{
		Ref:        "root",
		Name:       "Workspace Root",
		Depth:      0,
		Path:       "root",
		Elements:   directElements["root"],
		ChildViews: childViews["root"],
		Connectors: connectorsByView["root"],
		ID:         metaID(ws, "views", "root"),
		Synthetic:  true,
	}}
	for ref, el := range ws.Elements {
		if el == nil || !el.HasView {
			continue
		}
		name := el.ViewName
		if name == "" {
			name = el.Name
		}
		depth, ok := depthByView[ref]
		if !ok {
			depth = -1
		}
		path := pathByView[ref]
		if path == "" {
			path = "unreachable"
		}
		rows = append(rows, viewRow{
			Ref:        ref,
			Name:       name,
			Label:      el.ViewLabel,
			Parent:     parentOf[ref],
			Depth:      depth,
			Path:       path,
			Elements:   directElements[ref],
			ChildViews: childViews[ref],
			Connectors: connectorsByView[ref],
			ID:         metaID(ws, "views", ref),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Synthetic != rows[j].Synthetic {
			return rows[i].Synthetic
		}
		return rows[i].Ref < rows[j].Ref
	})
	return rows
}

// buildViewPaths walks the view adjacency from the synthetic root and returns
// each view's depth and "root/.../view" path.
func buildViewPaths(adjacency map[string]map[string]bool) (map[string]int, map[string]string) {
	depthByView := map[string]int{"root": 0}
	pathByView := map[string]string{"root": "root"}
	queue := []string{"root"}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		children := make([]string, 0, len(adjacency[current]))
		for child := range adjacency[current] {
			children = append(children, child)
		}
		sort.Strings(children)
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

// sortViewTreeRows orders rows in hierarchy order: root first, then by depth,
// path, and ref.
func sortViewTreeRows(rows []viewRow) {
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
}

// renderViewTree prints the derived view hierarchy using branch glyphs, similar
// to the unix `tree` command. Each view is nested under the view that owns its
// placement (the synthetic workspace root sits at the top).
func renderViewTree(w io.Writer, rows []viewRow) {
	if len(rows) == 0 {
		return
	}
	byRef := make(map[string]viewRow, len(rows))
	for _, row := range rows {
		byRef[row.Ref] = row
	}

	children := make(map[string][]viewRow)
	roots := make([]viewRow, 0, 1)
	for _, row := range rows {
		parent := viewTreeParent(row)
		if parent == "" {
			roots = append(roots, row)
			continue
		}
		if _, ok := byRef[parent]; !ok {
			roots = append(roots, row)
			continue
		}
		children[parent] = append(children[parent], row)
	}
	sortViewTreeRows(roots)
	for ref := range children {
		sortViewTreeRows(children[ref])
	}

	visited := make(map[string]bool, len(rows))
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "VIEW\tREF\tELEMENTS\tCONNECTORS")
	for _, root := range roots {
		if visited[root.Ref] {
			continue
		}
		visited[root.Ref] = true
		writeViewTreeLine(tw, "", "", root)
		renderViewTreeChildren(tw, root.Ref, "", children, visited)
	}
	_ = tw.Flush()
	writeStripedRows(w, buf.String())
}

// writeStripedRows writes tab-aligned tree lines, underlining the header and
// applying a subtle background to alternating rows so long rows are easier to
// follow. Styling happens after tab alignment so ANSI sequences never affect
// column widths.
func writeStripedRows(w io.Writer, text string) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if !term.IsColorEnabled(w) {
		for _, line := range lines {
			_, _ = fmt.Fprintln(w, line)
		}
		return
	}
	width := 0
	for _, line := range lines {
		if n := utf8.RuneCountInString(line); n > width {
			width = n
		}
	}
	for i, line := range lines {
		if i == 0 {
			_, _ = fmt.Fprintln(w, term.Colorize(w, term.ColorUnderline, line))
			continue
		}
		if pad := width - utf8.RuneCountInString(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		_, _ = fmt.Fprintln(w, term.Stripe(w, i, line))
	}
}

func renderViewTreeChildren(w io.Writer, parentRef, prefix string, children map[string][]viewRow, visited map[string]bool) {
	kids := children[parentRef]
	for i, child := range kids {
		if visited[child.Ref] {
			continue
		}
		visited[child.Ref] = true
		branch, childPrefix := "├── ", prefix+"│   "
		if i == len(kids)-1 {
			branch, childPrefix = "└── ", prefix+"    "
		}
		writeViewTreeLine(w, prefix, branch, child)
		renderViewTreeChildren(w, child.Ref, childPrefix, children, visited)
	}
}

// writeViewTreeLine writes one node, keeping the ref and counts in separate
// tabs so the caller's tabwriter aligns them into distinct columns.
func writeViewTreeLine(w io.Writer, prefix, branch string, row viewRow) {
	label := prefix + branch + viewTreeLabel(row)
	_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", label, row.Ref, row.Elements, row.Connectors)
}

// viewTreeParent resolves the owning view ref for a row, preferring the direct
// parent placement and falling back to the derived path.
func viewTreeParent(row viewRow) string {
	if row.Parent != "" {
		return row.Parent
	}
	if idx := strings.LastIndex(row.Path, "/"); idx > 0 {
		return row.Path[:idx]
	}
	return ""
}

func viewTreeLabel(row viewRow) string {
	if row.Name != "" {
		return row.Name
	}
	return row.Ref
}

func metaID(ws *workspace.Workspace, kind, ref string) int32 {
	if ws == nil || ws.Meta == nil {
		return 0
	}
	var metadata map[string]*workspace.ResourceMetadata
	switch kind {
	case "elements":
		metadata = ws.Meta.Elements
	case "views":
		metadata = ws.Meta.Views
	case "connectors":
		metadata = ws.Meta.Connectors
	}
	if m, ok := metadata[ref]; ok && m != nil {
		return int32(m.ID)
	}
	return 0
}

func matches(search string, values ...string) bool {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}
	return false
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func idString(id int32) string {
	if id == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", id)
}

func failf(cmd *cobra.Command, format string, compact bool, command string, err error) error {
	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, command, err)
	}
	return err
}
