package render

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRenderCmd(wdir *string) *cobra.Command {
	var (
		format string
		output string
	)

	c := &cobra.Command{
		Use:   "render <view>",
		Short: "Render a workspace view to text output formats",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			view := normalizeViewRef(args[0])
			if !strings.EqualFold(format, "mermaid") {
				return fmt.Errorf("unsupported --format %q (supported: mermaid)", format)
			}
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			if view != workspace.RootRef {
				if _, ok := ws.Elements[view]; !ok {
					return fmt.Errorf("view %q not found", view)
				}
			}

			content, err := renderMermaid(ws, view)
			if err != nil {
				return err
			}

			if output == "" {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), content)
				return nil
			}
			if err := os.WriteFile(output, []byte(content), 0600); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", output)
			return nil
		},
	}

	c.Flags().StringVar(&format, "format", "mermaid", "render output format")
	c.Flags().StringVarP(&output, "output", "o", "", "write render output to file")
	_ = c.RegisterFlagCompletionFunc("view", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ViewRefs(wdir)
	})
	return c
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
		label := ""
		if strings.TrimSpace(c.Label) != "" {
			label = fmt.Sprintf("|%s|", escapeMermaid(c.Label))
		}
		_, _ = fmt.Fprintf(&b, "  %s -->%s %s\n", mermaidID(c.Source), label, mermaidID(c.Target))
	}

	return b.String(), nil
}

func viewElements(ws *workspace.Workspace, view string) []string {
	refs := make([]string, 0)
	for ref, element := range ws.Elements {
		if element == nil {
			continue
		}
		if len(element.Placements) == 0 && view == workspace.RootRef {
			refs = append(refs, ref)
			continue
		}
		for _, placement := range element.Placements {
			if normalizeViewRef(placement.ParentRef) == view {
				refs = append(refs, ref)
				break
			}
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
