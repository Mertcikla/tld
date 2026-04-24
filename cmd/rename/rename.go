package rename

import (
	"fmt"

	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/completion"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRenameCmd(wdir *string) *cobra.Command {
	var from string
	var to string

	c := &cobra.Command{
		Use:   "rename",
		Short: "Rename an element in elements.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}
			err := workspace.RenameElement(*wdir, from, to)
			if err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "rename", err)
				}
				return fmt.Errorf("rename element: %w", err)
			}
			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "rename", "rename", fmt.Sprintf("%s -> %s", from, to))
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Renamed element %s to %s in elements.yaml\n", from, to)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Note: References in connectors.yaml and other diagrams were updated automatically.")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Change recorded locally. Run 'tld apply' to push to cloud.")
			return nil
		},
	}

	c.Flags().StringVar(&from, "from", "", "current element ref (required)")
	c.Flags().StringVar(&to, "to", "", "new element ref (required)")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	_ = c.RegisterFlagCompletionFunc("from", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	return c
}
