package remove

import (
	"fmt"

	"github.com/mertcikla/tld/v2/cmd/crudsync"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "remove",
		Short: "Remove workspace resources (applies instantly)",
		Long: `Remove workspace resources and apply instantly.

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
	var dryRun bool
	var yamlOnly bool
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "element <ref>",
		Short: "Remove an element (applies instantly)",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ElementRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			if dryRun {
				err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					return workspace.RemoveElement(cloneDir, ref)
				})
				if err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
					}
					return fmt.Errorf("dry-run remove element: %w", err)
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove element", "dry-run", ref)
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: del: %s", ref)
				return nil
			}
			if err := workspace.RemoveElement(*wdir, ref); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return fmt.Errorf("remove element: %w", err)
			}
			if yamlOnly {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove element", "remove", ref)
				}
				term.Successf(cmd.OutOrStdout(), "del: %s", ref)
				term.Info(cmd.OutOrStdout(), "YAML only (--yaml-only): not applied.")
				return nil
			}
			if !cmdutil.WantsJSON(*format) {
				term.Successf(cmd.OutOrStdout(), "del: %s", ref)
			}
			_, err := crudsync.SyncAndReport(cmd, *wdir, crudsync.Options{
				Target:  target,
				DataDir: dataDir,
				Command: "remove element",
				Format:  *format,
				Compact: *compact,
			}, cmd.OutOrStdout())
			return err
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().BoolVar(&yamlOnly, "yaml-only", false, "write YAML only without applying")
	c.Flags().StringVar(&target, "target", "", "apply target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func newConnectorCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		view     string
		from     string
		to       string
		label    string
		dryRun   bool
		yamlOnly bool
		target   string
		dataDir  string
	)

	c := &cobra.Command{
		Use:   "connector",
		Short: "Remove matching connector(s) (applies instantly)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRun {
				var n int
				err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					var runErr error
					n, runErr = workspace.RemoveConnectorWithLabel(cloneDir, view, from, to, label)
					return runErr
				})
				if err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
					}
					return fmt.Errorf("dry-run remove connector: %w", err)
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove connector", "dry-run", fmt.Sprintf("%s:%s:%s", view, from, to))
				}
				if n == 0 {
					term.Info(cmd.OutOrStdout(), "Dry-run: no matching connectors found — nothing would be removed.")
				} else {
					term.Successf(cmd.OutOrStdout(), "dry-run: del: %d", n)
				}
				return nil
			}
			n, err := workspace.RemoveConnectorWithLabel(*wdir, view, from, to, label)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return fmt.Errorf("remove connector: %w", err)
			}
			if yamlOnly {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove connector", "remove", fmt.Sprintf("%s:%s:%s", view, from, to))
				}
				if n == 0 {
					term.Info(cmd.OutOrStdout(), "No matching connectors found — nothing removed.")
				} else {
					term.Successf(cmd.OutOrStdout(), "del: %d", n)
				}
				term.Info(cmd.OutOrStdout(), "YAML only (--yaml-only): not applied.")
				return nil
			}
			if !cmdutil.WantsJSON(*format) {
				if n == 0 {
					term.Info(cmd.OutOrStdout(), "No matching connectors found — nothing removed.")
				} else {
					term.Successf(cmd.OutOrStdout(), "del: %d", n)
				}
			}
			_, syncErr := crudsync.SyncAndReport(cmd, *wdir, crudsync.Options{
				Target:  target,
				DataDir: dataDir,
				Command: "remove connector",
				Format:  *format,
				Compact: *compact,
			}, cmd.OutOrStdout())
			return syncErr
		},
	}

	c.Flags().StringVar(&view, "view", "", "view ref (required)")
	c.Flags().StringVar(&from, "from", "", "source element ref (required)")
	c.Flags().StringVar(&to, "to", "", "target element ref (required)")
	c.Flags().StringVar(&label, "label", "", "connector label")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().BoolVar(&yamlOnly, "yaml-only", false, "write YAML only without applying")
	c.Flags().StringVar(&target, "target", "", "apply target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	_ = c.MarkFlagRequired("view")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	elementComp := func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	}
	_ = c.RegisterFlagCompletionFunc("view", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ViewRefs(wdir)
	})
	_ = c.RegisterFlagCompletionFunc("from", elementComp)
	_ = c.RegisterFlagCompletionFunc("to", elementComp)
	return c
}
