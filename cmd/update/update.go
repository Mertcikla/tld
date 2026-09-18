package update

import (
	"fmt"

	"github.com/mertcikla/tld/v2/cmd/crudsync"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewUpdateCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "update",
		Short: "Update a resource field (applies instantly)",
		Long: `Update a resource field and apply instantly.

Use --yaml-only to stage the YAML change without applying.`,
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
	var yamlOnly bool
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "element <ref> <field> <value>",
		Short: "Update an element field (applies instantly)",
		Args:  cobra.ExactArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completion.ElementRefs(wdir)
			case 1:
				return completion.ElementFields(), cobra.ShellCompDirectiveNoFileComp
			default:
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, field, value := args[0], args[1], args[2]
			if err := workspace.UpdateElementField(*wdir, ref, field, value); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update element", err)
				}
				return fmt.Errorf("update element: %w", err)
			}
			if yamlOnly {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update element", "update", ref)
				}
				term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
				term.Info(cmd.OutOrStdout(), "YAML only (--yaml-only): not applied.")
				return nil
			}
			if !cmdutil.WantsJSON(*format) {
				term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
			}
			_, err := crudsync.SyncAndReport(cmd, *wdir, crudsync.Options{
				Target:  target,
				DataDir: dataDir,
				Command: "update element",
				Format:  *format,
				Compact: *compact,
			}, cmd.OutOrStdout())
			return err
		},
	}
	c.Flags().BoolVar(&yamlOnly, "yaml-only", false, "write YAML only without applying")
	c.Flags().StringVar(&target, "target", "", "apply target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func newConnectorCmd(wdir, format *string, compact *bool) *cobra.Command {
	var yamlOnly bool
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "connector <ref> <field> <value>",
		Short: "Update a connector field (applies instantly)",
		Args:  cobra.ExactArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completion.ConnectorKeys(wdir)
			case 1:
				return completion.ConnectorFields(), cobra.ShellCompDirectiveNoFileComp
			case 2:
				if args[1] == "direction" {
					return completion.ConnectorDirections(), cobra.ShellCompDirectiveNoFileComp
				}
				return nil, cobra.ShellCompDirectiveNoFileComp
			default:
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, field, value := args[0], args[1], args[2]
			if err := workspace.UpdateConnectorField(*wdir, ref, field, value); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update connector", err)
				}
				return fmt.Errorf("update connector: %w", err)
			}
			if yamlOnly {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update connector", "update", ref)
				}
				term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
				term.Info(cmd.OutOrStdout(), "YAML only (--yaml-only): not applied.")
				return nil
			}
			if !cmdutil.WantsJSON(*format) {
				term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
			}
			_, err := crudsync.SyncAndReport(cmd, *wdir, crudsync.Options{
				Target:  target,
				DataDir: dataDir,
				Command: "update connector",
				Format:  *format,
				Compact: *compact,
			}, cmd.OutOrStdout())
			return err
		},
	}
	c.Flags().BoolVar(&yamlOnly, "yaml-only", false, "write YAML only without applying")
	c.Flags().StringVar(&target, "target", "", "apply target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}
