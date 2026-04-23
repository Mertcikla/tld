package remove

import (
	"fmt"

	"github.com/mertcikla/diag/tld/internal/cmdutil"
	"github.com/mertcikla/diag/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "remove",
		Short: "Remove workspace resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return cobra.NoArgs(cmd, args)
		},
	}
	c.AddCommand(newElementCmd(wdir, format, compact))
	c.AddCommand(newConnectorCmd(wdir, format, compact))
	return c
}

func newElementCmd(wdir, format *string, compact *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "element <ref>",
		Short: "Remove an element from elements.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			if err := workspace.RemoveElement(*wdir, ref); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return fmt.Errorf("remove element: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove element", "remove", ref)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s from elements.yaml\n", ref)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Change recorded locally in elements.yaml. Run 'tld apply' to push to cloud.")
			return nil
		},
	}
}

func newConnectorCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		view string
		from string
		to   string
	)

	c := &cobra.Command{
		Use:   "connector",
		Short: "Remove matching connector(s) from connectors.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, err := workspace.RemoveConnector(*wdir, view, from, to)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return fmt.Errorf("remove connector: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove connector", "remove", fmt.Sprintf("%s:%s:%s", view, from, to))
			}
			if n == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No matching connectors found - nothing removed.")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %d connector(s) from connectors.yaml\n", n)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Change recorded locally in connectors.yaml. Run 'tld apply' to push to cloud.")
			}
			return nil
		},
	}

	c.Flags().StringVar(&view, "view", "", "view ref (required)")
	c.Flags().StringVar(&from, "from", "", "source element ref (required)")
	c.Flags().StringVar(&to, "to", "", "target element ref (required)")
	_ = c.MarkFlagRequired("view")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")
	return c
}
