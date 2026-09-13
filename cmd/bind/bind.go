package bind

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewBindCmd binds an existing architecture element to the code it owns. The
// binding is what `tld impact` uses to map changes back to the architecture.
func NewBindCmd(wdir, format *string, compact *bool) *cobra.Command {
	var filePath, symbol string

	c := &cobra.Command{
		Use:   "bind <ref>",
		Short: "Bind an architecture element to the code it owns",
		Long: `Attach a code path (and optionally a symbol) to an existing element.

The binding is metadata used by 'tld impact' to reconcile code changes with the
authored architecture. It never changes the element's place in the diagram.`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ElementRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := strings.TrimSpace(args[0])
			filePath = strings.TrimSpace(filePath)
			symbol = strings.TrimSpace(symbol)
			if filePath == "" && symbol == "" {
				return fmt.Errorf("nothing to bind: provide --file and/or --symbol")
			}
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			element, ok := ws.Elements[ref]
			if !ok {
				return fmt.Errorf("element %q not found", ref)
			}
			spec := &workspace.Element{
				Name:     element.Name,
				Kind:     element.Kind,
				FilePath: filePath,
				Symbol:   symbol,
			}
			if err := workspace.UpsertElement(*wdir, ref, spec); err != nil {
				return fmt.Errorf("bind element: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "bind", "bind", ref)
			}
			term.Successf(cmd.OutOrStdout(), "bind: %s", ref)
			if filePath != "" {
				term.Infof(cmd.OutOrStdout(), "file=%s", filePath)
			}
			if symbol != "" {
				term.Infof(cmd.OutOrStdout(), "symbol=%s", symbol)
			}
			return nil
		},
	}

	c.Flags().StringVar(&filePath, "file", "", "code path or glob this element owns")
	c.Flags().StringVar(&symbol, "symbol", "", "named code symbol within --file")
	_ = c.RegisterFlagCompletionFunc("file", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveDefault
	})
	return c
}
