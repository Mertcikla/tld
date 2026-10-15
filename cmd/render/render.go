package render

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRenderCmd(wdir *string) *cobra.Command {
	var (
		format string
		output string
		view   string
	)

	c := &cobra.Command{
		Use:   "render <view>",
		Short: "Export a view (diagram) as Mermaid",
		Long: `Export a view as Mermaid, preserving connector labels and directions.

A view is required: pass it as an argument or with --view. Use root for the
top-level diagram, an owning element ref or name, or a numeric view ID.
Find available views with 'tld list views --tree'. Output goes to stdout unless
--output is supplied. Use --to mermaid for the diagram format and --format json
for a structured response containing the rendered content and emitted/omitted
connector counts. Off-view relationships remain stored and can appear in the UI
as context/proxy connectors or badges; this Mermaid export draws only directly
placed endpoints. Off-view details are reported on stderr in text mode.`,
		Example: `  tld render root
  tld render platform -o diagram.mmd
  tld render --view root --output diagram.mmd`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := strings.TrimSpace(view)
			if len(args) == 1 {
				target = args[0]
			}
			if strings.TrimSpace(target) == "" {
				return fmt.Errorf("view is required: pass it as an argument or with --view")
			}
			viewRef := normalizeViewRef(target)
			if !strings.EqualFold(format, "mermaid") {
				return fmt.Errorf("unsupported --to %q (supported: mermaid)", format)
			}
			ws, err := loadWorkspace(cmd, *wdir)
			if err != nil {
				return err
			}
			if viewRef != workspace.RootRef {
				resolved, resolveErr := cmdutil.ResolveViewArg(ws, viewRef)
				if resolveErr != nil {
					return fmt.Errorf("view %q not found", target)
				}
				viewRef = resolved
			}

			diagnostics := ws.DiagnoseView(viewRef)
			content, err := renderMermaid(ws, viewRef)
			if err != nil {
				return err
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(content), 0600); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
			}
			if cmdutil.WantsJSONFromCmd(cmd) {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), cmdutil.JSONOutput{
					Command: "render",
					Status:  "ok",
					Summary: map[string]int{"elements": diagnostics.Elements, "connectors": diagnostics.Connectors, "rendered_connectors": diagnostics.RenderedConnectors, "omitted_connectors": len(diagnostics.OmittedConnectors), "off_view_connectors": len(diagnostics.OffViewConnectors), "unresolved_connectors": len(diagnostics.UnresolvedConnectors)},
					Extra: map[string]any{
						"view":                  viewRef,
						"omitted_connectors":    diagnostics.OmittedConnectors,
						"off_view_connectors":   diagnostics.OffViewConnectors,
						"unresolved_connectors": diagnostics.UnresolvedConnectors,
						"connector_visibility":  diagnostics.ConnectorVisibility,
						"off_view_hint":         workspace.OffViewHint,
						"format":                "mermaid",
						"content":               content,
						"output":                output,
					},
				})
			}
			if len(diagnostics.OffViewConnectors) > 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%d off-view connector(s) in %q are omitted from Mermaid only:\n", len(diagnostics.OffViewConnectors), viewRef)
				for _, key := range diagnostics.OffViewConnectors {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "  "+key+": "+diagnostics.ConnectorVisibility[key].Description())
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), workspace.OffViewHint)
			}
			if len(diagnostics.UnresolvedConnectors) > 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: unresolved connectors have missing endpoint definitions; run 'tld validate' for details.")
				for _, key := range diagnostics.UnresolvedConnectors {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "  "+key+": "+diagnostics.ConnectorVisibility[key].Description())
				}
			}
			if output == "" {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), content)
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", output)
			return nil
		},
	}

	c.Flags().StringVar(&format, "to", "mermaid", "diagram format: mermaid")
	c.Flags().StringVarP(&output, "output", "o", "", "write render output to file")
	c.Flags().StringVar(&view, "view", "", "root, owner ref/name, or view ID (alternative to the argument)")
	cmdutil.AddReadSourceFlag(c)
	_ = c.RegisterFlagCompletionFunc("view", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ViewRefs(wdir)
	})
	return c
}

func loadWorkspace(cmd *cobra.Command, wdir string) (*workspace.Workspace, error) {
	sess, err := cmdutil.OpenSession(cmd, wdir, "", "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.Close() }()
	return sess.LoadWorkspace()
}

func renderMermaid(ws *workspace.Workspace, view string) (string, error) {
	if ws == nil {
		return "", fmt.Errorf("workspace is required")
	}

	elements := viewElements(ws, view)
	sort.Strings(elements)
	elementSet := make(map[string]bool, len(elements))
	for _, ref := range elements {
		elementSet[ref] = true
	}

	var b strings.Builder
	b.WriteString("flowchart LR\n")
	if len(elements) == 0 {
		b.WriteString("  empty[\"No elements in view\"]\n")
		return b.String(), nil
	}

	for _, ref := range elements {
		e := ws.Elements[ref]
		label := ref
		if e != nil && strings.TrimSpace(e.Name) != "" {
			label = e.Name
		}
		_, _ = fmt.Fprintf(&b, "  %s[\"%s\"]\n", mermaidID(ref), escapeMermaid(label))
	}

	connectorKeys := make([]string, 0, len(ws.Connectors))
	for key := range ws.Connectors {
		connectorKeys = append(connectorKeys, key)
	}
	sort.Strings(connectorKeys)

	for _, key := range connectorKeys {
		c := ws.Connectors[key]
		if c == nil {
			continue
		}
		if normalizeViewRef(c.View) != view {
			continue
		}
		if !elementSet[c.Source] || !elementSet[c.Target] {
			continue
		}
		from, to := mermaidID(c.Source), mermaidID(c.Target)
		arrow := "-->"
		switch strings.ToLower(strings.TrimSpace(c.Direction)) {
		case "", "forward":
		case "backward":
			from, to = to, from
		case "both", "bidirectional":
			arrow = "<-->"
		case "none":
			arrow = "---"
		default:
			return "", fmt.Errorf("connector %q has unsupported direction %q", key, c.Direction)
		}
		label := ""
		if strings.TrimSpace(c.Label) != "" {
			label = fmt.Sprintf("|%s|", escapeMermaid(c.Label))
		}
		_, _ = fmt.Fprintf(&b, "  %s %s%s %s\n", from, arrow, label, to)
	}

	return b.String(), nil
}

func viewElements(ws *workspace.Workspace, view string) []string {
	refs := make([]string, 0)
	for ref := range ws.Elements {
		if ws.ElementPlacedInView(ref, view) {
			refs = append(refs, ref)
		}
	}
	return refs
}

func mermaidID(ref string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_", ":", "_")
	return "n_" + replacer.Replace(ref)
}

func escapeMermaid(input string) string {
	out := strings.ReplaceAll(input, "\r\n", " ")
	out = strings.ReplaceAll(out, "\n", " ")
	out = strings.ReplaceAll(out, "\r", " ")
	out = strings.ReplaceAll(out, "&", "&amp;")
	out = strings.ReplaceAll(out, "\\", "\\\\")
	out = strings.ReplaceAll(out, "\"", "&quot;")
	return out
}

func normalizeViewRef(ref string) string {
	if strings.TrimSpace(ref) == "" {
		return workspace.RootRef
	}
	return ref
}
