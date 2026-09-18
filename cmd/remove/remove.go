package remove

import (
	"fmt"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
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
	var dryRun bool
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "element <ref>",
		Short: "Remove an element from elements.yaml",
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
			// Capture server IDs before YAML removal (RemoveElement also
			// drops the cached metadata entries).
			preWS, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			var elementID, viewID int32
			if preWS.Meta != nil {
				if m, ok := preWS.Meta.Elements[ref]; ok && m != nil {
					elementID = int32(m.ID)
				}
				if m, ok := preWS.Meta.Views[ref]; ok && m != nil {
					viewID = int32(m.ID)
				}
			}
			if err := workspace.RemoveElement(*wdir, ref); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return fmt.Errorf("remove element: %w", err)
			}
			if err := runRemoveElementServer(cmd, preWS, target, dataDir, ref, elementID, viewID); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove element", "remove", ref)
			}
			term.Successf(cmd.OutOrStdout(), "del: %s", ref)
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

// runRemoveElementServer deletes the element (and its view, if any) on the
// server synchronously. The YAML cache entry was already removed by the caller;
// IDs were captured beforehand.
func runRemoveElementServer(cmd *cobra.Command, ws *workspace.Workspace, target, dataDir, ref string, elementID, viewID int32) error {
	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return err
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return err
		}
	}
	ctx := cmd.Context()
	if elementID == 0 {
		// No cached ID (e.g. hand-written YAML): nothing to delete server-side.
		return nil
	}
	if err := runner.DeleteElement(ctx, elementID); err != nil {
		return cmdutil.WithUnauthorizedHint("server delete element failed", err)
	}
	if viewID != 0 {
		_ = runner.DeleteView(ctx, viewID)
	}
	return nil
}

func newConnectorCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		view    string
		from    string
		to      string
		label   string
		dryRun  bool
		target  string
		dataDir string
	)

	c := &cobra.Command{
		Use:   "connector",
		Short: "Remove matching connector(s) from connectors.yaml",
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
			// Capture connector IDs before YAML removal.
			preWS, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return err
			}
			matchedKeys := matchConnectorKeys(preWS, view, from, to, label)
			n, err := workspace.RemoveConnectorWithLabel(*wdir, view, from, to, label)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return fmt.Errorf("remove connector: %w", err)
			}
			if n > 0 {
				if err := runRemoveConnectorsServer(cmd, preWS, target, dataDir, matchedKeys); err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
					}
					return err
				}
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove connector", "remove", fmt.Sprintf("%s:%s:%s", view, from, to))
			}
			if n == 0 {
				term.Info(cmd.OutOrStdout(), "No matching connectors found — nothing removed.")
			} else {
				term.Successf(cmd.OutOrStdout(), "del: %d", n)
			}
			return nil
		},
	}

	c.Flags().StringVar(&view, "view", "", "view ref (required)")
	c.Flags().StringVar(&from, "from", "", "source element ref (required)")
	c.Flags().StringVar(&to, "to", "", "target element ref (required)")
	c.Flags().StringVar(&label, "label", "", "connector label")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, or remote")
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

// matchConnectorKeys returns YAML keys matching the view/source/target/label
// selector (empty label matches any single connector).
func matchConnectorKeys(ws *workspace.Workspace, view, from, to, label string) []string {
	var out []string
	if ws == nil {
		return out
	}
	for key, c := range ws.Connectors {
		if c == nil {
			continue
		}
		if c.View != view || c.Source != from || c.Target != to {
			continue
		}
		if label != "" && c.Label != label {
			continue
		}
		out = append(out, key)
	}
	return out
}

// runRemoveConnectorsServer deletes the given connectors on the server
// synchronously. Keys without cached IDs are skipped.
func runRemoveConnectorsServer(cmd *cobra.Command, ws *workspace.Workspace, target, dataDir string, keys []string) error {
	var ids []int32
	if ws.Meta != nil {
		for _, k := range keys {
			if m, ok := ws.Meta.Connectors[k]; ok && m != nil && m.ID != 0 {
				ids = append(ids, int32(m.ID))
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return err
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return err
		}
	}
	ctx := cmd.Context()
	for _, id := range ids {
		if err := runner.DeleteConnector(ctx, id); err != nil {
			return cmdutil.WithUnauthorizedHint("server delete connector failed", err)
		}
	}
	return exec.DropConnectorMeta(ws.Dir, keys...)
}
