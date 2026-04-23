package remove

import (
	"github.com/mertcikla/diag/tld/internal/cmdutil"
	"fmt"

	"github.com/mertcikla/diag/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func newRemoveElementCmd(wdir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "element <ref>",
		Short: "Remove an element from elements.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			if err := workspace.RemoveElement(*wdir, ref); err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "remove element", err)
				}
				return fmt.Errorf("remove element: %w", err)
			}
			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "remove element", "remove", ref)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s from elements.yaml\n", ref)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Change recorded locally in elements.yaml. Run 'tld apply' to push to cloud.")
			return nil
		},
	}
}
